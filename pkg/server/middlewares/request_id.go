package middlewares

import (
	"context"
	"net/http"

	"markdown.ninja/stdx/uuid"
)

type requestIDContextKey struct{}

// RequestIDCtxKey is the key that holds the unique request ID in a request context.
var RequestIDCtxKey = requestIDContextKey{}

func RequestID(headerName string) func(next http.Handler) http.Handler {
	if headerName == "" {
		panic("middlewares.RequestID: header name is empty")
	}
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.NewV7()

			w.Header().Set(headerName, requestID.String())

			ctx := r.Context()
			ctx = context.WithValue(ctx, RequestIDCtxKey, requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}
