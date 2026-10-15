package pingoo

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"
	"unicode/utf8"

	"markdown.ninja/pingoo-go/rules"
	"markdown.ninja/stdx/httpx"
)

type MiddlewareConfig struct {
	// serverless bool
	// geoip: local | http | cloudflare | disabled
	// rate limiting: memory | redis | (api | pingoo)
	// Rules
	// CdnProvider string
	Logging LoggingConfig
	Rules   []rules.Rule
}

type LoggingConfig struct {
	Disabled  bool
	GetLogger func(ctx context.Context) *slog.Logger
}

type MiddlewareOptionsHeaders struct {
}

// httpCtxKeyGeoip type to use when setting the Context
type httpCtxKeyGeoip struct{}

// CtxKey is the key that holds the unique Context in a request context.
var CtxKeyGeoip httpCtxKeyGeoip = httpCtxKeyGeoip{}

func (client *Client) Middleware(config *MiddlewareConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = &MiddlewareConfig{}
	}
	return func(nextMiddleware http.Handler) http.Handler {
		fn := func(res http.ResponseWriter, req *http.Request) {

			// we need to apply the response rules BEFORE forwarding to the other middlewares / response
			// handlers because otherwise the headers and body may have been already sent
			// TODO: we may wrap res so that we apply the response rules when WriteHeader is called
			for _, rule := range config.Rules {
				if rule.Match == nil || (rule.Match != nil && rule.Match(req)) {
					for _, action := range rule.Actions {
						action.Apply(res, req)
					}
				}
			}

			// userAgent := strings.TrimSpace(req.UserAgent())
			// var err error
			path := req.URL.Path
			ctx := req.Context()
			logger := client.getLogger(ctx)

			if len(userAgent) == 0 || len(userAgent) > 300 || !utf8.ValidString(userAgent) ||
				len(path) > 1024 || !utf8.ValidString(path) ||
				len(req.Method) > 20 {
				client.serveBlockedResponse(res)
				return
			}

			clientIpStr, _, err := net.SplitHostPort(req.RemoteAddr)
			if err != nil {
				logger.Error(fmt.Sprintf("pingoo.Middleware: RemoteAddr (%s) is not valid: %s", clientIpStr, err))
				clientIpStr = "0.0.0.0"
			}

			clientIp, err := netip.ParseAddr(clientIpStr)
			if err != nil {
				logger.Error(fmt.Sprintf("pingoo.Middleware: error parsing client IP [%s]: %s", clientIpStr, err))
				clientIp = netip.AddrFrom4([4]byte{0, 0, 0, 0})
			}

			geoipInfo, err := client.GeoipLookup(ctx, clientIp)
			if err != nil {
				logger.Error(fmt.Sprintf("pingoo.Middleware: looking up geoip information [%s]: %s", clientIpStr, err))
			}

			ctx = context.WithValue(ctx, CtxKeyGeoip, geoipInfo)
			req = req.WithContext(ctx)

			// if strings.HasPrefix(path, "/__pingoo/") {
			// 	client.handleHttpRequest(ctx, clientIp, res, req)
			// 	return
			// }

			// analyzeRequestInput := analyzeRequestInput{
			// 	HttpMethod:       req.Method,
			// 	Hostname:         req.Host,
			// 	UserAgent:        userAgent,
			// 	Ip:               clientIp.String(),
			// 	Asn:              geoipInfo.ASN,
			// 	Country:          geoipInfo.Country,
			// 	Path:             req.URL.Path,
			// 	HttpVersionMajor: int64(req.ProtoMajor),
			// 	HttpVersionMinor: int64(req.ProtoMinor),
			// 	Headers:          convertHttpheaders(req.Header),
			// }
			// analyzeRequestOutput, err := client.analyzeRequest(ctx, analyzeRequestInput)
			// if err != nil {
			// 	// fail open
			// 	logger.Error(err.Error(),
			// 		slog.String("user_agent", userAgent),
			// 		slog.String("ip_address", clientIp.String()),
			// 		slog.Int64("asn", geoipInfo.ASN),
			// 	)
			// 	nextMiddleware.ServeHTTP(res, req)
			// 	return
			// }

			// switch analyzeRequestOutput.Outcome {
			// case AnalyzeRequestOutcomeAllowed, AnalyzeRequestOutcomeVerifiedBot:
			// 	break
			// case AnalyzeRequestOutcomeChallenge:
			// 	break
			// // it's causing too much troubles
			// // 	req.URL.Path = "/__pingoo/challenge"
			// // 	client.handleHttpRequest(ctx, clientIp, res, req)
			// // 	return
			// case AnalyzeRequestOutcomeBlocked:
			// 	client.serveBlockedResponse(res)
			// 	return

			// default:
			// 	// fail open
			// 	logger.Error("pingoo.Middleware: unknown outcome",
			// 		slog.String("outcome", string(analyzeRequestOutput.Outcome)),
			// 		slog.String("ip", clientIp.String()),
			// 		slog.String("user_agent", userAgent),
			// 		slog.String("path", path),
			// 		slog.Int64("asn", geoipInfo.ASN),
			// 	)
			// }

			nextMiddleware.ServeHTTP(res, req)
		}
		return http.HandlerFunc(fn)
	}
}

func (client *Client) serveBlockedResponse(res http.ResponseWriter) {
	sleepForMs := rand.Int64N(500) + 1000
	time.Sleep(time.Duration(sleepForMs) * time.Millisecond)

	message := "Access denied\n"

	res.Header().Set(httpx.HeaderConnection, "close")
	res.Header().Del(httpx.HeaderETag)
	res.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoCache)
	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeTextUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(message)), 10))
	res.WriteHeader(http.StatusForbidden)
	res.Write([]byte(message))
}

func (client *Client) serveInternalError(res http.ResponseWriter) {
	sleepForMs := rand.Int64N(500) + 1000
	time.Sleep(time.Duration(sleepForMs) * time.Millisecond)

	message := "Internal Error. Please try again.\n"

	res.Header().Set(httpx.HeaderConnection, "close")
	res.Header().Del(httpx.HeaderETag)
	res.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoCache)
	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeTextUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(message)), 10))
	res.WriteHeader(http.StatusInternalServerError)
	res.Write([]byte(message))
}
