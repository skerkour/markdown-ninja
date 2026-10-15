package middlewarex

import (
	"net/http"

	"markdown.ninja/stdx/httpx"
)

// var epoch = time.Unix(0, 0).UTC().Format(http.TimeFormat)

func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlNoCache)
		// w.Header().Set(httpx.HeaderExpires, epoch) // for Proxies

		next.ServeHTTP(w, r)
	})
}
