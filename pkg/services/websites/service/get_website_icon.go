package service

import (
	"context"
	"io"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/pkg/storage"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/retry"
)

func (service *WebsitesService) GetWebsiteIcon(ctx context.Context, websiteID guid.GUID, size int) (icon io.ReadCloser, err error) {
	if !websites.WebsiteIconSizes.Contains(size) {
		return nil, errs.InvalidArgument("invalid icon size")
	}

	err = retry.Do(func() (retryErr error) {
		object, retryErr := service.storage.GetObject(ctx, generateStorageKeyForWebsiteIcon(websiteID, size), &storage.GetObjectOptions{})
		if retryErr != nil {
			if object != nil {
				// if there is an error, we close the object stream to avoid leaks
				object.Close()
			}
			return retryErr
		}

		icon = object
		return nil
	}, retry.Context(ctx), retry.Attempts(3), retry.Delay(20*time.Millisecond))
	if err != nil {
		return nil, err
	}

	return icon, nil
}
