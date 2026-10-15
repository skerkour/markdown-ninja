package service

import (
	"path/filepath"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) getStoragePrefixForWebsite(websiteID guid.GUID) (prefix string) {
	prefix = filepath.Join(content.WebsitesStorageBasePath, websiteID.String())
	return
}

func (service *ContentService) getStorageKey(asset content.Asset) (storageKey string) {
	assetIDStr := asset.ID.String()
	assetIDFirstChars := assetIDStr[:4]

	storageKey = filepath.Join(service.getStoragePrefixForWebsite(asset.WebsiteID), "assets", assetIDFirstChars, assetIDStr)
	// fmt.Sprintf("%s/assets/%s/%s", service.getStoragePrefixForWebsite(asset.WebsiteID), assetIDFirstChars, assetIDStr)
	return
}
