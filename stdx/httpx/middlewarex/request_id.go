package middlewarex

import (
	"context"
	"net/http"

	"markdown.ninja/stdx/uuid"
)

type requestIDContextKey struct{}

// RequestIDCtxKey is the key that holds the unique request ID in a request context.
var RequestIDCtxKey = requestIDContextKey{}

func RequestID(header string) func(next http.Handler) http.Handler {
	isHeaderEmpty := header == ""
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.NewV7()

			if !isHeaderEmpty {
				w.Header().Set(header, requestID.String())
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, RequestIDCtxKey, requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}
