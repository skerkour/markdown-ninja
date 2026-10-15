package service

import (
	"html/template"
	"net"
	"time"

	"golang.org/x/sync/singleflight"
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails/repository"
	"markdown.ninja/pkg/services/emails/templates"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/memorycache"
	"markdown.ninja/stdx/queue"
)

type EmailsService struct {
	config     config.Config
	repo       repository.EmailsRepository
	db         db.DB
	queue      queue.Queue
	mailer     mailer.Mailer
	httpConfig config.Http

	kernel               kernel.PrivateService
	websitesService      websites.Service
	contactsService      contacts.Service
	eventsService        events.Service
	contentService       content.Service
	organizationsService organizations.Service

	newsletterEmailTemplate    *template.Template
	dnsResolver                *net.Resolver
	sendEmailCache             *memorycache.Cache[string, any]
	sendEmailSingleflightGroup singleflight.Group
}

func NewEmailsService(conf config.Config, db db.DB, queue queue.Queue, mailer mailer.Mailer, dnsResolver *net.Resolver, kernel kernel.PrivateService,
	eventsService events.Service, contentService content.Service,
	organizationsService organizations.Service) *EmailsService {
	repo := repository.NewEmailsRepository()
	newsletterEmailTemplate := template.Must(template.New("emails.newsletterEmailTemplate").Parse(templates.NewsletterEmailTemplate))

	sendEmailCache := memorycache.New(
		memorycache.WithTTL[string, any](time.Minute),
	)

	return &EmailsService{
		config:     conf,
		repo:       repo,
		db:         db,
		queue:      queue,
		mailer:     mailer,
		httpConfig: conf.HTTP,

		kernel:               kernel,
		websitesService:      nil,
		contactsService:      nil,
		eventsService:        eventsService,
		contentService:       contentService,
		organizationsService: organizationsService,

		newsletterEmailTemplate:    newsletterEmailTemplate,
		dnsResolver:                dnsResolver,
		sendEmailCache:             sendEmailCache,
		sendEmailSingleflightGroup: singleflight.Group{},
	}
}

func (service *EmailsService) InjectServices(websitesService websites.Service, contactsService contacts.Service) {
	service.websitesService = websitesService
	service.contactsService = contactsService
}
