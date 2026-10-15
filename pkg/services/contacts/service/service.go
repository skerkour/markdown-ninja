package service

import (
	"fmt"
	"html/template"

	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/jwt"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/services/contacts/repository"
	"markdown.ninja/pkg/services/contacts/templates"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/queue"
)

type ContactsService struct {
	repo  repository.ContactsRepository
	db    db.DB
	queue queue.Queue
	// env    config.Env
	mailer      mailer.Mailer
	jwtProvider *jwt.Provider
	pingoo      *pingoo.Client

	kernel          kernel.PrivateService
	websitesService websites.Service
	storeService    store.Service
	eventsService   events.Service
	emailsService   emails.Service

	httpConfig               config.Http
	verifyEmailEmailTemplate *template.Template
}

func NewContactsService(conf config.Config, db db.DB, mailer mailer.Mailer, queue queue.Queue,
	jwtProvider *jwt.Provider, kernel kernel.PrivateService,
	websitesService websites.Service, eventsService events.Service,
	emailsService emails.Service, pingoo *pingoo.Client) (service *ContactsService, err error) {
	repo := repository.NewContactsRepository()

	verifyEmailEmailTemplate, err := template.New("contacts.verifyEmailEmailTemplate").Parse(templates.VerifyEmailEmailTemplate)
	if err != nil {
		err = fmt.Errorf("contacts.NewService: Parsing verifyEmailEmailTemplate: %w", err)
		return
	}

	service = &ContactsService{
		repo:        repo,
		db:          db,
		queue:       queue,
		mailer:      mailer,
		jwtProvider: jwtProvider,

		kernel:          kernel,
		websitesService: websitesService,
		storeService:    nil,
		eventsService:   eventsService,
		emailsService:   emailsService,
		pingoo:          pingoo,

		httpConfig:               conf.HTTP,
		verifyEmailEmailTemplate: verifyEmailEmailTemplate,
	}
	return
}

func (service *ContactsService) InjectServices(storeService store.Service) {
	service.storeService = storeService
}
