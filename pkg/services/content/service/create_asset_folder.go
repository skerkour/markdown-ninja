package service

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) CreateAssetFolder(ctx context.Context, input content.CreateAssetFolderInput) (folder content.Asset, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
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

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	parentFolder := strings.TrimSpace(input.Folder)
	err = service.validateAssetFolder(parentFolder)
	if err != nil {
		return
	}

	name := strings.TrimSpace(input.Name)
	err = service.validateAssetFolderName(name)
	if err != nil {
		return
	}

	// check that parent exists or create it
	_, err = service.findOrCreateFolder(ctx, service.db, input.WebsiteID, parentFolder)
	if err != nil {
		return
	}

	// check that folder doesn't alread yexist
	_, err = service.repo.FindAssetByPath(ctx, service.db, input.WebsiteID, parentFolder, name)
	if err != nil {
		if errs.IsNotFound(err) {
			err = nil
		} else {
			return
		}
	} else {
		err = content.ErrAssetAlreadyExists(filepath.Join(parentFolder, name))
		if err != nil {
			return
		}
	}

	now := time.Now().UTC()
	folder = content.Asset{
		ID:        guid.NewTimeBased(),
		CreatedAt: now,
		UpdatedAt: now,
		Type:      content.AssetTypeFolder,
		Name:      name,
		Folder:    parentFolder,
		MediaType: "",
		Size:      0,
		Hash:      []byte{},
		WebsiteID: input.WebsiteID,
		ProductID: nil,
	}
	err = service.repo.CreateAsset(ctx, service.db, folder)
	if err != nil {
		return
	}

	return
}
