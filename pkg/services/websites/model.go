package websites

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/set"
)

type FeedType string

const (
	FeedTypeRss  FeedType = "rss"
	FeedTypeJson FeedType = "json"
)

const (
	WebsiteNameMinLength        = 2
	WebsiteNameMaxLength        = 42
	WebsiteSlugMinLength        = 3
	WebsiteSlugMaxLength        = 42
	WebsiteHeaderMaxLength      = 1500
	WebsiteFooterMaxLength      = 1500
	WebsiteDescriptionMaxLength = 500

	AdMaxLength           = 1500
	AnnouncementMaxLength = 256

	WebsiteIconMaxSize = 5_000_000 // 5 MB

	RobotsTxtMaxLength = 1500

	TemplateBase     = "base.html"
	TemplatePosts    = "posts.html"
	TemplatePage     = "page.html"
	TemplatePost     = "post.html"
	TemplateError    = "error.html"
	TemplateError404 = "error-404.html"
	TemplateTags     = "tags.html"
	TemplateTag      = "tag.html"

	MarkdownNinjaPathPrefix = "/__markdown_ninja"
	PreviewPrefix           = MarkdownNinjaPathPrefix + "/preview/"

	DefaultWebsiteLanguage = "en"

	DefaultTheme = "blog"
)

var (
	WebsiteSlugRegexp = regexp.MustCompile("^[a-z0-9-]{3,42}$")
)

var WebsiteIconSizes = set.NewFromSlice([]int{
	32,
	64,
	128,
	180, // apple
	192, // android
	256,
	512,
	1024,
})

var DefaultWebsiteNavigation = WebsiteNavigation{
	Primary: []WebsiteNavigationItem{
		{
			Label: "Home",
			Url:   new("/"),
		},
		{
			Label: "Blog",
			Url:   new("/blog"),
		},
	},
	Secondary: []WebsiteNavigationItem{},
}

type Currency string

const (
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
)

var AllCurrencies = set.NewFromSlice([]Currency{
	CurrencyUSD,
	CurrencyEUR,
})

var DefaultColors = ThemeColors{
	Background: "#ffffff",
	Text:       "#000000",
	Accent:     "#0047ff",
	// Headings:          "#000000",
	// Links:             "#0ea5e9",
	// ButtonsBackground: "#0ea5e9",
	// ButtonsText:       "#ffffff",
	// border: #0000008e;
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

type Website struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// The ModifiedAt field is used for caching purpose.
	// YOU NEED to update it each time you update something related to the site that are not pages
	// ex: tags
	ModifiedAt time.Time `db:"modified_at" json:"-"`

	BlockedAt     *time.Time        `db:"blocked_at" json:"blocked_at"`
	BlockedReason string            `db:"blocked_reason" json:"-"`
	Name          string            `db:"name" json:"name"`
	Slug          string            `db:"slug" json:"slug"`
	Header        string            `db:"header" json:"header"`
	Footer        string            `db:"footer" json:"footer"`
	Navigation    WebsiteNavigation `db:"navigation" json:"navigation"`
	Language      string            `db:"language" json:"language"`
	PrimaryDomain string            `db:"primary_domain" json:"primary_domain"`
	Description   string            `db:"description" json:"description"`
	RobotsTxt     string            `db:"robots_txt" json:"robots_txt"`
	Currency      Currency          `db:"currency" json:"currency"`
	CustomIcon    bool              `db:"custom_icon" json:"custom_icon"`
	// The BLAKE3 hash of the originally uploaded image
	CustomIconHash kernel.BytesHex `db:"custom_icon_hash" json:"custom_icon_hash"`
	Colors         ThemeColors     `db:"colors" json:"colors"`
	Theme          string          `db:"theme" json:"theme"`
	Announcement   *string         `db:"announcement" json:"announcement"`
	Ad             *string         `db:"ad" json:"ad"`
	Logo           *string         `db:"logo" json:"logo"`
	PoweredBy      bool            `db:"powered_by" json:"powered_by"`

	OrganizationID guid.GUID `db:"organization_id" json:"organization_id"`

	Domains   []Domain   `db:"-" json:"domains"`
	Redirects []Redirect `db:"-" json:"redirects"`
	// Revenue = sales - refunds
	Revenue     *int64 `db:"-" json:"revenue"`
	Subscribers *int64 `db:"-" json:"subscribers"`
}

type WebsiteNavigation struct {
	Primary   []WebsiteNavigationItem `json:"primary" yaml:"primary"`
	Secondary []WebsiteNavigationItem `json:"secondary" yaml:"secondary"`
}

func (item *WebsiteNavigation) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, item)
		return nil
	case string:
		json.Unmarshal([]byte(v), item)
		return nil
	default:
		return fmt.Errorf("WebsiteNavigation.Scan: Unsupported type: %T", v)
	}
}

func (item *WebsiteNavigation) Value() (driver.Value, error) {
	return json.Marshal(item)
}

type WebsiteNavigationItem struct {
	Label    string                  `json:"label"` // TODO: rename?
	Url      *string                 `json:"url,omitempty"`
	Children []WebsiteNavigationItem `json:"children,omitempty"`
}

type ThemeColors struct {
	Background string `json:"background" yaml:"background"`
	Text       string `json:"text" yaml:"text"`
	Accent     string `json:"accent" yaml:"accent"`

	// these colors are currently unused
	// Headings          string `json:"headings" yaml:"headings"`
	// Links             string `json:"links" yaml:"links"`
	// ButtonsBackground string `json:"buttons_background" yaml:"buttons_background"`
	// ButtonsText       string `json:"buttons_text" yaml:"buttons_text"`
}

func (colors *ThemeColors) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, colors)
		return nil
	case string:
		json.Unmarshal([]byte(v), colors)
		return nil
	default:
		return fmt.Errorf("ThemeColors.Scan: Unsupported type: %T", v)
	}
}

func (colors *ThemeColors) Value() (driver.Value, error) {
	return json.Marshal(colors)
}

// supported pattern -> To
// /old -> /new
// /:year/:month/:post -> /:month/:year/:post
// /old/* -> /404
// Not supported yet, but wanted
// /:path* -> /new/:path
// /*.json -> /json
//
// inspiration: https://benhoyt.com/writings/go-routing/#split-switch
// type Redirect struct {
// 	To     string
// 	Status int
// }

// We need to use to_url for db otherwise Postgresql doesn't accept only "to"
type Redirect struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Pattern     string `db:"pattern" json:"pattern"`
	Domain      string `db:"domain" json:"domain"`
	PathPattern string `db:"path_pattern" json:"path_pattern"`
	To          string `db:"to_url" json:"to"`
	Status      int64  `db:"status" json:"status"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`
}

type Domain struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Hostname  string `db:"hostname" json:"hostname"`
	TlsActive bool   `db:"tls_active" json:"tls_active"`
	// TlsCertUpdatedAt *time.Time `db:"tls_cert_updated_at" json:"-"`
	// TlsCertNextUpdate *time.Time

	WebsiteID guid.GUID `db:"website_id" json:"-"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type CreateWebsiteInput struct {
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	OrganizationID guid.GUID `json:"organization_id"`
	Currency       *Currency `json:"currency"`
}

type GetWebsiteInput struct {
	ID        guid.GUID `json:"id"`
	Domains   bool      `json:"domains"`
	Redirects bool      `json:"redirects"`
}

type UpdateWebsiteInput struct {
	ID              guid.GUID          `json:"id"`
	Name            *string            `json:"name"`
	Description     *string            `json:"description"`
	Header          *string            `json:"header"`
	Footer          *string            `json:"footer"`
	Slug            *string            `json:"slug"`
	Navigation      *WebsiteNavigation `json:"navigation"`
	RobotsTxt       *string            `json:"robots_txt"`
	Blocked         *bool              `json:"blocked"`
	Currency        *Currency          `json:"currency"`
	BackgroundColor *string            `json:"background_color"`
	TextColor       *string            `json:"text_color"`
	AccentColor     *string            `json:"accent_color"`
	Theme           *string            `json:"theme"`
	Ad              *string            `json:"ad"`
	Announcement    *string            `json:"announcement"`
	Logo            *string            `json:"logo"`
	PoweredBy       *bool              `json:"powered_by"`
}

type DeleteWebsiteInput struct {
	ID guid.GUID `json:"id"`
}

type GetWebsitesForOrganizationInput struct {
	OrganizationID *guid.GUID `json:"organization_id"`
}

type ServeContentOutput struct {
	Status    int
	Data      io.ReadCloser
	Size      int64
	MediaType string
	Redirect  *Redirect
	Filename  string
}

type SaveRedirectsInput struct {
	WebsiteID guid.GUID       `json:"website_id"`
	Redirects []RedirectInput `json:"redirects"`
}

type RedirectInput struct {
	Pattern string `json:"pattern"`
	To      string `json:"to"`
	// Status  int
}

// type ParsedTheme struct {
// 	Name      string
// 	Templates map[string]*template.Template
// 	Assets    fs.FS
// }

type SetDomainAsPrimaryInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	// if null, the Markdown Ninja subdomain will be used
	Domain *string `json:"domain"`
}

type AddDomainInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	Hostname  string    `json:"hostname"`
	Primary   bool      `json:"primary"`
}

type RemoveDomainInput struct {
	ID guid.GUID `json:"id"`
}

type CheckTlsCertificateForDomainInput struct {
	DomainID guid.GUID `json:"domain_id"`
}

type ListWebsitesInput struct {
	Query string `json:"query"`
}

type UpdateWebsiteIconInput struct {
	WebsiteID guid.GUID
	Data      io.Reader
}

type AdminStatistics struct {
	Websites int64 `json:"websites"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Const
////////////////////////////////////////////////////////////////////////////////////////////////////

const DefaultRobotsTxt = `User-agent: *
Disallow: /


User-agent: Googlebot
Allow: /

User-agent: msnbot
Allow: /

User-Agent: Bingbot
Allow: /

User-Agent: search.marginalia.nu
Allow: /

User-Agent: DuckDuckBot
Allow: /

User-Agent: Kagibot
Allow: /

User-agent: Applebot
Allow: /



# non-exhaustive list of annoyances

User-agent: ia_archiver
Disallow: /

User-agent: archive.org_bot
Disallow: /

User-agent: GPTBot
Disallow: /

User-agent: Amazonbot
Disallow: /

User-agent: anthropic-ai
Disallow: /

User-agent: ChatGPT-User
Disallow: /

User-agent: ClaudeBot
Disallow: /

User-agent: Claude-Web
Disallow: /

User-agent: DataForSeoBot
Disallow: /

User-agent: magpie-crawler
Disallow: /

User-agent: PerplexityBot
Disallow: /
`

var WebsiteSlugBlocklist = set.NewFromSlice(websiteSlugBlocklist)

var websiteSlugBlocklist = []string{
	"admin",
	"administrator",
	"api",
	"app",
	"audio",
	"blog",
	"bot",
	"bots",
	"cdn",
	"community",
	"dev",
	"digest",
	"digests",
	"discover",
	"docs",
	"domain",
	"domains",
	"email",
	"emails",
	"example",
	"examples",
	"explore",
	"feed",
	"feeds",
	"forum",
	"home",
	"help",
	"localhost",
	"old",
	"legacy",
	"live",
	"mail",
	"mails",
	"next",
	"ninja",
	"origin",
	"page",
	"pages",
	"podcast",
	"production",
	"profile",
	"radio",
	"root",
	"search",
	"site",
	"sites",
	"staging",
	"stream",
	"streaming",
	"subscribe",
	"support",
	"test",
	"trending",
	"tv",
	"unsubscribe",
	"url",
	"video",
	"videos",
	"vod",
	"w",
	"website",
	"websites",
	"ww",
	"wwww",
	"wwwww",
	"xxx",
}
