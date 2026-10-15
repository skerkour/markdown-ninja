package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContentService) DeleteAsset(ctx context.Context, input content.DeleteAssetInput) (err error) {
	assetToDelete, err := service.repo.FindAssetByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, assetToDelete.WebsiteID)
		if err != nil {
			return
		}
	} else {
		var website websites.Website
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			err = kernel.ErrPermissionDenied
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, assetToDelete.WebsiteID)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.deleteAssetInternal(ctx, tx, assetToDelete)
		return txErr
	})
	return err
}

func (service *ContentService) DeleteAssetInternal(ctx context.Context, tx db.Tx, assetID guid.GUID) (err error) {
	assetToDelete, err := service.repo.FindAssetByID(ctx, service.db, assetID)
	if err != nil {
		return
	}

	err = service.deleteAssetInternal(ctx, tx, assetToDelete)
	if err != nil {
		return
	}

	return
}

func (service *ContentService) deleteAssetInternal(ctx context.Context, tx db.Tx, assetToDelete content.Asset) (err error) {
	logger := slogx.FromCtx(ctx)

	if assetToDelete.Type == content.AssetTypeFolder && assetToDelete.Name == "assets" && assetToDelete.Folder == "/" {
		err = content.ErrCantDeleteTheAssetsFolder
		return
	}

	// if it's a folder we also need to delete its children
	if assetToDelete.Type == content.AssetTypeFolder {

		var children []content.Asset
		children, err = service.repo.FindAssetsAllChildren(ctx, tx, assetToDelete.WebsiteID, assetToDelete.Path())
		if err != nil {
			return
		}

		childrenIDs := make([]guid.GUID, len(children))

		for i, child := range children {
			childrenIDs[i] = child.ID

			if child.Type != content.AssetTypeFolder {
				// TODO: improve by reducing the number of queries
				job := queue.NewJobInput{
					Data: content.JobDeleteAssetData{
						StorageKey: service.getStorageKey(child),
					},
					// retry every 2 hours for 48 hours
					RetryDelay: new(int64(2 * 3600)),
					RetryMax:   new(int64(24)),
				}
				err = service.queue.Push(ctx, tx, job)
				if err != nil {
					errMessage := "content.DeleteAsset: Pushing DeleteFile job to queue for child"
					logger.Error(errMessage, slogx.Err(err))
					err = errs.Internal(errMessage, err)
					return
				}
			}
		}

		err = service.repo.DeleteAssets(ctx, tx, childrenIDs)
		if err != nil {
			return
		}
	} else {
		// TODO: batch jobs
		// on the other hand, if it's not a folder we need to delete the data
		job := queue.NewJobInput{
			Data: content.JobDeleteAssetData{
				StorageKey: service.getStorageKey(assetToDelete),
			},
			// retry every 2 hours for 48 hours
			RetryDelay: new(int64(2 * 3600)),
			RetryMax:   new(int64(24)),
		}
		err = service.queue.Push(ctx, tx, job)
		if err != nil {
			errMessage := "content.DeleteAsset: Pushing DeleteFile job to queue"
			logger.Error(errMessage, slogx.Err(err))
			err = errs.Internal(errMessage, err)
			return
		}
	}

	err = service.repo.DeleteAsset(ctx, tx, assetToDelete.ID)
	if err != nil {
		return
	}

	return
}
