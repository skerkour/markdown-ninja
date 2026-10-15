package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/guid"
)

const ASSETS_DIR = "assets"

type localAsset struct {
	Path string
	Hash []byte
}

func (client *Client) uploadWebsiteAssets(ctx context.Context, websiteID guid.GUID, sync bool) (err error) {
	directoryInfo, err := os.Stat(ASSETS_DIR)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			client.logger.Debug("assets: assets directory does not exist. Skipping.")
			err = nil
			return
		}
		err = fmt.Errorf("assets: error getting assets folder info: %w", err)
		return
	}
	if !directoryInfo.IsDir() {
		err = errors.New("assets: assets is not a folder")
		return
	}

	websiteAssets, err := client.apiClient.ListAssets(ctx, content.ListAssetsInput{WebsiteID: websiteID})
	if err != nil {
		err = fmt.Errorf("assets: error fetching website assets: %w", err)
		return
	}
	websiteAssetsByPath := make(map[string]content.Asset, len(websiteAssets))
	for _, asset := range websiteAssets {
		websiteAssetsByPath[asset.Path()] = asset
	}

	localAssets, err := client.loadLocalAssets(ASSETS_DIR)
	if err != nil {
		return
	}

	for _, localAsset := range localAssets {
		uploadLocalAsset := false
		websiteAsset, existsRemote := websiteAssetsByPath["/"+localAsset.Path]
		if existsRemote {
			if !bytes.Equal(websiteAsset.Hash, localAsset.Hash) {
				deleteAssetInput := content.DeleteAssetInput{
					ID: websiteAsset.ID,
				}
				deleteAssetErr := client.apiClient.DeleteAsset(ctx, deleteAssetInput)
				if deleteAssetErr != nil {
					client.logger.Error(fmt.Sprintf("assets: error deleting website asset %s: %s", websiteAsset.Path(), deleteAssetErr.Error()))
					continue
				}
				uploadLocalAsset = true
			}
		} else {
			uploadLocalAsset = true
		}
		if uploadLocalAsset {
			_, err = client.uploadLocalWebsiteAsset(ctx, websiteID, localAsset)
			if err != nil {
				client.logger.Error(err.Error())
			} else {
				client.logger.Info(fmt.Sprintf("Asset uploaded: %s", localAsset.Path))
			}
		}
	}

	// delete remote assets that don't exist locally
	if sync {
		localAssetsByPath := make(map[string]localAsset, len(localAssets))
		for _, asset := range localAssets {
			localAssetsByPath["/"+asset.Path] = asset
		}

		for _, websiteAsset := range websiteAssets {
			if websiteAsset.Type == content.AssetTypeFolder {
				continue
			}
			_, existsLocally := localAssetsByPath[websiteAsset.Path()]
			if !existsLocally {
				err = client.apiClient.DeleteAsset(ctx, content.DeleteAssetInput{ID: websiteAsset.ID})
				if err != nil {
					client.logger.Error(err.Error())
				} else {
					client.logger.Info(fmt.Sprintf("Asset deleted: %s", websiteAsset.Path()))
				}
			}
		}
	}

	return
}

func (client *Client) uploadLocalWebsiteAsset(ctx context.Context, websiteID guid.GUID, asset localAsset) (ret content.Asset, err error) {
	assetName := filepath.Base(asset.Path)
	assetFolder := "/" + filepath.Dir(asset.Path)

	assetFile, err := os.Open(asset.Path)
	if err != nil {
		err = fmt.Errorf("assets: error opening local asset for upload %s: %w", asset.Path, err)
		return
	}
	defer assetFile.Close()

	fileInfo, err := assetFile.Stat()
	if err != nil {
		err = fmt.Errorf("assets: error getting information about the file %s: %w", asset.Path, err)
		return
	}

	if fileInfo.Size() > kernel.MaxAssetSize {
		err = fmt.Errorf("assets: file %s is too large. Assets are currently limited to %d Bytes", asset.Path, kernel.MaxAssetSize)
		return
	}

	uploadAssetInput := content.UploadAssetInput{
		WebsiteID: websiteID,
		Name:      assetName,
		Folder:    &assetFolder,
		Data:      assetFile,
	}
	_, err = client.apiClient.UploadAsset(ctx, uploadAssetInput)
	if err != nil {
		err = fmt.Errorf("assets: error uploading %s: %w", asset.Path, err)
		return
	}

	return
}

func (client *Client) uploadLocalProductAsset(ctx context.Context, websiteID, productID guid.GUID,
	asset localAsset) (ret content.Asset, err error) {
	assetName := filepath.Base(asset.Path)
	assetFolder := "/" + filepath.Dir(asset.Path)

	assetFile, err := os.Open(asset.Path)
	if err != nil {
		err = fmt.Errorf("assets: error opening local asset for upload %s: %w", asset.Path, err)
		return
	}
	defer assetFile.Close()

	fileInfo, err := assetFile.Stat()
	if err != nil {
		err = fmt.Errorf("assets: error getting information about the file %s: %w", asset.Path, err)
		return
	}

	if fileInfo.Size() > kernel.MaxAssetSize {
		err = fmt.Errorf("assets: file %s is too large. Assets are currently limited to %d Bytes", asset.Path, kernel.MaxAssetSize)
		return
	}

	uploadAssetInput := content.UploadAssetInput{
		WebsiteID: websiteID,
		Name:      assetName,
		Folder:    &assetFolder,
		Data:      assetFile,
		ProductID: &productID,
	}
	_, err = client.apiClient.UploadAsset(ctx, uploadAssetInput)
	if err != nil {
		err = fmt.Errorf("assets: error uploading %s: %w", asset.Path, err)
		return
	}

	return
}

func (client *Client) loadLocalAssets(assetsDir string) (localAssets []localAsset, err error) {
	localAssets = make([]localAsset, 0, 100)

	fileSystem := os.DirFS(assetsDir)
	err = fs.WalkDir(fileSystem, ".", func(path string, file fs.DirEntry, err error) (walkErr error) {
		realPath := filepath.Join(assetsDir, path)

		if err != nil {
			walkErr = err
			return
		}
		if !fs.ValidPath(path) {
			walkErr = fmt.Errorf("assets: %s is not a valid path", realPath)
			return
		}
		if strings.Contains(path, "..") {
			walkErr = fmt.Errorf("assets: %s is not a valid path", realPath)
			return
		}

		fileType := file.Type()
		if fileType.IsDir() || !fileType.IsRegular() {
			return
		}

		info, walkErr := file.Info()
		if walkErr != nil {
			walkErr = fmt.Errorf("assets: error getting info for file %s: %w", realPath, walkErr)
			return
		}

		if info.Size() > kernel.MaxAssetSize {
			client.logger.Warn(fmt.Sprintf("assets: Ignoring %s: file is too large", realPath))
			return
		}

		assetFile, walkErr := fileSystem.Open(path)
		if walkErr != nil {
			walkErr = fmt.Errorf("assets: error opening %s: %w", realPath, walkErr)
			return
		}
		defer assetFile.Close()

		assetHasher := blake3.New()

		_, walkErr = io.Copy(assetHasher, assetFile)
		if walkErr != nil {
			walkErr = fmt.Errorf("assets: error computing hash for %s: %w", realPath, walkErr)
			return
		}

		assetHash := assetHasher.Sum(nil)

		localAsset := localAsset{
			Path: realPath,
			Hash: assetHash,
		}
		localAssets = append(localAssets, localAsset)

		return
	})

	if err != nil {
		err = fmt.Errorf("assets: error walking assets dir (%s): %v", assetsDir, err)
		return
	}
	return
}
