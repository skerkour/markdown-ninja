package slogx

import (
	"context"
	"log/slog"
	"os"
)

type contextKey struct{}

var ctxKey = contextKey{}

// ToCtx returns a copy of ctx with logger associated.
func ToCtx(ctx context.Context, logger *slog.Logger) context.Context {
	// if existingLogger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
	// 	if existingLogger == logger {
	// 		// Do not store same logger.
	// 		return ctx
	// 	}
	// }
	return context.WithValue(ctx, ctxKey, logger)
}

// FromCtx returns the Logger associated with the ctx. If no logger
// is associated, a New() logger is returned with a addedfield "slogx.FromCtx": "error".
//
// For example, to add a field to an existing logger in the context, use this
// notation:
//
//	ctx := r.Context()
//	logger := slogx.FromCtx(ctx)
//	logger = logger.With(...)
func FromCtx(ctx context.Context) *slog.Logger {
	if existingLogger, ok := ctx.Value(ctxKey).(*slog.Logger); ok {
		return existingLogger
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(slog.String("slogx.FromCtx", "error"))
	return logger
}
