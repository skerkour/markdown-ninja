package service

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

// findOrCreateFolder checks if the given folder exists
// if not, it will create all the required parent folder and the folder itself.
// It returns the found or created folder
func (service *ContentService) findOrCreateFolder(ctx context.Context, db db.Queryer, websiteID guid.GUID, folder string) (ret content.Asset, err error) {
	// err = service.validateAssetFolder(folder)
	// if err != nil {
	// 	return
	// }

	parent := filepath.Dir(folder)
	name := filepath.Base(folder)
	ret, err = service.repo.FindAssetByPath(ctx, db, websiteID, parent, name)
	if err == nil {
		if ret.Type != content.AssetTypeFolder {
			err = content.ErrAssetAlreadyExistsButIsNotAFolder(folder)
			return
		}

		return
	} else {
		if !errs.IsNotFound(err) {
			return
		}
		err = nil
	}

	// parent := filepath.Dir(folder)
	// name := filepath.Base(folder)
	// err = service.validateAssetFolderName(name)
	// if err != nil {
	// 	return
	// }

	folders := strings.Split(folder, "/")
	parentFolder := "/assets"
	now := time.Now().UTC()

	for i, folderName := range folders {
		if i <= 1 {
			// we don't need to create /assets
			continue
		}

		err = service.validateAssetFolderName(folderName)
		if err != nil {
			return
		}

		folderFullPath := filepath.Join(parentFolder, folderName)
		var existingAsset content.Asset
		existingAsset, err = service.repo.FindAssetByPath(ctx, db, websiteID, parentFolder, folderName)
		if err == nil {
			if existingAsset.Type != content.AssetTypeFolder {
				err = content.ErrAssetAlreadyExistsButIsNotAFolder(folderFullPath)
				return
			}
			// if asset folder already exists we don't need to do anything
			ret = existingAsset
		} else {
			if !errs.IsNotFound(err) {
				return
			}

			ret = content.Asset{
				ID:        guid.NewTimeBased(),
				CreatedAt: now,
				UpdatedAt: now,
				Type:      content.AssetTypeFolder,
				Name:      folderName,
				Folder:    parentFolder,
				MediaType: "",
				Size:      0,
				Hash:      []byte{},
				WebsiteID: websiteID,
				ProductID: nil,
			}
			err = service.repo.CreateAsset(ctx, db, ret)
			if err != nil {
				return
			}
		}
		parentFolder = folderFullPath
	}

	return
}
