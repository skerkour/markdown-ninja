package service

import (
	"context"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) InitNewWebsiteData(ctx context.Context, tx db.Queryer, website websites.Website) (err error) {
	now := time.Now().UTC()

	bodyMarkdown := `Hello World!`
	size := int64(len(bodyMarkdown))
	bodyHash := blake3.Sum256([]byte(bodyMarkdown))

	homePage := content.Page{
		ID:           guid.NewTimeBased(),
		CreatedAt:    now,
		UpdatedAt:    now,
		Date:         now,
		Type:         content.PageTypePage,
		Title:        website.Name,
		Path:         "/",
		Description:  bodyMarkdown,
		Language:     "en",
		Size:         size,
		BodyHash:     bodyHash[:],
		MetadataHash: []byte{},
		Status:       content.PageStatusPublished,
		BodyMarkdown: bodyMarkdown,
		WebsiteID:    website.ID,
	}
	metadataHash := content.HashPageMetadata(homePage.Type, homePage.Path, homePage.Date, homePage.SendAsNewsletter, homePage.Language, homePage.Title, homePage.Description, []string{})
	homePage.MetadataHash = metadataHash[:]

	err = service.repo.CreatePage(ctx, tx, homePage)
	if err != nil {
		return
	}

	// create /assets folder
	assetsFolder := content.Asset{
		ID:        guid.NewTimeBased(),
		CreatedAt: now,
		UpdatedAt: now,
		Type:      content.AssetTypeFolder,
		Name:      "assets",
		Folder:    "/",
		MediaType: "",
		Size:      0,
		Hash:      []byte{},
		WebsiteID: website.ID,
		ProductID: nil,
	}
	err = service.repo.CreateAsset(ctx, tx, assetsFolder)
	if err != nil {
		return
	}

	return
}
