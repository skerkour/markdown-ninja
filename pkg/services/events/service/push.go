package service

import (
	"context"
	"log/slog"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/log/slogx"
)

func (service *Service) Push(ctx context.Context, event events.Event) {
	logger := slogx.FromCtx(ctx)
	if event.Data == nil {
		logger.Error("events.Push: data is null", slog.String("event.type", event.Type.String()))
		return
	}

	service.eventsBuffer.Push(event)
}
