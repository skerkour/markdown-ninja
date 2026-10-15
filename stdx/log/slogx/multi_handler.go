package slogx

import (
	"context"
	"log/slog"
)

// we do this to have a compile-time error if MultiHandler no longer satisfies the slog.handler interface
var _ slog.Handler = (*MultiHandler)(nil)

type MultiHandler struct {
	handlers []slog.Handler
}

func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{
		handlers: handlers,
	}
}

// Implements slog.Handler
func (mh *MultiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for i := range mh.handlers {
		if mh.handlers[i].Enabled(ctx, l) {
			return true
		}
	}

	return false
}

// Implements slog.Handler
func (mh *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for i := range mh.handlers {
		if mh.handlers[i].Enabled(ctx, r.Level) {
			err := mh.handlers[i].Handle(ctx, r.Clone())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Implements slog.Handler
func (mh *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	for i := range mh.handlers {
		mh.handlers[i] = mh.handlers[i].WithAttrs(attrs)
	}

	return mh
}

// Implements slog.Handler
func (mh *MultiHandler) WithGroup(name string) slog.Handler {
	for i := range mh.handlers {
		mh.handlers[i] = mh.handlers[i].WithGroup(name)
	}

	return mh
}
