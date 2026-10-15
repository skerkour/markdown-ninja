package site

import (
	"html/template"
	"regexp"
	"time"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/guid"
)

const (
	// We use only digits because a significant amount of people in the world don't have latin keyboards
	AuthCodeAlphabet = "0123456789"
	// AuthCodeLength is the length in characters of the email verification code sent during subscription or login
	AuthCodeLength    = 8
	SignupMaxAttempts = 5
	LoginMaxAttempts  = 5

	// allows for file size up to 999,999,999,999 bytes
	RangeHeaderMaxSize = 31
)

var (
	AuthCodeHashParams = crypto.DefaultHashPasswordParams
	RangeHeaderRegexp  = regexp.MustCompile(`bytes=(\d+)-(\d*)`)
)

////////////////////////////////////////////////////////////////////////////////////////////////////
// Model
////////////////////////////////////////////////////////////////////////////////////////////////////

type MarkdowNinjaData struct {
	Country string            `json:"country"`
	Website Website           `json:"website"`
	Page    *Page             `json:"page"`
	Contact *contacts.Contact `json:"contact"`
}

type Website struct {
	Url          template.URL               `json:"url"`
	Name         string                     `json:"name"`
	Description  string                     `json:"description"`
	Navigation   websites.WebsiteNavigation `json:"navigation"`
	Language     string                     `json:"language"`
	Ad           *string                    `json:"ad"`
	Announcement *string                    `json:"announcement"`
	Colors       websites.ThemeColors       `json:"colors"`
	Logo         *string                    `json:"logo"`
	PoweredBy    bool                       `json:"powered_by"`
	Theme        string                     `json:"theme"`

	// TODO
	Header template.HTML `json:"-"`
	Footer template.HTML `json:"-"`
}

type PageMetadata struct {
	Date         time.Time        `json:"date"`
	ModifiedAt   time.Time        `json:"modified_at"`
	Type         content.PageType `json:"type"`
	Title        string           `json:"title"`
	Url          template.URL     `json:"url"`
	Path         string           `json:"path"`
	Description  string           `json:"description"`
	Language     string           `json:"language"`
	BodyHash     kernel.BytesHex  `json:"body_hash"`
	MetadataHash kernel.BytesHex  `json:"metadata_hash"`
}

type Page struct {
	PageMetadata
	Tags []Tag  `json:"tags"`
	Body string `json:"body"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Contact struct {
	Name                   string `json:"name"`
	Email                  string `json:"email"`
	SubscribedToNewsletter bool   `json:"subscribed_to_newsletter"`
}

// type ServeContentOutput struct {
// 	Status                       int
// 	Data                         io.ReadCloser
// 	Size                         int64
// 	MediaType                    string
// 	Redirect                     *websites.Redirect
// 	Filename                     string
// 	ContentDispositionAttachment bool
// }

type Order struct {
	ID        guid.GUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	TotalAmount int64             `json:"total_amount"`
	Currency    websites.Currency `json:"currency"`
	Status      store.OrderStatus `json:"status"`
	InvoiceUrl  *string           `json:"invoice_url"`
}

type Product struct {
	ID guid.GUID `json:"id"`

	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        store.ProductType `json:"type"`

	Content []ProductPage `json:"content"`
}

type ProductPage struct {
	ID       guid.GUID `json:"id"`
	Position int64     `json:"position"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type ListPagesInput struct {
	Tag  *string           `schema:"tag"`
	Type *content.PageType `schema:"type"`
}

type GetPageInput struct {
	Slug *string `schema:"slug"`
}

type ServeContentInput struct {
	Path string
}

type TrackEventPageViewInput struct {
	Path              string `json:"path"`
	HeaderReferrer    string `json:"header_referrer"`
	QueryParameterRef string `json:"query_parameter_ref"`
}

type LoginInput struct {
	Email string `json:"email"`
}

type CompleteLoginInput struct {
	SessionID guid.GUID `json:"session_id"`
	Code      string    `json:"code"`
}

type SubscribeInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CompleteSubscriptionInput struct {
	ContactID guid.GUID `json:"contact_id"`
	Code      string    `json:"code"`
}

type UnsubscribeInput struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

type LoginOutput struct {
	SessionID guid.GUID `json:"session_id"`
}

type SubscribeOutput struct {
	ContactID guid.GUID `json:"contact_id"`
}

type UpdateMyAccount struct {
	Email                  *string `json:"email"`
	Name                   *string `json:"name"`
	SubscribedToNewsletter *bool   `json:"subscribed_to_newsletter"`

	BillingAddress *kernel.Address `json:"billing_address"`
}

type ServeVideoIframeInput struct {
	AssetID guid.GUID
}

type ServePreviewInput struct {
	PageID guid.GUID
}

type GetProductInput struct {
	ProductID guid.GUID `schema:"id"`
}

type SearchInput struct {
	Query string `json:"query"`
}
