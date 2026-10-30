package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"log/slog"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/kms"
	"markdown.ninja/pkg/services/certmanager"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/proxyproto"
	"markdown.ninja/webapp"
)

type server struct {
	db db.DB

	kernelService        kernel.Service
	websitesService      websites.Service
	contactsService      contacts.Service
	emailsService        emails.Service
	storeService         store.Service
	eventsService        events.Service
	siteService          site.Service
	contentService       content.Service
	organizationsService organizations.Service
	kms                  *kms.Kms

	// env                config.Env
	webappDomain            string
	websitesRootDomain      string
	stripeWebhookSecret     string
	stripePublicKey         string
	httpConfig              config.Http
	pingooConfig            config.Pingoo
	pingooClient            *pingoo.Client
	logger                  *slog.Logger
	webappIndexHtmlTemplate *template.Template
	webappIndexHtmlHash     []byte
	emailsConfig            config.Emails
	websitesBaseUrl         *url.URL
	blockedCountries        []string

	autocertManager *autocert.Manager
	certManager     *certmanager.CertManager

	webappFs      fs.FS
	webappHandler func(res http.ResponseWriter, req *http.Request)
}

func Start(ctx context.Context, conf config.Config, db db.DB, pingooClient *pingoo.Client, kernelService kernel.Service,
	websitesService websites.Service, contactsService contacts.Service, emailsService emails.Service,
	storeService store.Service, eventsService events.Service, siteService site.Service, contentService content.Service,
	organizationsService organizations.Service, logger *slog.Logger, kms *kms.Kms,
) (err error) {
	webappFs := webapp.FS()
	webappHandler, err := httpx.WebappHandler(webapp.FS(), nil)
	if err != nil {
		return err
	}

	webappIndexHtml, err := fs.ReadFile(webappFs, "index.html")
	if err != nil {
		return fmt.Errorf("server: error reading webapp/index.html: %w", err)
	}
	webappIndexHtmlHash := blake3.Sum256(webappIndexHtml)
	webappIndexHtmlTemplate, err := template.New("webapp/index.html").
		Parse(string(webappIndexHtml))
	if err != nil {
		return fmt.Errorf("server: error parsing webapp/index.html template: %w", err)
	}

	autocertManager := &autocert.Manager{
		// Email:  "",
		Prompt: autocert.AcceptTOS,
		// HostPolicy: certManager.HostPolicy,
		// Cache: certManager,
	}

	certManager, err := certmanager.NewCertManager(ctx,
		db, kms, autocertManager, websitesService, conf.HTTP, kernelService,
	)
	if err != nil {
		return fmt.Errorf("server: error creating Certmanager: %w", err)
	}
	// it's okay to do that after creating certManager because autocertManager is a pointer
	autocertManager.Cache = certManager

	server := server{
		db: db,

		kernelService:        kernelService,
		websitesService:      websitesService,
		contactsService:      contactsService,
		emailsService:        emailsService,
		storeService:         storeService,
		eventsService:        eventsService,
		siteService:          siteService,
		contentService:       contentService,
		organizationsService: organizationsService,
		kms:                  kms,

		webappDomain:            conf.HTTP.WebappDomain,
		websitesRootDomain:      conf.HTTP.WebsitesRootDomain,
		stripeWebhookSecret:     conf.Stripe.WebhookSecret,
		stripePublicKey:         conf.Stripe.PublicKey,
		httpConfig:              conf.HTTP,
		pingooClient:            pingooClient,
		pingooConfig:            conf.Pingoo,
		logger:                  logger,
		webappIndexHtmlTemplate: webappIndexHtmlTemplate,
		webappIndexHtmlHash:     webappIndexHtmlHash[:],
		emailsConfig:            conf.Emails,
		websitesBaseUrl:         conf.HTTP.WebsitesBaseUrl,
		blockedCountries:        conf.BlockedCountries,

		autocertManager: autocertManager,
		certManager:     certManager,

		webappFs:      webappFs,
		webappHandler: webappHandler,
	}

	return server.run(ctx)
}

func (server *server) run(ctx context.Context) (err error) {
	logger := slogx.FromCtx(ctx)
	portStr := strconv.Itoa(int(server.httpConfig.Port))
	shutdownErr := make(chan error)
	http3ShutdownErr := make(chan error)

	routes, err := server.routes(ctx)
	if err != nil {
		return
	}

	// httpRedirectServer is used by certmanager to get certificates and redirect HTTP requests to HTTPS
	// var httpRedirectServer *http.Server
	// var listener net.Listener

	// TODO: correct HTTP/2 settings
	http2Server := &http2.Server{
		IdleTimeout:     idleTimeout,
		ReadIdleTimeout: http2ReadIdleTimeout,
	}
	h2cHandler := h2c.NewHandler(routes, http2Server)

	// h2cHandler loads the first request entierly in memory which may be abused.
	// Thus, we limit the size of requests to the maximum size of uploads (assets) + 10 MB for metadata
	httpHandler := http.MaxBytesHandler(h2cHandler, kernel.MaxAssetSize+10_000_000)

	var http3Server *http3.Server
	httpServer := http.Server{
		Addr:              server.httpConfig.Ip + ":" + portStr,
		Handler:           httpHandler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		// we need to disable http's logger because the TCP healthchecks are flooding our logs with
		// TLS handshake errors
		ErrorLog: log.New(io.Discard, "", 0),
	}

	go func() {
		<-ctx.Done()
		logger.Info("server: Shutting down HTTP server", slog.String("shutdown_timeout", shutdownTimeout.String()))
		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		var serverShutdownWaitGroup sync.WaitGroup
		serverShutdownWaitGroup.Go(func() {
			shutdownErr <- httpServer.Shutdown(ctx)
		})
		if server.httpConfig.Tls {
			serverShutdownWaitGroup.Go(func() {
				http3ShutdownErr <- http3Server.Shutdown(ctx)
			})
		}
		serverShutdownWaitGroup.Wait()
	}()

	if server.httpConfig.Tls {
		tlsConfig := server.autocertManager.TLSConfig()
		tlsConfig.GetCertificate = server.certManager.GetCertificate
		tlsConfig.MinVersion = tls.VersionTLS13
		httpServer.TLSConfig = tlsConfig

		http3Server = &http3.Server{
			Addr:      server.httpConfig.Ip + ":" + portStr,
			Port:      int(server.httpConfig.Port),
			TLSConfig: tlsConfig,
			QUICConfig: &quic.Config{
				Allow0RTT:      false,
				MaxIdleTimeout: idleTimeout,
			},
			Handler:     routes,
			IdleTimeout: idleTimeout,
			Logger:      logger,
		}
	}

	logger.Info("server: Starting HTTP server", slog.String("address", httpServer.Addr), slog.Bool("tls", server.httpConfig.Tls))
	httpServerListenerConfig := net.ListenConfig{}
	httpServerListenerConfig.SetMultipathTCP(false)
	httpServerListener, err := httpServerListenerConfig.Listen(ctx, "tcp", httpServer.Addr)
	if err != nil {
		return fmt.Errorf("server: error listening on port [%s]: %w", portStr, err)
	}
	if server.httpConfig.ProxyProtocol {
		httpServerListener = &proxyproto.Listener{
			Listener:          httpServerListener,
			ReadHeaderTimeout: 10 * time.Second,
		}
	}
	defer httpServerListener.Close()

	if server.httpConfig.Tls {
		var errGroup errgroup.Group
		errGroup.Go(func() error {
			logger.Info("server: Starting HTTP3/QUIC server", slog.String("address", httpServer.Addr))
			return http3Server.ListenAndServe()
		})
		errGroup.Go(func() error {
			return httpServer.ServeTLS(httpServerListener, "", "")
		})
		err = errGroup.Wait()
	} else {
		err = httpServer.Serve(httpServerListener)
	}

	// ErrServerClosed hapens when shutting down the server via context e.g. for graceful shutdown
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	var errHttp3Shutdown error
	errHttpShutdown := <-shutdownErr
	if server.httpConfig.Tls {
		errHttp3Shutdown = <-http3ShutdownErr
	}

	return errors.Join(errHttpShutdown, errHttp3Shutdown)
}
