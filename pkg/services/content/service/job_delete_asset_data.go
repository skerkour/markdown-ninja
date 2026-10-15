package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/retry"
)

func (service *ContentService) JobDeleteAssetData(ctx context.Context, input content.JobDeleteAssetData) (err error) {
	err = retry.Do(func() error {
		return service.storage.DeleteObject(ctx, input.StorageKey)
	}, retry.Context(ctx), retry.Attempts(20), retry.Delay(1*time.Minute))

	return
}
