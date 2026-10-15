package service

import (
	"context"
	"path/filepath"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/log/slogx"
)

func (service *ContentService) GetAsset(ctx context.Context, input content.GetAssetInput) (asset content.Asset, err error) {
	logger := slogx.FromCtx(ctx)
	if input.ID == nil && input.Path == nil {
		logger.Error("content.GetAsset: Both ID and path are null")
		err = content.ErrAssetNotFound
		return
	} else if input.ID != nil && input.Path != nil {
		logger.Error("content.GetAsset: Both ID and path are NOT null")
		err = content.ErrAssetNotFound
		return
	}

	if input.ID != nil {
		asset, err = service.repo.FindAssetByID(ctx, service.db, *input.ID)
		if err != nil {
			return
		}
	} else {
		if input.WebsiteID == nil || input.Path == nil {
			logger.Error("content.GetAsset: Both websiteID or path are null")
			err = content.ErrAssetNotFound
			return
		}

		parentFolder := filepath.Dir(*input.Path)
		fileName := filepath.Base(*input.Path)

		asset, err = service.repo.FindAssetByPath(ctx, service.db, *input.WebsiteID, parentFolder, fileName)
		if err != nil {
			return
		}
	}

	return
}
