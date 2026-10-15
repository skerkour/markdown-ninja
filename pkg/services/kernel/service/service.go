package service

import (
	"html/template"

	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/services/kernel/templates"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/queue"
)

type KernelService struct {
	config config.Config
	db     db.DB
	queue  queue.Queue
	mailer mailer.Mailer

	pingooClient    *pingoo.Client
	stripePublicKey string
	emailsConfig    config.Emails
	// pingooAppId                  string
	// pingooEndpoint               string
	organizationsService organizations.Service
	pingooConfig         config.Pingoo

	signupEmailTemplate             *template.Template
	loginAlertEmailTemplate         *template.Template
	twoFaDisabledAlertEmailTemplate *template.Template
}

func NewKernelService(conf config.Config, db db.DB, queue queue.Queue, mailer mailer.Mailer,
	pingooClient *pingoo.Client) *KernelService {
	signupEmailTemplate := template.Must(template.New("kernel.signupEmailTemplate").Parse(templates.SignupEmailTemplate))
	loginAlertEmailTemplate := template.Must(template.New("kernel.loginAlertEmailTemplate").Parse(templates.LoginAlertEmailTemplate))
	twoFaDisabledAlertEmailTemplate := template.Must(template.New("kernel.twoFaDisabledAlertEmailTemplate").Parse(templates.TwoFaDisabledEmailTemplate))

	return &KernelService{
		config: conf,
		db:     db,
		queue:  queue,
		mailer: mailer,

		pingooClient:    pingooClient,
		stripePublicKey: conf.Stripe.PublicKey,
		emailsConfig:    conf.Emails,
		// pingooAppId:                  conf.Pingoo.AppID,
		// pingooEndpoint:               conf.Pingoo.Endpoint,
		organizationsService: nil,
		pingooConfig:         conf.Pingoo,

		signupEmailTemplate:             signupEmailTemplate,
		loginAlertEmailTemplate:         loginAlertEmailTemplate,
		twoFaDisabledAlertEmailTemplate: twoFaDisabledAlertEmailTemplate,
	}
}

func (service *KernelService) InjectServices(organizationsService organizations.Service) {
	service.organizationsService = organizationsService
}
