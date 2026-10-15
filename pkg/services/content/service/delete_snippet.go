package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
)

func (service *ContentService) DeleteSnippet(ctx context.Context, input content.DeleteSnippetInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	snippet, err := service.repo.FindSnippetByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, snippet.WebsiteID)
	if err != nil {
		return
	}

	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.DeleteSnippet(ctx, tx, snippet.ID)
		if txErr != nil {
			return
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, snippet.WebsiteID, now)
		return txErr
	})
	return err
}
