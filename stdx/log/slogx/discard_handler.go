package slogx

import (
	"context"
	"log/slog"
)

type DiscardHandler struct {
}

// NewDiscardHandler returns a new DiscardHandler that do nothing and discards logs
func NewDiscardHandler() *DiscardHandler {
	return &DiscardHandler{}
}

func (handler *DiscardHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (handler *DiscardHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (handler *DiscardHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handler
}

func (handler *DiscardHandler) WithGroup(name string) slog.Handler {
	return handler
}
