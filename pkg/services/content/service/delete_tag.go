package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
)

func (service *ContentService) DeleteTag(ctx context.Context, input content.DeleteTagInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	tag, err := service.repo.FindTagByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, tag.WebsiteID)
	if err != nil {
		return
	}

	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.DeleteTag(ctx, tx, tag.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, tag.WebsiteID, now)
		return txErr
	})

	return err
}
