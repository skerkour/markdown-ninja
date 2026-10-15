package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
)

func (service *ContentService) DeletePage(ctx context.Context, input content.DeletePageInput) (err error) {
	var page content.Page
	httpCtx := httpctx.FromCtx(ctx)

	page, err = service.repo.FindPageByID(ctx, service.db, input.PageID)
	if err != nil {
		return err
	}

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, page.WebsiteID)
		if err != nil && !httpCtx.AccessToken.IsAdmin {
			return err
		}
	} else {
		var website websites.Website
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			err = kernel.ErrPermissionDenied
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, page.WebsiteID)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	now := time.Now().UTC()

	if page.Path == "/" {
		err = content.ErrHomepageCantBeDeleted
		return err
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.DeletePage(ctx, tx, page.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, page.WebsiteID, now)
		return txErr
	})
	return err
}
