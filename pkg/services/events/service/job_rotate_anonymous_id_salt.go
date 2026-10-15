package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/settings"
	"markdown.ninja/stdx/log/slogx"
)

func (service *Service) JobRotateAnonymousIDSalt(ctx context.Context, input events.JobRotateAnonymousIDSalt) error {
	_, err := service.rotateAnonymousIDSalt(ctx)
	return err
}

func (service *Service) rotateAnonymousIDSalt(ctx context.Context) (string, error) {
	logger := slogx.FromCtx(ctx)

	var salt [32]byte
	rand.Read(salt[:])

	setting := events.SettingAnonymousIDSalt{
		Salt: base64.RawURLEncoding.EncodeToString(salt[:]),
	}

	err := settings.Set(ctx, service.db, setting)
	if err != nil {
		return "", err
	}

	logger.Info("events: anonymousID salt successfully rotated")

	return setting.Salt, nil
}
