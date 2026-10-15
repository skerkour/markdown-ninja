package httpx

const (
	HeaderLastModified            = "Last-Modified"
	HeaderContentType             = "Content-Type"
	HeaderContentEncoding         = "Content-Encoding"
	HeaderCacheControl            = "Cache-Control"
	HeaderContentLength           = "Content-Length"
	HeaderConnection              = "Connection"
	HeaderETag                    = "ETag"
	HeaderServer                  = "Server"
	HeaderIfModifiedSince         = "If-Modified-Since"
	HeaderExpires                 = "Expires"
	HeaderIfNoneMatch             = "If-None-Match"
	HeaderAccept                  = "Accept"
	HeaderUserAgent               = "User-Agent"
	HeaderAuthorization           = "Authorization"
	HeaderAltSvc                  = "Alt-Svc"
	HeaderStrictTransportSecurity = "Strict-Transport-Security"
	HeaderContentDisposition      = "Content-Disposition"
	HeaderContentRange            = "Content-Range"
	HeaderReferer                 = "Referer"
	HeaderDoNotTrack              = "Dnt"
	HeaderRange                   = "Range"
	HeaderAcceptLanguage          = "Accept-Language"
	HeaderAcceptRanges            = "Accept-Ranges"
)

// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Caching
const (
	CacheControlNoCache = "private, no-cache, no-store, must-revalidate" // "no-cache, no-store, no-transform, must-revalidate, private, max-age=0"
	CacheControlDynamic = "public, no-cache, must-revalidate"            // TODO https://web.dev/http-cache/, https://web.dev/love-your-cache/
	// CacheControlDynamic    = "max-age=0, must-revalidate, public" // TODO https://web.dev/http-cache/, https://web.dev/love-your-cache/
	// CacheControlNoCache       = "no-cache, no-store, no-transform, must-revalidate" // "no-cache, no-store, no-transform, must-revalidate, private, max-age=0"
	CacheControl30Seconds = "public, max-age=30, must-revalidate"
	CacheControl1Minute   = "public, max-age=60, must-revalidate"
	CacheControl5Minutes  = "public, max-age=300, stale-while-revalidate=30"
	CacheControl10Minutes = "public, max-age=600, stale-while-revalidate=60"
	CacheControl15Minutes = "public, max-age=900, stale-while-revalidate=90"
	CacheControl30Minutes = "public, max-age=1800, stale-while-revalidate=180"
	CacheControl1Hour     = "public, max-age=3600, stale-while-revalidate=360"
	CacheControl1Day      = "public, max-age=86400, stale-while-revalidate=600"
	CacheControl1Week     = "public, max-age=604800, stale-while-revalidate=600"
	CacheControl1Month    = "public, max-age=2592000, stale-while-revalidate=600"
	CacheControlImmutable = "public, max-age=31536000, immutable"
)

const (
	CacheControl30SecondsCdnOnly = "public, max-age=0, s-max-age=30, must-revalidate"
)

const (
	MediaTypeText = "text/plain"
	MediaTypeXml  = "application/xml"
	MediaTypeJson = "application/json"

	MediaTypeHtmlUtf8 = "text/html; charset=utf-8"
	MediaTypeTextUtf8 = "text/plain; charset=utf-8"

	MediaTypeJsonFeed = "application/feed+json"
	MediaTypeRSS      = "application/rss+xml"
	MediaTypeAtom     = "application/atom+xml"

	MediaTypeAudio = "audio"
	MediaTypeVideo = "video"
	MediaTypePNG   = "image/png"
	MediaTypeJPEG  = "image/jpeg"
	MediaTypeSVG   = "image/svg+xml"

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
	// alternative value from the 'file' command line program: text/x-shellscript
	MediaTypeShellScript = "application/x-sh"
)

const (
	AcceptRangesBytes = "bytes"
)
