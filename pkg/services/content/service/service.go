package service

import (
	"fmt"
	"regexp"
	"text/template"

	"github.com/microcosm-cc/bluemonday"
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/content/repository"
	"markdown.ninja/pkg/services/content/templates"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/pkg/storage"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/set"
)

type ContentService struct {
	repo    repository.ContentRepository
	db      db.DB
	queue   queue.Queue
	storage storage.Storage

	kernel               kernel.PrivateService
	websitesService      websites.Service
	storeService         store.Service
	emailsService        emails.Service
	organizationsService organizations.Service

	snippetNameBlocklist set.Set[string]
	pageUrlBlocklist     set.Set[string]
	snippetsRegexp       *regexp.Regexp
	htmlStripper         *bluemonday.Policy
	videoIframeTemplate  *template.Template
	xssSanitizer         *bluemonday.Policy
	httpConfig           config.Http
}

func NewContentService(conf config.Config, db db.DB, queue queue.Queue, storage storage.Storage,
	kernel kernel.PrivateService, organizationsService organizations.Service) (service *ContentService, err error) {
	repo := repository.NewContentRepository()

	snippetNameBlocklist := set.NewFromSlice(content.SnippetNameBlocklist)
	pageUrlBlocklist := set.NewFromSlice(content.PageUrlBlocklist)

	snippetsRegexp, err := regexp.Compile("{{<.*>}}")
	if err != nil {
		return nil, fmt.Errorf("content.NewService: compiling snippetsRegexp: %w", err)
	}

	htmlStripper := bluemonday.StrictPolicy()
	xssSanitizer := bluemonday.UGCPolicy()
	xssSanitizer.RequireNoFollowOnLinks(false)

	// we use a text/template instead of html/template because we know for sure that the input is safe
	// and to avoid "&" characters being replaced to "&amp;"
	videoIframeTemplate, err := template.New("content.videoIframeTemplate").Parse(templates.VideoIframeTemplate)
	if err != nil {
		return nil, fmt.Errorf("content.NewService: Parsing videoIframeTemplate: %w", err)
	}

	service = &ContentService{
		repo:    repo,
		db:      db,
		queue:   queue,
		storage: storage,

		kernel:               kernel,
		organizationsService: organizationsService,
		websitesService:      nil,
		storeService:         nil,
		emailsService:        nil,

		snippetNameBlocklist: snippetNameBlocklist,
		pageUrlBlocklist:     pageUrlBlocklist,
		snippetsRegexp:       snippetsRegexp,
		htmlStripper:         htmlStripper,
		videoIframeTemplate:  videoIframeTemplate,
		xssSanitizer:         xssSanitizer,
		httpConfig:           conf.HTTP,
	}

	return
}

func (service *ContentService) InjectServices(websitesService websites.Service, storeService store.Service,
	emailsService emails.Service) {
	service.websitesService = websitesService
	service.storeService = storeService
	service.emailsService = emailsService
}
