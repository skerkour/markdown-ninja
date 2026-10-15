package events

import (
	"fmt"
	"time"

	"markdown.ninja/stdx/guid"
)

type Browser int32
type OperatingSystem int32

const (
	CustomEventNameMaxSize = 42
)

const (
	OsOther OperatingSystem = iota
	OsLinux
	OsMacOs
	OsWindows
	OsAndroid
	OsIos
	OsChromeOs
)

// MarshalText implements encoding.TextMarshaler.
func (os OperatingSystem) MarshalText() (ret []byte, err error) {
	switch os {
	case OsLinux:
		ret = []byte("Linux")
	case OsMacOs:
		ret = []byte("macOS")
	case OsWindows:
		ret = []byte("Windows")
	case OsAndroid:
		ret = []byte("Android")
	case OsIos:
		ret = []byte("iOS")
	case OsChromeOs:
		ret = []byte("ChromeOS")
	default:
		ret = []byte("other")
	}

	return ret, nil
}

func (os OperatingSystem) String() string {
	ret, _ := os.MarshalText()
	return string(ret)
}

func (os *OperatingSystem) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case "Linux":
		*os = OsLinux
	case "macOS":
		*os = OsMacOs
	case "Windows":
		*os = OsWindows
	case "Android":
		*os = OsAndroid
	case "iOS":
		*os = OsIos
	case "ChromeOS":
		*os = OsChromeOs
	default:
		*os = OsOther
		err = fmt.Errorf("Unknown OperatingSystem: %s", string(data))
	}

	return
}

const (
	BrowserOther Browser = iota
	BrowserChrome
	BrowserSafari
	BrowserFirefox
	BrowserEdge
	BrowserOpera
	BrowserInternetExplorer
	BrowserSamsungInternet
	BrowserBrave
	BrowserDuckDuckGoPrivacyBrowser
	BrowserYandex
	BrowserVivaldi
)

// MarshalText implements encoding.TextMarshaler.
func (browser Browser) MarshalText() (ret []byte, err error) {
	switch browser {
	case BrowserOther:
		ret = []byte("other")
	case BrowserChrome:
		ret = []byte("Chrome")
	case BrowserSafari:
		ret = []byte("Safari")
	case BrowserFirefox:
		ret = []byte("Firefox")
	case BrowserEdge:
		ret = []byte("Edge")
	case BrowserOpera:
		ret = []byte("Opera")
	case BrowserInternetExplorer:
		ret = []byte("Internet Explorer")
	case BrowserSamsungInternet:
		ret = []byte("Samsung Internet")
	case BrowserBrave:
		ret = []byte("Brave")
	case BrowserDuckDuckGoPrivacyBrowser:
		ret = []byte("DuckDuckGo Privacy Browser")
	case BrowserYandex:
		ret = []byte("Yandex Browser")
	case BrowserVivaldi:
		ret = []byte("Vivaldi")
	default:
		ret = []byte("other")
		err = fmt.Errorf("Unknown Browser: %d", browser)
	}
	return
}

func (browser Browser) String() string {
	ret, _ := browser.MarshalText()
	return string(ret)
}

// func (browser Browser) MarshalJSON() (ret []byte, err error) {
// 	return browser.MarshalText()
// }

// UnmarshalText implements encoding.TextUnmarshaler.
func (browser *Browser) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case "other":
		*browser = BrowserOther
	case "Chrome":
		*browser = BrowserChrome
	case "Safari":
		*browser = BrowserSafari
	case "Firefox":
		*browser = BrowserFirefox
	case "Edge":
		*browser = BrowserEdge
	case "Opera":
		*browser = BrowserOpera
	case "Internet Explorer":
		*browser = BrowserInternetExplorer
	case "Samsung Internet":
		*browser = BrowserSamsungInternet
	case "Brave":
		*browser = BrowserBrave
	case "DuckDuckGo Privacy Browser":
		*browser = BrowserDuckDuckGoPrivacyBrowser
	case "Yandex Browser":
		*browser = BrowserYandex
	case "Vivaldi":
		*browser = BrowserVivaldi
	default:
		*browser = BrowserOther
		err = fmt.Errorf("Unknown Browser: %s", string(data))
	}
	return nil
}

// func (browser *Browser) UnmarshalJSON(data []byte) (err error) {
// 	return browser.UnmarshalText(data)
// }

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

// it's better to put the most-commonly used fields directly into the event struct than in the data JSON
// field.
// https://www.heap.io/blog/when-to-avoid-jsonb-in-a-postgresql-schema

// the number of columns in database that the Event entity has.
// Used when batching inserts
const EventDatabaseColumns = 13

type Event struct {
	Time time.Time `db:"time" json:"time"`

	Type EventType `db:"type" json:"type"`

	// Name     *string   `db:"name"`
	// Referrer string    `db:"referrer"`
	// // or Path or Url
	// Page        string          `db:"page"`
	// CountryCode string          `db:"country_code"`
	// Browser     Browser         `db:"browser"`
	// OS          OperatingSystem `db:"os"`
	// // name of the event. null if it's a pageView

	Data any `db:"data" json:"data"`

	Path            *string          `db:"path" json:"path"`
	Country         *string          `db:"country" json:"country"`
	Browser         *Browser         `db:"browser" json:"browser"`
	OperatingSystem *OperatingSystem `db:"operating_system" json:"operating_system"`
	Referrer        *string          `db:"referrer" json:"referrer"`

	WebsiteID    guid.GUID  `db:"website_id" json:"website_id"`
	AnonymousID  *guid.GUID `db:"anonymous_id" json:"anonymous_id"`
	OrderID      *guid.GUID `db:"order_id" json:"order_id"`
	NewsletterID *guid.GUID `db:"newsletter_id" json:"newsletter_id"`
}

// Query parameters:
// ref
// UTM Medium
// UTM Source
// UTM Campaign
// UTM Content
// UTM Term

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type TrackPageViewInput struct {
	WebsitePrimaryDomain string
	Path                 string
	IpAddress            string
	HeaderReferrer       string
	HeaderUserAgent      string
	QueryParameterRef    string
	IsTor                bool

	WebsiteID guid.GUID
}

type TrackEmailSentInput struct {
	FromAddress string
	ToAddress   string

	WebsiteID    guid.GUID
	NewsletterID *guid.GUID
}

type TrackSubscribedToNewsletterInput struct {
	WebsiteID guid.GUID
}

type TrackUnsubscribedFromNewsletterInput struct {
	WebsiteID guid.GUID
}

type TrackOrderPlacedInput struct {
	UserAgent string
	Country   string
	OrderID   guid.GUID
	WebsiteID guid.GUID
}

type TrackOrderCompletedInput struct {
	OrderID     guid.GUID
	WebsiteID   guid.GUID
	TotalAmount int64
}

type TrackOrderCanceledInput struct {
	OrderID     guid.GUID
	WebsiteID   guid.GUID
	TotalAmount int64
	Country     string
}

// type TrackEventInput struct {
// 	WebsiteID guid.GUID
// 	ContactID *guid.GUID
// 	Data      any
// WebsitePrimaryDomain string
// CustomEventName      *string
// Url                  *url.URL
// Path                 string
// IpAddress            string
// Referrer             *string
// Headers              http.Header
// }

type GetAnalyticsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type AnalyticsData struct {
	TotalPageViews int64                    `json:"total_page_views"`
	PageViews      []Counter                `json:"page_views"`
	TotalVisitors  int64                    `json:"total_visitors"`
	Visitors       []Counter                `json:"visitors"`
	Pages          []Counter                `json:"pages"`
	Referrers      []Counter                `json:"referrers"`
	Countries      []Counter                `json:"countries"`
	Browsers       []CounterBrowser         `json:"browsers"`
	OSes           []CounterOperatingSystem `json:"oses"`
	NewSubscribers int64                    `json:"new_subscribers"`
}

type Counter struct {
	Label string `db:"label" json:"label"`
	Count int64  `db:"count" json:"count"`
}

type PageViewsAndVisitors struct {
	Day       time.Time `db:"day"`
	PageViews int64     `db:"page_views"`
	Visitors  int64     `db:"visitors"`
}

type CounterOperatingSystem struct {
	Label OperatingSystem `db:"label" json:"label"`
	Count int64           `db:"count" json:"count"`
}

type CounterBrowser struct {
	Label Browser `db:"label" json:"label"`
	Count int64   `db:"count" json:"count"`
}
