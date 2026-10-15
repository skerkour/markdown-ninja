package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContentService) DeleteWebsiteData(ctx context.Context, tx db.Queryer, websiteID guid.GUID) (err error) {
	logger := slogx.FromCtx(ctx)

	job := queue.NewJobInput{
		Data: content.JobDeleteAssetsDataWithPrefix{
			Prefix: service.getStoragePrefixForWebsite(websiteID),
		},
		Timeout:    new(int64(600)),
		RetryDelay: new(int64(3600)),
	}
	err = service.queue.Push(ctx, tx, job)
	if err != nil {
		errMessage := "content.DeleteWebsiteAssets: Pushing DeleteAssetsWithPrefix job to queue"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	err = service.repo.DeleteWebsiteAssets(ctx, tx, websiteID)
	if err != nil {
		return
	}

	return
}
