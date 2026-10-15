package service

import (
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites/repository"
	"markdown.ninja/pkg/storage"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/queue"
)

type WebsitesService struct {
	config  config.Config
	repo    repository.WebsitesRepository
	db      db.DB
	queue   queue.Queue
	mailer  mailer.Mailer
	storage storage.Storage

	kernel               kernel.PrivateService
	emailsService        emails.Service
	contentService       content.Service
	storeService         store.Service
	eventsService        events.Service
	contactsService      contacts.Service
	organizationsService organizations.Service

	websitesRootDomain string
}

func NewWebsitesService(conf config.Config, db db.DB, queue queue.Queue, mailer mailer.Mailer,
	storage storage.Storage,
	kernel kernel.PrivateService, emailsService emails.Service, contentService content.Service,
	eventsService events.Service, organizationsService organizations.Service) (service *WebsitesService, err error) {
	repo := repository.NewWebsitesRepository()

	service = &WebsitesService{
		config:  conf,
		repo:    repo,
		db:      db,
		queue:   queue,
		mailer:  mailer,
		storage: storage,

		kernel:               kernel,
		emailsService:        emailsService,
		contentService:       contentService,
		eventsService:        eventsService,
		storeService:         nil,
		contactsService:      nil,
		organizationsService: organizationsService,

		websitesRootDomain: conf.HTTP.WebsitesRootDomain,
	}
	return
}

func (service *WebsitesService) InjectServices(storeService store.Service, contactsService contacts.Service) {
	service.storeService = storeService
	service.contactsService = contactsService
}
