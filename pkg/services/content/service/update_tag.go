package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
)

func (service *ContentService) UpdateTag(ctx context.Context, input content.UpdateTagInput) (tag content.Tag, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	tag, err = service.repo.FindTagByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, tag.WebsiteID)
	if err != nil {
		return
	}

	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	now := time.Now().UTC()

	err = service.validateTagName(name)
	if err != nil {
		return
	}

	err = service.validateTagDescription(description)
	if err != nil {
		return
	}

	if name != tag.Name {
		var existingTag content.Tag
		// chack that tag with same name doesn't already exists
		existingTag, err = service.repo.FindTagByName(ctx, service.db, tag.WebsiteID, name)
		if err == nil && !existingTag.ID.Equal(tag.ID) {
			err = content.ErrTagAlreadyExists(name)
		} else if err != nil {
			if errs.IsNotFound(err) {
				err = nil
			}
		}
		if err != nil {
			return
		}
	}

	tag.UpdatedAt = now
	tag.Name = name
	tag.Description = description

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.UpdateTag(ctx, tx, tag)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, tag.WebsiteID, now)
		return txErr
	})

	return
}
