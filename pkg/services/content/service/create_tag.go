package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) CreateTag(ctx context.Context, input content.CreateTagInput) (tag content.Tag, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}
	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
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

	// chack that tag with same name doesn't already exists
	_, err = service.repo.FindTagByName(ctx, service.db, input.WebsiteID, name)
	if err == nil {
		err = content.ErrTagAlreadyExists(name)
	} else {
		if errs.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		return
	}

	tag = content.Tag{
		ID:          guid.NewTimeBased(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Name:        name,
		Description: description,
		WebsiteID:   input.WebsiteID,
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreateTag(ctx, tx, tag)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, tag.WebsiteID, now)
		return txErr
	})

	return
}
