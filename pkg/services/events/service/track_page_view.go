package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/log/slogx"
)

func (service *Service) TrackPageView(ctx context.Context, input events.TrackPageViewInput) {
	go service.trackPageViewInBackground(ctx, input)
}

// TODO: a way to detect bots would be to detect all anonymous IDs that tried to access special pages such
// as robots.txt, sitemap.xml, feed.rss ...
func (service *Service) trackPageViewInBackground(ctx context.Context, input events.TrackPageViewInput) {
	logger := slogx.FromCtx(ctx)
	now := time.Now().UTC()
	path := strings.TrimSpace(input.Path)
	userAgent := strings.TrimSpace(input.HeaderUserAgent)
	websitePrimaryDomain := input.WebsitePrimaryDomain
	headerReferrer := strings.ToLower(strings.TrimSpace(input.HeaderReferrer))
	queryParameterRef := strings.ToLower(strings.TrimSpace(input.QueryParameterRef))
	var referrer string
	httpCtx := httpctx.FromCtx(ctx)

	if path == "" {
		logger.Error("events.trackPageViewInBackground: path is empty")
		return
	}

	browser, os, isBot := service.parseUserAgent(userAgent)
	if isBot {
		return
	}

	getAnonymousIdInput := getAnonymousIdInput{
		time:      now,
		websiteID: input.WebsiteID,
		IpAddress: httpCtx.Client.IP,
		UserAgent: userAgent,
	}
	anonymousId := getAnonymousID(*service.anonymousIDSalt.Load(), getAnonymousIdInput)

	referrer = service.cleanupReferrer(websitePrimaryDomain, headerReferrer, queryParameterRef)

	event := events.Event{
		Time: now,
		Type: events.EventTypePageView,
		Data: events.EventDataPageView{},

		Referrer:        &referrer,
		Path:            &path,
		Country:         &httpCtx.Client.CountryCode,
		Browser:         &browser,
		OperatingSystem: &os,

		WebsiteID:   input.WebsiteID,
		AnonymousID: &anonymousId,
	}

	service.eventsBuffer.Push(event)
}
