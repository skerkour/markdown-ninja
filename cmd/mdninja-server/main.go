package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/stripe/stripe-go/v81"
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/migrations"
	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/buildinfo"
	"markdown.ninja/pkg/jwt"
	"markdown.ninja/pkg/scheduler"
	"markdown.ninja/pkg/server"
	contacts "markdown.ninja/pkg/services/contacts/service"
	content "markdown.ninja/pkg/services/content/service"
	emails "markdown.ninja/pkg/services/emails/service"
	events "markdown.ninja/pkg/services/events/service"
	kernel "markdown.ninja/pkg/services/kernel/service"
	organizations "markdown.ninja/pkg/services/organizations/service"
	site "markdown.ninja/pkg/services/site/service"
	store "markdown.ninja/pkg/services/store/service"
	websites "markdown.ninja/pkg/services/websites/service"
	"markdown.ninja/pkg/worker"
	"markdown.ninja/stdx/cobra"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/migrate"
	"markdown.ninja/stdx/queue/postgres"
)

var flagServerConfigPath string

func init() {
	rootCmd.Flags().StringVar(&flagServerConfigPath, "config", config.DefaultConfigPath, fmt.Sprintf("Configuration file (default: %s)", config.DefaultConfigPath))

	rootCmd.AddCommand(healthcheckCmd)
}

func main() {
	log.SetOutput(os.Stdout)

	err := rootCmd.ExecuteContext(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}

var rootCmd = &cobra.Command{
	Use:           "mdninja-server",
	Short:         "Markdown Ninja Server. Visit https://markdown.ninja for more information",
	Version:       buildinfo.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		// for graceful shutdown
		ctx, cancelCtx := signal.NotifyContext(ctx, os.Interrupt, os.Kill,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT,
		)
		defer cancelCtx()

		logger, logLevel, lokiWriter := newLogger(ctx)
		// inject the logger into ctx
		ctx = slogx.ToCtx(ctx, logger)

		conf, err := config.Load(ctx, flagServerConfigPath)
		if err != nil {
			return err
		}

		logLevel.Set(conf.Logs.Level)
		if conf.Logs.LokiEndpoint != nil {
			lokiWriter.SetEndpoint(*conf.Logs.LokiEndpoint)
		}

		logConfig(logger, conf)

		// Database & queue
		dbPool, err := db.Connect(conf.Database.Url, conf.Database.PoolSize)
		if err != nil {
			return err
		}
		logger.Info("successfully connected to database")

		err = migrateDatabase(ctx, dbPool)
		if err != nil {
			return err
		}
		logger.Debug("successfully migrated database schema")

		queue := postgres.NewPostgreSQLQueue(ctx, dbPool, logger)

		// initialize drivers
		mailer, err := loadMailer(conf)
		if err != nil {
			return err
		}

		s3Client, err := loadS3(conf)
		if err != nil {
			return err
		}

		kms, err := loadKms(conf)
		if err != nil {
			return err
		}

		jwtProvider, err := jwt.NewProvider(ctx, dbPool, kms, &jwt.NewProviderOptions{
			Issuer: conf.Jwt.Issuer,
		})
		if err != nil {
			return err
		}

		pingooClient, err := pingoo.NewClient(ctx, conf.Pingoo.ApiKey, conf.Pingoo.ProjectID, &pingoo.ClientConfig{
			Url:    conf.Pingoo.Url,
			Logger: logger,
			GetLogger: func(ctx context.Context) *slog.Logger {
				return slogx.FromCtx(ctx)
			},
		})
		if err != nil {
			return err
		}

		stripe.Key = conf.Stripe.SecretKey
		stripe.EnableTelemetry = false

		dnsResolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				dialer := net.Dialer{
					Timeout: 3 * time.Second,
				}
				dnsServer := "1.1.1.1"
				return dialer.DialContext(ctx, network, dnsServer)
			},
		}

		// init services
		kernelService := kernel.NewKernelService(conf, dbPool, queue, mailer, pingooClient)

		organizationsService := organizations.NewOrganizationsService(conf, dbPool, mailer, queue, kernelService, pingooClient)

		contentService, err := content.NewContentService(conf, dbPool, queue, s3Client, kernelService, organizationsService)
		if err != nil {
			return err
		}

		eventsService, err := events.NewService(ctx, dbPool, dbPool /*eventsDb*/, queue, kernelService)
		if err != nil {
			return err
		}

		emailsService := emails.NewEmailsService(conf, dbPool, queue, mailer, dnsResolver, kernelService,
			eventsService, contentService, organizationsService,
		)

		websitesService, err := websites.NewWebsitesService(conf, dbPool, queue, mailer, s3Client,
			kernelService, emailsService, contentService, eventsService, organizationsService,
		)
		if err != nil {
			return err
		}

		contactsService, err := contacts.NewContactsService(conf, dbPool, mailer, queue, jwtProvider,
			kernelService, websitesService, eventsService, emailsService, pingooClient)
		if err != nil {
			return err
		}

		storeService, err := store.NewStoreService(dbPool, queue, conf, mailer,
			kernelService, websitesService, contentService, contactsService, eventsService, emailsService,
			organizationsService, pingooClient,
		)
		if err != nil {
			return err
		}

		siteService, err := site.NewSiteService(conf, dbPool, queue, mailer, logger, kernelService, websitesService,
			contentService, eventsService, contactsService, emailsService, storeService,
		)
		if err != nil {
			return err
		}

		contentService.InjectServices(websitesService, storeService, emailsService)
		websitesService.InjectServices(storeService, contactsService)
		emailsService.InjectServices(websitesService, contactsService)
		eventsService.InjectServices(websitesService)
		contactsService.InjectServices(storeService)
		organizationsService.InjectServices(websitesService, eventsService, contentService, storeService)

		var gracefulShutdownWaitGroup sync.WaitGroup

		if conf.Worker.Concurrency != 0 {
			gracefulShutdownWaitGroup.Add(1)
			go func() {
				errWorker := worker.Start(ctx, logger, conf.Worker.Concurrency, queue,
					kernelService, websitesService, emailsService, storeService, contentService,
					siteService, contactsService, eventsService, organizationsService,
				)
				gracefulShutdownWaitGroup.Done()
				if errWorker != nil {
					logger.Error("cli.server: error running worker", slogx.Err(errWorker))
				}
			}()
		}

		gracefulShutdownWaitGroup.Add(1)
		go func() {
			errScheduler := scheduler.Start(ctx, dbPool, queue, jwtProvider, emailsService, contactsService, siteService,
				kernelService, contentService, storeService,
			)
			gracefulShutdownWaitGroup.Done()
			if errScheduler != nil {
				logger.Error("cli.server: error running scheduler", slogx.Err(errScheduler))
			}
		}()

		err = server.Start(ctx, conf, dbPool, pingooClient, kernelService, websitesService, contactsService, emailsService,
			storeService, eventsService, siteService, contentService, organizationsService, logger, kms)
		if err != nil {
			logger.Error("cli.server: error running server", slogx.Err(err))
			cancelCtx()
		}

		gracefulShutdownWaitGroup.Wait()
		// wait a little bit more to be sure to flush all the buffers
		time.Sleep(time.Second)

		return err
	},
}

func migrateDatabase(ctx context.Context, dbConn db.DB) (err error) {
	logger := slogx.FromCtx(ctx)

	migrations, err := migrate.Load(migrations.MigrationsFs)
	if err != nil {
		err = fmt.Errorf("error loading database migrations: %w", err)
		return
	}

	err = db.Migrate(ctx, logger, dbConn, migrations)
	if err != nil {
		err = fmt.Errorf("error applying database migrations: %w", err)
		return
	}

	return
}

func logConfig(logger *slog.Logger, conf config.Config) {
	logger.Info("config: Config successfully loaded.",
		slog.String("git_commit", buildinfo.GitCommit()),
		slog.Group("config",
			slog.Group("go",
				slog.Int("GOMAXPROCS", runtime.GOMAXPROCS(-1)),
				slog.String("GOMEMLIMIT", os.Getenv("GOMEMLIMIT")),
			),
			slog.Group("database",
				slog.Int("pool_size", conf.Database.PoolSize),
			),
			slog.Group("emails",
				slog.String("provider", string(conf.Emails.Provider)),
				slog.String("notify_address", conf.Emails.NotifyAddressStr),
				slog.String("contact_address", conf.Emails.ContactAddressStr),
			),
			slog.Group("s3",
				slog.String("provider", string(conf.S3.Provider)),
			),
			slog.Bool("aws", conf.Aws != nil),
			slog.Bool("scaleway", conf.Scaleway != nil),
			slog.Group("pingoo",
				slog.String("project", conf.Pingoo.ProjectID),
				slog.String("url", formatStringPtr(conf.Pingoo.Url)),
			),
			slog.Group("http",
				slog.String("ip", conf.HTTP.Ip),
				slog.Uint64("port", uint64(conf.HTTP.Port)),
				slog.Bool("proxy_protocol", conf.HTTP.ProxyProtocol),
				slog.String("webapp_base_url", conf.HTTP.WebappBaseUrlStr),
				slog.String("websites_base_url", conf.HTTP.WebsitesBaseUrlStr),
				slog.String("websites_root_domain", conf.HTTP.WebsitesRootDomain),
				slog.String("websites_port", conf.HTTP.WebsitesPort),
				slog.Bool("tls", conf.HTTP.Tls),
			),
			slog.Group("logs",
				slog.String("level", conf.Logs.Level.String()),
			),
		),
	)
}

func formatStringPtr(input *string) string {
	if input == nil {
		return "null"
	}
	return *input
}
