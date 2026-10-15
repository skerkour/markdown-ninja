package service

import (
	"context"
	"io"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/storage"
	"markdown.ninja/stdx/retry"
)

func (service *ContentService) GetAssetData(ctx context.Context, asset content.Asset, options *content.GetAssetDataOptions) (ret io.ReadCloser, err error) {
	if asset.Type == content.AssetTypeFolder {
		return nil, content.ErrAssetIsAFolder(asset.Path())
	}
	getObjectOptions := storage.GetObjectOptions{}
	if options != nil {
		getObjectOptions.Range = options.Range
	}

	err = retry.Do(func() (retryErr error) {
		object, retryErr := service.storage.GetObject(ctx, service.getStorageKey(asset), &getObjectOptions)
		if retryErr != nil {
			if object != nil {
				// if there is an error, we close the object stream to avoid leaks
				object.Close()
			}
			return retryErr
		}

		ret = object
		return nil
	}, retry.Context(ctx), retry.Attempts(4), retry.Delay(15*time.Millisecond), retry.MaxDelay(100*time.Millisecond))
	if err != nil {
		return nil, err
	}

	return ret, nil
}
