package middlewares

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"markdown.ninja/stdx/httpx"
)

// CleanupHostHeader cleans up Go's req.Host
// It's particularily useful for local development so that localhost:3000, localhost:3001 and localhost:8080
// will all map to the same host and use the same host router
func CleanupHostHeader() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, req *http.Request) {
			var err error
			var host string

			if host == "" {
				host = strings.ToLower(strings.TrimSpace(req.Host))
			}

			// don't allow access by IP address
			_, err = netip.ParseAddr(host)
			if err == nil {
				httpx.ServerErrorNotFound(w)
				return
			}

			// if host contains ":" we assume it's in the form of hostname:port
			// so we extract only the hostname
			if strings.Contains(host, ":") {
				host, _, err = net.SplitHostPort(host)
				if err != nil {
					httpx.ServerErrorInternal(w)
					return
				}
			}

			req.Host = host

			next.ServeHTTP(w, req)
		}

		return http.HandlerFunc(fn)
	}
}
