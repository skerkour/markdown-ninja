package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/klauspost/compress/zstd"
	"markdown.ninja/pingoo-go"
	pingoomiddleware "markdown.ninja/pingoo-go/middleware"
	"markdown.ninja/pingoo-go/rules"
	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/pkg/server/middlewares"
	"markdown.ninja/pkg/server/website"
	"markdown.ninja/stdx/httpx/hostrouter"
	"markdown.ninja/stdx/httpx/middlewarex"
)

func (server *server) routes(ctx context.Context) (rootRouter chi.Router, err error) {
	rootRouter = chi.NewRouter()
	websiteRoutes := website.Routes(ctx, server.siteService, server.contactsService, server.storeService)
	// api := NewApi(server.webappDomain, server.kernelService, server.websitesService, server.contactsService,
	// 	server.emailsService, server.storeService, server.eventsService, server.contentService, server.organizationsService)

	compressionMiddleware := chimiddleware.NewCompressor(5, "application/*", "text/*", "image/svg+xml")
	compressionMiddleware.SetEncoder("zstd", func(encoderRes io.Writer, encoderLevel int) io.Writer {
		zstdEncoder, encoderErr := zstd.NewWriter(encoderRes, zstd.WithEncoderCRC(true), zstd.WithEncoderLevel(zstd.SpeedDefault))
		if encoderErr != nil {
			panic(fmt.Errorf("server: error instantiating zstd for compressing HTTP responses: %w", encoderErr))
		}
		return zstdEncoder
	})

	pingooMiddlewareConfig := pingoo.MiddlewareConfig{
		Logging: pingoo.LoggingConfig{
			GetLogger: func(ctx context.Context) *slog.Logger {
				// return slogx.FromCtx(ctx)
				return nil
			},
		},
		Rules: []rules.Rule{
			{
				Actions: []rules.Action{
					rules.ActionSetResponseHeader{
						Headers: []rules.HttpHeader{
							{"X-Content-Type-Options", "nosniff"},
							{"X-Frame-Options", "Deny"},
							// "X-Frame-Options":        "Deny",
							// w.Header().Set("Content-Security-Policy", "default-src 'self'")
							// w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
						},
					},
				},
			},
			// {
			// 	Match: func(req *http.Request) bool {
			// 		return !strings.HasPrefix(req.URL.Path, "/__markdown_ninja/")
			// 	},
			// 	Actions: []rules.Action{
			// 		rules.ActionSetResponseHeader{
			// 			Headers: []rules.HttpHeader{
			// 				{"X-Frame-Options", "Deny"},
			// 			},
			// 		},
			// 	},
			// },
		},
	}

	// All routes
	rootRouter.Use(middlewares.RequestID(requestIDHeader))
	rootRouter.Use(middlewares.SetLogger(server.logger))
	rootRouter.Use(pingoomiddleware.LoggingMiddleware(ctx, server.pingooClient, pingooMiddlewareConfig))
	// For now we set recover after RequestID and SetLogger because we need the logger to be set
	// for the log to be sent, but ideally recover would be the first middleware.
	rootRouter.Use(middlewares.Recoverer)
	// rootRouter.Use(chimiddleware.Recoverer)
	rootRouter.Use(chimiddleware.Timeout(900 * time.Second))
	rootRouter.Use(middlewares.SetSecurityHeaders)
	rootRouter.Use(middlewares.SetServerHeader)
	if server.httpConfig.Tls {
		rootRouter.Use(middlewares.AddAltSvcHeader())
	}
	rootRouter.Use(middlewares.CleanupHostHeader())
	rootRouter.Use(chimiddleware.CleanPath)
	rootRouter.Use(middlewares.Redirects(server.webappDomain, server.websitesRootDomain))
	// rootRouter.Use(chimiddleware.RedirectSlashes)
	rootRouter.Use(server.pingooClient.Middleware(&pingooMiddlewareConfig))
	rootRouter.Use(middlewares.SetHTTPContext(server.pingooClient))
	rootRouter.Use(middlewares.BlockCountries(server.blockedCountries))
	rootRouter.Use(middlewares.Auth(server.webappDomain, server.kernelService, server.organizationsService,
		server.contactsService, server.pingooClient))
	rootRouter.Use(compressionMiddleware.Handler)

	// if server.env != config.EnvDev {
	// 	rootRouter.Use(middleware.Http3AltSvc(nil))
	// }
	// if conf.HTTP.AccessLogs {
	// 	rootRouter.Use(loggingMiddleware)
	// }

	// Webapp & API
	webappAndApiRouter := chi.NewRouter()
	webappAndApiRouter.NotFound(server.serveWebapp)
	webappAndApiRouter.Use(middlewarex.StrictTransportSecurity(nil, true))
	webappAndApiRouter.Route("/api", server.Api)

	// We need to use the /__markdown_ninja/ prefix so it does not interfer with the pages of the websites.
	websiteRoutes.Get("/__markdown_ninja/healthcheck", apiutil.GetEndpointOk(server.kernelService.Healthcheck))
	webappAndApiRouter.Get("/__markdown_ninja/healthcheck", apiutil.GetEndpointOk(server.kernelService.Healthcheck))

	// Host router
	hostRouter := hostrouter.New()
	hostRouter.Map(server.webappDomain, webappAndApiRouter)
	hostRouter.Map("*", websiteRoutes)

	// Final router
	rootRouter.Mount("/", hostRouter)

	return
}
