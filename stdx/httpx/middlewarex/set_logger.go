package middlewarex

import (
	"log/slog"
	"net/http"

	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/uuid"
)

// SetLogger injects `logger` in the context of requests
func SetLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			routeLogger := logger.With(
				slog.Group("http", slog.String("method", req.Method), slog.String("path", req.URL.Path)),
			)

			reqIDContextValue := ctx.Value(RequestIDCtxKey)
			if requestID, ok := reqIDContextValue.(uuid.UUID); ok {
				routeLogger = routeLogger.With(slog.String("request_id", requestID.String()))
			}

			ctx = slogx.ToCtx(ctx, routeLogger)
			req = req.WithContext(ctx)

			next.ServeHTTP(w, req)
		}
		return http.HandlerFunc(fn)
	}
}
