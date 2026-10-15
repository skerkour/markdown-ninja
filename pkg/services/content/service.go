package content

import (
	"context"
	"io"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type Service interface {
	InitNewWebsiteData(ctx context.Context, tx db.Queryer, website websites.Website) (err error)

	// Assets
	// UploadAsset is used to store asset
	// bypassAuthCheck is reserved for internal calls when you want to upload assets from an internal
	// service or worker
	UploadAsset(ctx context.Context, input UploadAssetInput, bypassAuthCheck bool) (asset Asset, err error)
	GetAsset(ctx context.Context, input GetAssetInput) (asset Asset, err error)
	GetAssetData(ctx context.Context, asset Asset, options *GetAssetDataOptions) (ret io.ReadCloser, err error)
	// DeleteAssetI(ctx context.Context, tx db.Queryer, assetID guid.GUID) (err error)
	DeleteWebsiteData(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error)
	// GetVideoIframe(ctx context.Context, assetID guid.GUID) (iframeHtml string, err error)
	FindProductAssets(ctx context.Context, db db.Queryer, productID guid.GUID) (assets []Asset, err error)
	CreateAssetFolder(ctx context.Context, input CreateAssetFolderInput) (asset Asset, err error)
	ListAssets(ctx context.Context, input ListAssetsInput) (assets []Asset, err error)
	FindWebsiteAssetByID(ctx context.Context, db db.Queryer, websiteID, assetID guid.GUID) (asset Asset, err error)
	// if the asset is a folder then all its children are also deleted
	DeleteAsset(ctx context.Context, input DeleteAssetInput) (err error)
	DeleteAssetInternal(ctx context.Context, tx db.Tx, assetID guid.GUID) (err error)
	// GetUsedStorageForWebsites returns the storage used (in Bytes) by the content for all the websites
	// of the given organization
	GetUsedStorageForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (storage int64, err error)
	GetAssetsCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error)

	// ReplaceAsset
	// UpdateAsset

	// Pages
	CreatePage(ctx context.Context, input CreatePageInput) (page Page, err error)
	DeletePage(ctx context.Context, input DeletePageInput) (err error)
	GetPage(ctx context.Context, input GetPageInput) (page Page, err error)
	UpdatePage(ctx context.Context, input UpdatePageInput) (page Page, err error)
	FindPageByPath(ctx context.Context, db db.Queryer, websiteID guid.GUID, path string) (page Page, err error)
	FindPublishedPagesMetadata(ctx context.Context, db db.Queryer, websiteID guid.GUID, pageTypes []PageType, limit int64) (posts []PageMetadata, err error)
	FindPublishedPagesMetadataForTag(ctx context.Context, db db.Queryer, websiteID guid.GUID, pageTypes []PageType, tag string) (pages []PageMetadata, err error)
	FindPageByID(ctx context.Context, db db.Queryer, pageID guid.GUID) (page Page, err error)
	FindLastPublishedPageOrPost(ctx context.Context, db db.Queryer, websiteID guid.GUID) (page Page, err error)
	FindLastPublishedPost(ctx context.Context, db db.Queryer, websiteID guid.GUID) (page Page, err error)
	ListPages(ctx context.Context, input ListPagesInput) (pages kernel.PaginatedResult[PageMetadata], err error)
	ListPosts(ctx context.Context, input ListPagesInput) (posts kernel.PaginatedResult[PageMetadata], err error)
	ValidatePageBodyMarkdown(body string) (err error)
	GetPagesCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error)
	ValidatePageTitle(titel string) error

	// Tags
	CreateTag(ctx context.Context, input CreateTagInput) (tag Tag, err error)
	UpdateTag(ctx context.Context, input UpdateTagInput) (tag Tag, err error)
	DeleteTag(ctx context.Context, input DeleteTagInput) (err error)
	FindTagsForPage(ctx context.Context, db db.Queryer, pageID guid.GUID) (tags []Tag, err error)
	FindTags(ctx context.Context, db db.Queryer, websiteID guid.GUID) (tags []Tag, err error)
	FindTag(ctx context.Context, db db.Queryer, websiteID guid.GUID, tag string) (ret Tag, err error)
	GetTags(ctx context.Context, input GetTagsInput) (tags []Tag, err error)

	// Snippets
	CreateSnippet(ctx context.Context, input CreateSnippetInput) (snippet Snippet, err error)
	UpdateSnippet(ctx context.Context, input UpdateSnippetInput) (snippet Snippet, err error)
	DeleteSnippet(ctx context.Context, input DeleteSnippetInput) (err error)
	FindSnippets(ctx context.Context, db db.Queryer, websiteID guid.GUID) (snippets []Snippet, err error)
	ListSnippets(ctx context.Context, input ListSnippetsInput) (ret kernel.PaginatedResult[Snippet], err error)
	RenderMarkdown(website websites.Website, markdownInput string, snippets []Snippet, isEmail bool) (html string)
	RenderSnippets(htmlInput string, snippets []Snippet, isEmail bool) (ret string)
	SanitizeHtml(input string) string

	// Jobs
	JobDeleteAssetData(ctx context.Context, input JobDeleteAssetData) (err error)
	JobDeleteAssetsDataWithPrefix(ctx context.Context, input JobDeleteAssetsDataWithPrefix) (err error)
	JobPublishPages(ctx context.Context, input JobPublishPages) (err error)

	// Tasks
	TaskPublishPages(ctx context.Context)
}
