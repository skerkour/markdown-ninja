package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/log/slogx"
)

func (service *Service) TrackEmailSent(ctx context.Context, input events.TrackEmailSentInput) {
	go service.trackEmailSentInBackground(ctx, input)
}

func (service *Service) trackEmailSentInBackground(ctx context.Context, input events.TrackEmailSentInput) {
	logger := slogx.FromCtx(ctx)

	if input.FromAddress == "" {
		logger.Error("events.trackEmailSentInBackground: from_address is empty")
		return
	}

	if input.ToAddress == "" {
		logger.Error("events.trackEmailSentInBackground: to_address is empty")
		return
	}

	now := time.Now().UTC()
	data := events.EventDataEmailSent{
		FromAddress: input.FromAddress,
		ToAddress:   input.ToAddress,
	}
	event := events.Event{
		Time:         now,
		Type:         events.EventTypeEmailSent,
		Data:         data,
		WebsiteID:    input.WebsiteID,
		NewsletterID: input.NewsletterID,
	}

	service.eventsBuffer.Push(event)
}
