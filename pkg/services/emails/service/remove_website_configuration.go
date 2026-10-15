package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *EmailsService) RemoveWebsiteConfiguration(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error) {
	configuration, err := service.repo.FindWebsiteConfiguration(ctx, db, websiteID)
	if err != nil {
		return
	}
	logger := slogx.FromCtx(ctx)

	service.sendEmailCache.Delete(getWebsiteConfigurationCacheKey(websiteID))
	service.sendEmailCache.Delete(getSenderApiTokenCacheKey(websiteID))

	if configuration.FromDomain != "" {
		job := queue.NewJobInput{
			Data: emails.JobDeleteWebsiteConfigurationData{
				Domain: configuration.FromDomain,
			},
		}
		err = service.queue.Push(ctx, db, job)
		if err != nil {
			errMessage := "emails.RemoveWebsiteConfiguration: Pushing DeleteWebsiteData job to queue"
			logger.Error(errMessage, slogx.Err(err))
			err = errs.Internal(errMessage, err)
			return
		}

	}

	return
}
