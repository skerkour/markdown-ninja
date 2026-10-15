package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/events/repository"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/pkg/settings"
	"markdown.ninja/stdx/ahocorasick"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

type Service struct {
	repo     repository.EventsRepository
	eventsDb db.DB
	db       db.DB
	queue    queue.Queue

	kernel          kernel.PrivateService
	websitesService websites.Service

	// TODO: Use a better concurrent data structure?
	eventsBuffer eventsBuffer
	botMatcher   *ahocorasick.Matcher

	// a random salt used to generate anonymousIDs
	// the salt is currently rotated daily
	anonymousIDSalt atomic.Pointer[string]
}

func NewService(ctx context.Context, db db.DB, eventsDb db.DB, queue queue.Queue, kernel kernel.PrivateService) (service *Service, err error) {
	repo := repository.NewEventsRepository()

	botMatcher := ahocorasick.NewStringMatcher(botsFingerprints)

	anonymousIDSaltSetting, err := settings.Get[events.SettingAnonymousIDSalt](ctx, db)
	if err != nil && errors.Is(err, settings.ErrSettingNotFound) {
		// if the setting doesn't exist yet, we create it and get it again
		_, err = service.rotateAnonymousIDSalt(ctx)
		if err != nil {
			return nil, fmt.Errorf("events: error rotating anonymousID salt: %w", err)
		}
		anonymousIDSaltSetting, err = settings.Get[events.SettingAnonymousIDSalt](ctx, db)
	} else if err != nil {
		return nil, fmt.Errorf("events: error getting anonymousID salt from DB: %w", err)
	}

	service = &Service{
		repo:     repo,
		eventsDb: eventsDb,
		db:       db,
		queue:    queue,

		kernel: kernel,

		eventsBuffer:    newEventsBuffer(),
		botMatcher:      botMatcher,
		anonymousIDSalt: atomic.Pointer[string]{},
	}
	service.anonymousIDSalt.Store(&anonymousIDSaltSetting.Salt)

	logger := slogx.FromCtx(ctx)
	service.flushEventsBufferInBackground(ctx, logger)
	go service.refreshAnonymousIDSaltInBackground(ctx)

	return
}

func (service *Service) InjectServices(websitesService websites.Service) {
	service.websitesService = websitesService
}

func (service *Service) refreshAnonymousIDSaltInBackground(ctx context.Context) {
	logger := slogx.FromCtx(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}

		settingValue, err := settings.Get[events.SettingAnonymousIDSalt](ctx, service.db)
		if err != nil {
			logger.Error("events: error refreshing anonymousID salt from database", slogx.Err(err),
				slog.String("setting.key", events.SettingAnonymousIDSalt{}.Key()))
			continue
		}

		if *service.anonymousIDSalt.Load() != settingValue.Salt {
			service.anonymousIDSalt.Store(&settingValue.Salt)
		}
	}
}
