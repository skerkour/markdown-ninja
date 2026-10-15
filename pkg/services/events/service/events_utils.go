package service

import (
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/useragent"
)

type getAnonymousIdInput struct {
	time      time.Time
	websiteID guid.GUID
	IpAddress netip.Addr
	UserAgent string
	// Headers   http.Header
}

var botsFingerprints = []string{
	"bot",
	"crawl",
	"scrap",
	"spider",
	"spyder",
	"wget",
	"curl",
}

// getAnonymousID returns a 128 bits unique identifier for the given input.
// It should be very fast (currently around 300 nanoseconds).
func getAnonymousID(salt string, input getAnonymousIdInput) guid.GUID {
	// currently, the anoynmous identifier is the first 16 bytes (128 bits) of the BLAKE3 hash of
	// the input data.
	var hash [32]byte
	hasher := blake3.New()
	hasher.Write(input.websiteID.Bytes())
	hasher.Write([]byte(input.IpAddress.AsSlice()))
	hasher.Write([]byte(input.UserAgent))
	hasher.Write([]byte(salt))
	hasher.Sum(hash[:0])

	return guid.GUID(hash[:16])
}

// TODO: improve bot detection
// See https://whatmyuseragent.com/browser
func (service *Service) parseUserAgent(userAgent string) (browser events.Browser, os events.OperatingSystem, isBot bool) {
	if userAgent == "" || len(userAgent) > 256 {
		isBot = true
		return
	}

	userAgentLowercase := strings.ToLower(userAgent)
	if service.botMatcher.Contains([]byte(userAgentLowercase)) {
		isBot = true
		return
	}

	// userAgentLowercase := strings.ToLower(userAgent)
	// isBot = service.waf.IsBot(userAgentLowercase)
	// if isBot {
	// 	return
	// }

	ua := useragent.Parse(userAgent)
	if ua.Bot {
		isBot = true
		return
	}

	switch ua.OS {
	case useragent.Windows, useragent.WindowsPhone:
		os = events.OsWindows
	case useragent.ChromeOS:
		os = events.OsChromeOs
	case useragent.Linux:
		os = events.OsLinux
	case useragent.Android:
		os = events.OsAndroid
	case useragent.IOS:
		os = events.OsIos
	case useragent.MacOS:
		os = events.OsMacOs
	default:
		os = events.OsOther
	}

	switch ua.Name {
	case useragent.Chrome, "Chrome Mobile":
		browser = events.BrowserChrome
	case useragent.Safari:
		browser = events.BrowserSafari
	case useragent.Edge:
		browser = events.BrowserEdge
	case useragent.InternetExplorer:
		browser = events.BrowserInternetExplorer
	case useragent.Firefox:
		browser = events.BrowserFirefox
	case "SamsungBrowser", "SamsungInternet":
		browser = events.BrowserSamsungInternet
	case "Mobile DuckDuckGo", "DuckDuckGo":
		browser = events.BrowserDuckDuckGoPrivacyBrowser
	case useragent.Opera, useragent.OperaMini, useragent.OperaTouch:
		browser = events.BrowserOpera
	case "Brave":
		browser = events.BrowserBrave
	case "YaBrowser":
		browser = events.BrowserYandex
	case useragent.Vivaldi:
		browser = events.BrowserVivaldi
	default:
		browser = events.BrowserOther
	}

	if (os == events.OsOther && browser == events.BrowserOther) ||
		!strings.Contains(userAgentLowercase, "mozilla") {
		isBot = true
		return
	}

	return
}

func (service *Service) cleanCustomEventName(eventName string) string {
	if len(eventName) > events.CustomEventNameMaxSize {
		return eventName[:events.CustomEventNameMaxSize] + "..."
	}

	return eventName
}

func (service *Service) cleanupReferrer(websitePrimaryDomain, referrerHeader, refUrlQueryParam string) (referrer string) {
	referrer = referrerHeader
	if referrer == "" {
		// TODO: handle the (rare) situation where ?ref=not-a-domain
		referrer = refUrlQueryParam
	}
	if referrer != "" {
		referrerUrl, referrerUrlErr := url.Parse(referrer)
		if referrerUrlErr != nil {
			referrer = ""
		} else {
			referrer = referrerUrl.Hostname()
			referrer = strings.TrimPrefix(referrer, "www.")
		}
	}
	if websitePrimaryDomain != "" && referrer != "" && websitePrimaryDomain == referrer {
		referrer = ""
	}

	return
}
