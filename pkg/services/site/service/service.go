package service

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"regexp"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/zeebo/blake3"
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/ratelimit"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/site/templates"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/memorycache"
	"markdown.ninja/stdx/queue"
	themespkg "markdown.ninja/themes"
)

// TODO: for all caches: should we compress entries?
type SiteService struct {
	db     db.DB
	queue  queue.Queue
	mailer mailer.Mailer

	kernel          kernel.PrivateService
	websitesService websites.Service
	eventsService   events.Service
	contentService  content.Service
	contactsService contacts.Service
	emailsService   emails.Service
	storeService    store.Service

	snippetsRegexp         *regexp.Regexp
	loginEmailTemplate     *template.Template
	subscribeEmailTemplate *template.Template
	httpConfig             config.Http
	// sitesRootDomain string
	defaultIcons map[int]defaultWebsiteIcon

	// the complete rendered HTML for pages
	pagesHtmlCache *memorycache.Cache[string, []byte]
	// cache assets up to 1 MB in size
	assetsCache   *memorycache.Cache[string, []byte]
	pagesCache    *memorycache.Cache[string, site.Page]
	feedsCache    *memorycache.Cache[string, []byte]
	sitemapsCache *memorycache.Cache[string, []byte]

	// pages are cached compressed to reduce the memory usage
	cacheZstdCompressor   *zstd.Encoder
	cacheZstdDecompressor *zstd.Decoder

	rateLimiter *ratelimit.Limiter

	themes map[string]parsedTheme
}

type defaultWebsiteIcon struct {
	Data []byte
	Etag string
}

func NewSiteService(conf config.Config, db db.DB, queue queue.Queue, mailer mailer.Mailer, logger *slog.Logger,
	kernel kernel.PrivateService, websitesService websites.Service, contentService content.Service,
	eventsService events.Service, contactsService contacts.Service,
	emailsService emails.Service, storeService store.Service) (service *SiteService, err error) {

	snippetsRegexp := regexp.MustCompile("{{<.*>}}")

	themes, err := loadThemes()
	if err != nil {
		return
	}

	loginEmailTemplate, err := template.New("site.loginEmailTemplate").Parse(templates.LoginEmailTemplate)
	if err != nil {
		err = fmt.Errorf("site.NewService: Parsing loginEmailTemplate: %w", err)
		return
	}

	subscribeEmailTemplate, err := template.New("site.subscribeEmailTemplate").Parse(templates.SubscribeEmailTemplate)
	if err != nil {
		err = fmt.Errorf("site.NewService: Parsing subscribeEmailTemplate: %w", err)
		return
	}

	defaultIcons, err := loadDefaultWebsitesIcons(themespkg.DefaultIconsFs())
	if err != nil {
		return
	}

	cacheZstdCompressor, _ := zstd.NewWriter(nil)
	cacheZstdDecompressor, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))

	zstdCompressInsertHook := func(key string, value []byte) (string, []byte) {
		// Text compresses very well, especiall for longer blog posts where we can see compression
		// ratios of 5x
		compressedPage := cacheZstdCompressor.EncodeAll(value, make([]byte, 0, len(value)/4))
		return key, compressedPage
	}
	zstdDecompressGetHook := func(key string, value []byte) (string, []byte) {
		// zstd should allocate a buffer of the original size of the data as it's should be stored
		// in Frame_Content_Size
		decompressedValue, err := cacheZstdDecompressor.DecodeAll(value, nil)
		if err != nil {
			logger.Error("site: error decompressing cached item", slog.String("key", key))
			// TODO: How to return error? panic?
			return key, value
		}
		return key, decompressedValue
	}

	pagesHtmlCache := memorycache.New(
		memorycache.WithTTL[string, []byte](48*time.Hour),
		memorycache.WithCapacity[string, []byte](500),
		memorycache.WithInsertHook(zstdCompressInsertHook),
		memorycache.WithGetHook(zstdDecompressGetHook),
	)

	assetsCache := memorycache.New(
		memorycache.WithTTL[string, []byte](48*time.Hour),
		memorycache.WithCapacity[string, []byte](100),
	)

	pagesCache := memorycache.New(
		memorycache.WithTTL[string, site.Page](48*time.Hour),
		memorycache.WithCapacity[string, site.Page](100),
	)

	feedsCache := memorycache.New(
		memorycache.WithTTL[string, []byte](48*time.Hour),
		memorycache.WithCapacity[string, []byte](100),
		memorycache.WithInsertHook(zstdCompressInsertHook),
		memorycache.WithGetHook(zstdDecompressGetHook),
	)

	sitemapsCache := memorycache.New(
		memorycache.WithTTL[string, []byte](48*time.Hour),
		memorycache.WithCapacity[string, []byte](100),
		memorycache.WithInsertHook(zstdCompressInsertHook),
		memorycache.WithGetHook(zstdDecompressGetHook),
	)

	service = &SiteService{
		db:     db,
		queue:  queue,
		mailer: mailer,

		kernel:          kernel,
		eventsService:   eventsService,
		websitesService: websitesService,
		contentService:  contentService,
		contactsService: contactsService,
		emailsService:   emailsService,
		storeService:    storeService,

		snippetsRegexp:         snippetsRegexp,
		loginEmailTemplate:     loginEmailTemplate,
		subscribeEmailTemplate: subscribeEmailTemplate,
		httpConfig:             conf.HTTP,
		defaultIcons:           defaultIcons,
		cacheZstdCompressor:    cacheZstdCompressor,
		cacheZstdDecompressor:  cacheZstdDecompressor,

		pagesHtmlCache: pagesHtmlCache,
		assetsCache:    assetsCache,
		pagesCache:     pagesCache,
		feedsCache:     feedsCache,
		sitemapsCache:  sitemapsCache,

		rateLimiter: ratelimit.New(),

		themes: themes,
	}
	return
}

func loadDefaultWebsitesIcons(defaulticonsFs fs.FS) (defaultIcons map[int]defaultWebsiteIcon, err error) {
	defaultIcons = make(map[int]defaultWebsiteIcon, len(websites.WebsiteIconSizes))

	for iconSize := range websites.WebsiteIconSizes.Iter() {
		var iconData []byte
		iconFilename := fmt.Sprintf("icon-%d.png", iconSize)
		iconData, err = fs.ReadFile(defaulticonsFs, iconFilename)
		if err != nil {
			err = fmt.Errorf("reading default icon (%s); %w", iconFilename, err)
			return
		}

		iconHash := blake3.Sum256(iconData)
		defaultIcons[iconSize] = defaultWebsiteIcon{
			Data: iconData,
			Etag: base64.RawURLEncoding.EncodeToString(iconHash[:]),
		}
	}

	return
}
