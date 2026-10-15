package middlewares

import (
	"net/http"

	"markdown.ninja/stdx/httpx"
)

func AddAltSvcHeader() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(res http.ResponseWriter, req *http.Request) {
			res.Header().Set(httpx.HeaderAltSvc, `h3=":443"; ma=86400`)
			next.ServeHTTP(res, req)
		}

		return http.HandlerFunc(fn)
	}
}
