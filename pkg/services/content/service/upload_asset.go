package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/pkg/storage"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) UploadAsset(ctx context.Context, input content.UploadAssetInput, bypassAuthCheck bool) (asset content.Asset, err error) {
	var website websites.Website

	if !bypassAuthCheck {
		actorID, err := service.kernel.CurrentUserID(ctx)
		if err == nil {
			err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
			if err != nil {
				return asset, err
			}

			website, err = service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
			if err != nil {
				return asset, err
			}
		} else {
			httpCtx := httpctx.FromCtx(ctx)
			if httpCtx.ApiKey == nil {
				return asset, kernel.ErrPermissionDenied
			}

			website, err = service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
			if err != nil {
				return asset, err
			}

			_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
			if err != nil {
				return asset, err
			}
		}
	}

	// TODO: clean and validate input
	now := time.Now().UTC()
	filename := strings.TrimSpace(input.Name)

	err = service.validateAssetFileName(filename)
	if err != nil {
		return
	}

	if input.ProductID != nil {
		var product store.Product

		product, err = service.storeService.FindProduct(ctx, service.db, *input.ProductID)
		if err != nil {
			return
		}

		if !input.WebsiteID.Equal(product.WebsiteID) {
			err = store.ErrProductNotFound
			return
		}

		// check if filename is not already in use
		// TODO: improve, use databse index instead of loop...
		var productAssets []content.Asset
		productAssets, err = service.repo.FindProductAssets(ctx, service.db, product.ID)
		if err != nil {
			return
		}
		for _, existingAsset := range productAssets {
			if filename == existingAsset.Name {
				err = store.ErrAssetFilnameAlreadyInUse(filename)
				return
			}
		}
	}

	// Create file
	folder := ""
	// folder is always empty for products' assets
	if input.ProductID == nil {
		if input.Folder != nil {
			folder = strings.TrimSpace(*input.Folder)
			err = service.validateAssetFolder(folder)
			if err != nil {
				return
			}
		} else {
			folder = service.generateDefaultAssetFolder()
		}

		// check that parent exists or create it
		_, err = service.findOrCreateFolder(ctx, service.db, input.WebsiteID, folder)
		if err != nil {
			return
		}

		// check that asset doesn't alread yexist
		_, err = service.repo.FindAssetByPath(ctx, service.db, input.WebsiteID, folder, filename)
		if err != nil {
			if errs.IsNotFound(err) {
				err = nil
			} else {
				return
			}
		} else {
			err = content.ErrAssetAlreadyExists(filepath.Join(folder, filename))
			if err != nil {
				return
			}
		}

		// _, err = service.mkdirAll(ctx, service.db, input.WebsiteID, folder)
		// if err != nil {
		// 	return
		// }
	}

	asset = content.Asset{
		ID:        guid.NewTimeBased(),
		CreatedAt: now,
		UpdatedAt: now,
		Type:      content.AssetTypeFile,
		Name:      filename,
		Folder:    folder,
		MediaType: "",
		Size:      0,
		Hash:      nil,
		WebsiteID: input.WebsiteID,
		ProductID: input.ProductID,
	}

	assetHasher := blake3.New()
	assetHasherForS3Integrity := sha256.New()
	inputDataHasherReader := io.TeeReader(input.Data, assetHasher)
	asset.Size, err = io.CopyN(assetHasherForS3Integrity, inputDataHasherReader, kernel.MaxAssetSize+1)
	if err == nil && asset.Size != kernel.MaxAssetSize {
		err = content.ErrAssetIsTooLarge(kernel.MaxAssetSize)
		return
	} else if err != nil && err != io.EOF {
		err = fmt.Errorf("content.UploadAsset: writing data to tmp file: %w", err)
		return
	}
	err = nil

	asset.Hash = assetHasher.Sum(nil)
	assetSha256 := assetHasherForS3Integrity.Sum(nil)

	_, err = input.Data.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("content.UploadAsset: seeking(0) tmp file (1st): %w", err)
		return
	}

	detectMediaTypeBuffer := bytes.NewBuffer(make([]byte, 512))
	_, err = io.CopyN(detectMediaTypeBuffer, input.Data, 512)
	if err != nil && err != io.EOF {
		err = fmt.Errorf("content.UploadAsset: Reading data to media type buffer: %w", err)
		return
	}
	err = nil

	asset.MediaType = service.DetectMimeType(ctx, filename, detectMediaTypeBuffer.Bytes())
	if strings.HasPrefix(asset.MediaType, "image") {
		asset.Type = content.AssetTypeImage
	} else if strings.HasPrefix(asset.MediaType, "audio") {
		asset.Type = content.AssetTypeAudio
	} else if strings.HasPrefix(asset.MediaType, "video") {
		asset.Type = content.AssetTypeVideo
	} else {
		asset.Type = content.AssetTypeFile
	}

	err = service.organizationsService.CheckBillingGatedAction(ctx, service.db, website.OrganizationID, organizations.BillingGatedActionUploadAsset{
		NewAssetSize: asset.Size,
		WebsiteID:    website.ID,
		AssetType:    asset.Type,
	})
	if err != nil {
		return
	}

	// upload asset to storage
	_, err = input.Data.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("content.UploadAsset: seeking(0) tmp file (2nd): %w", err)
		return
	}

	storageKey := service.getStorageKey(asset)
	putObjectOptions := &storage.PutObjectOptions{
		HashSha256: assetSha256,
	}
	err = service.storage.PutObject(ctx, storageKey, asset.Size, input.Data, putObjectOptions)
	if err != nil {
		err = fmt.Errorf("content.UploadAsset: uploading file to storage: %w", err)
		return
	}

	// TODO: if the HTTP request fails now, there will be an asset in the storage, but not in the
	// database. A form of garbage collection is needed to avoid accumulating useless blobs.
	// Another solution is to use a kind of 2-phases commit: create a file with status = uploading
	// and garbage collect these files instead of scanning the storage.

	err = service.repo.CreateAsset(ctx, service.db, asset)
	if err != nil {
		return
	}

	return
}
