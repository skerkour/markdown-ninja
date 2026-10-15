package middlewarex

import (
	"net/http"

	"markdown.ninja/stdx/httpx"
)

// StrictTransportSecurity sets the Strict-Transport-Security header to maxAge
// if maxAge is empty, it's set to 63072000
func StrictTransportSecurity(maxAge *string, includeSubDomains bool) func(next http.Handler) http.Handler {
	maxAgeKey := "max-age="
	headerValue := maxAgeKey

	if maxAge != nil {
		headerValue += *maxAge
	} else {
		headerValue += "63072000"
	}

	if includeSubDomains {
		headerValue += "; includeSubDomains"
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(httpx.HeaderStrictTransportSecurity, headerValue)
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
