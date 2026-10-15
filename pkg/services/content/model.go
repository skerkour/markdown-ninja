package content

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"time"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/timex"
)

// TODO: improve the allowed max sizes
const (
	WebsitesStorageBasePath = "websites"
	UsersStorageBasePath    = "users"
	MaxAssetNestedFolders   = 30
	AssetPathMaxSize        = 150
	// File name or folder name
	AssetNameMaxSize = 128

	SnippetNameMinLength    = 2
	SnippetNameMaxLength    = 42
	SnippetNameAlphabet     = "abcdefghijklmnopqrstuvwxyz0123456789-_"
	SnippetContentMinLength = 1
	SnippetContentMaxLength = 5_000

	TagNameMinSize        = 1
	TagNameMaxSize        = 42
	TagDescriptionMaxSize = 420
	TagNameAlphabet       = "abcdefghijklmnopqrstuvwxyz0123456789-"

	PageBodyMarkdownMaxSize   = 80_000 // 80_000 KB
	PageTitleMaxSize          = 256
	PageTitleMinSize          = 1
	ContentPathAlphabet       = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~/"
	ContentDescriptionMaxSize = 420
	PagePathMaxSize           = 256

	PageDefaultLanguage = "en"
)

type PageType string

const (
	PageTypePage PageType = "page"
	PageTypePost PageType = "post"
)

type PageStatus string

const (
	PageStatusPublished PageStatus = "published"
	PageStatusDraft     PageStatus = "draft"
	PageStatusScheduled PageStatus = "scheduled"
)

func (status PageStatus) IsDraft() bool {
	return status == PageStatusDraft
}

type AssetType int64

const (
	AssetTypeFolder AssetType = iota
	AssetTypeFile             // TODO: document?
	AssetTypeImage
	AssetTypeAudio
	AssetTypeVideo
)

const (
	AssetTypeFileStr   = "file"
	AssetTypeImageStr  = "image"
	AssetTypeAudioStr  = "audio"
	AssetTypeVideoStr  = "video"
	AssetTypeFolderStr = "folder"
)

func (assetType AssetType) MarshalText() (ret []byte, err error) {
	switch assetType {
	case AssetTypeFile:
		ret = []byte(AssetTypeFileStr)
	case AssetTypeImage:
		ret = []byte(AssetTypeImageStr)
	case AssetTypeAudio:
		ret = []byte(AssetTypeAudioStr)
	case AssetTypeVideo:
		ret = []byte(AssetTypeVideoStr)
	case AssetTypeFolder:
		ret = []byte(AssetTypeFolderStr)
	default:
		err = fmt.Errorf("unknown AssetType: %d", assetType)
		return nil, err
	}

	return ret, nil
}

func (assetType AssetType) String() string {
	ret, _ := assetType.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (assetType *AssetType) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case AssetTypeFileStr:
		*assetType = AssetTypeFile
	case AssetTypeImageStr:
		*assetType = AssetTypeImage
	case AssetTypeAudioStr:
		*assetType = AssetTypeAudio
	case AssetTypeVideoStr:
		*assetType = AssetTypeVideo
	case AssetTypeFolderStr:
		*assetType = AssetTypeFolder
	default:
		err = fmt.Errorf("unknown AssetType: %s", string(data))
		return err
	}

	return nil
}

// type VideoStatus int64

// const (
// 	VideoStatusCreated VideoStatus = iota
// 	VideoStatusUploading
// 	VideoStatusUploaded
// 	VideoStatusTranscoding
// 	VideoStatusReady
// 	// https://en.wikipedia.org/wiki/Transcoding
// 	VideoStatusError
// )

// const (
// 	VideoStatusCreatedStr     = "created"
// 	VideoStatusUploadingStr   = "uploading"
// 	VideoStatusUploadedStr    = "uploaded"
// 	VideoStatusReadyStr       = "ready"
// 	VideoStatusTranscodingStr = "transcoding"
// 	VideoStatusErrorStr       = "error"
// )

var PageUrlBlocklist = []string{
	"/sitemap.xml",
	"/robots.txt",
	"/rss.xml",
	"/feed.xml",
	"/feed.json",
	"/icon-32.png",
	"/icon-64.png",
	"/icon-128.png",
	"/icon-180.png",
	"/icon-192.png",
	"/icon-256.png",
	"/icon-512.png",
	"/podcast.xml",
	"/favicon.ico",
	"/favicon.png",

	"/checkout",
	"/account",
	"/store", // TODO: products?
}

var SnippetNameBlocklist = []string{
	"youtube",
	"gallery",
	"tweet",
	"subscribe",
	"form",
	"vimeo",
	"video",
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

type Asset struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Type AssetType `db:"type" json:"type"`
	Name string    `db:"name" json:"name"`
	// Folder is the parent's folder path (e.g. /asset/2023/01)
	// Folder always starts with /assets
	Folder string `db:"folder" json:"folder"`
	// The Media Type of the asset, a.k.a. MIME Type or Content Type
	MediaType string `db:"media_type" json:"media_type"`
	// The size of the asset in bytes
	Size int64 `db:"size" json:"size"`
	// BLAKE3
	Hash kernel.BytesHex `db:"hash" json:"hash"`

	// Only valid when asset is a product's asset (product_id IS NOT NULL)
	// ProductAssetType ProductAssetType `db:"product_asset_type"`

	WebsiteID guid.GUID  `db:"website_id" json:"-"`
	ProductID *guid.GUID `db:"product_id" json:"-"`
}

func (asset Asset) Path() string {
	return filepath.Join(asset.Folder, asset.Name)
}

type Page struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Date         time.Time  `db:"date" json:"date"`
	Type         PageType   `db:"type" json:"type"`
	Title        string     `db:"title" json:"title"`
	Path         string     `db:"path" json:"path"`
	Description  string     `db:"description" json:"description"`
	Language     string     `db:"language" json:"language"`
	Status       PageStatus `db:"status" json:"status"`
	BodyMarkdown string     `db:"body_markdown" json:"body_markdown"`
	// Size is the size of the markdown body
	Size int64 `db:"size" json:"size"`
	// BLAKE3 hash of the markdown body
	BodyHash         kernel.BytesHex `db:"body_hash" json:"body_hash"`
	MetadataHash     kernel.BytesHex `db:"metadata_hash" json:"metadata_hash"`
	SendAsNewsletter bool            `db:"send_as_newsletter" json:"send_as_newsletter"`
	NewsletterSentAt *time.Time      `db:"newsletter_sent_at" json:"newsletter_sent_at"`

	// TitleDraft  string            `db:"title_draft" json:"title_draft"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`

	Tags []Tag `db:"-" json:"tags"`
}

func (page *Page) ModifiedAt() time.Time {
	return timex.Max(page.UpdatedAt, page.Date)
}

type Snippet struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Name    string `db:"name" json:"name"`
	Content string `db:"content" json:"content"`
	// BLAKE3 hash of the content
	Hash kernel.BytesHex `db:"hash" json:"hash"`
	// whether to render the snippet in emails or not
	RenderInEmails bool `db:"render_in_emails" json:"render_in_emails"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`
}

type Tag struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`
}

type TagPageRelation struct {
	PageID guid.GUID `db:"page_id"`
	TagID  guid.GUID `db:"tag_id"`
}

// type Author struct {
// 	ID        guid.GUID `db:"id" json:"id"`
// 	CreatedAt time.Time `db:"created_at" json:"created_at"`
// 	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

// 	Name string `db:"name" json:"name"`
// 	Bio  string `db:"bio" json:"bio"`

// 	WebsiteID guid.GUID `db:"website_id" json:"-"`
// }

// type AuthorPageRelation struct {
// 	PageID   guid.GUID `db:"page_id"`
// 	AuthorID guid.GUID `db:"author_id"`
// }

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

// Pages

type CreatePageInput struct {
	WebsiteID        guid.GUID `json:"website_id"`
	Date             time.Time `json:"date"`
	Type             PageType  `json:"type"`
	Title            string    `json:"title"`
	Path             string    `json:"path"`
	Description      string    `json:"description"`
	Language         string    `json:"language"`
	Tags             []string  `json:"tags"`
	Draft            bool      `json:"draft"`
	BodyMarkdown     string    `json:"body_markdown"`
	SendAsNewsletter bool      `json:"send_as_newsletter"`
}

type UpdatePageInput struct {
	PageID           guid.GUID  `json:"id"`
	Date             time.Time  `json:"date"`
	UpdatedAt        *time.Time `json:"updated_at"`
	Title            string     `json:"title"`
	Path             string     `json:"path"`
	Draft            bool       `json:"draft"`
	Description      *string    `json:"description"`
	Language         string     `json:"language"`
	Tags             []string   `json:"tags"`
	BodyMarkdown     *string    `json:"body_markdown"`
	SendAsNewsletter bool       `json:"send_as_newsletter"`
}

type DeletePageInput struct {
	PageID guid.GUID `json:"id"`
}

type GetPageInput struct {
	ID guid.GUID `json:"id"`
}

type PageMetadata struct {
	ID               guid.GUID       `db:"id" json:"id"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
	Date             time.Time       `db:"date" json:"date"`
	Type             PageType        `db:"type" json:"type"`
	Title            string          `db:"title" json:"title"`
	Description      string          `db:"description" json:"description"`
	Path             string          `db:"path" json:"path"`
	Size             int64           `db:"size" json:"size"`
	BodyHash         kernel.BytesHex `db:"body_hash" json:"body_hash"`
	MetadataHash     kernel.BytesHex `db:"metadata_hash" json:"metadata_hash"`
	Status           PageStatus      `db:"status" json:"status"`
	Language         string          `db:"language" json:"lang"`
	SendAsNewsletter bool            `db:"send_as_newsletter" json:"send_as_newsletter"`
	NewsletterSentAt *time.Time      `db:"newsletter_sent_at" json:"newsletter_sent_at"`
}

func (page *PageMetadata) ModifiedAt() time.Time {
	return timex.Max(page.UpdatedAt, page.Date)
}

type ListPagesInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	Query     string    `json:"query"`
}

// Assets

type UploadAssetInput struct {
	Data multipart.File
	// Name of the file
	Name string
	// Folder is the path of the parent folder. If the parent folder doesn't exist, it's created on the fly.
	// if Folder is null, a default path is used. e.g /assets/{year}/{month}
	// Folder must starts with /assets
	Folder    *string
	WebsiteID guid.GUID
	ProductID *guid.GUID
}

type GetAssetInput struct {
	WebsiteID *guid.GUID
	ID        *guid.GUID
	Path      *string
}

type ListAssetsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	// if folder is empty we return children of /assets
	Folder *string `json:"folder"`
}

type CreateAssetFolderInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	// Folder is the path of the parent folder. If the parent folder doesn't exist, it's created on the fly
	// Folder must starts with /assets
	Folder string `json:"folder"`
	// The name of the new folder
	Name string `json:"name"`
}

type DeleteAssetInput struct {
	ID guid.GUID `json:"id"`
}

// type ReplaceAssetInput struct {
// 	Data io.Reader
// }

// type UpdateAssetInput struct {
// 	ID   guid.GUID `json:"id"`
// The path of new the parent folder. If the parent folder doesn't exist, it's created on the fly.
// 	Path *string   `json:"path"`
// 	Name *string   `json:"name"`
// }

// Tags

type CreateTagInput struct {
	WebsiteID   guid.GUID `json:"website_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type UpdateTagInput struct {
	ID          guid.GUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type DeleteTagInput struct {
	ID guid.GUID `json:"id"`
}

type GetTagsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

// Snippets

type CreateSnippetInput struct {
	WebsiteID      guid.GUID `json:"website_id"`
	Name           string    `json:"name"`
	Content        string    `json:"content"`
	RenderInEmails *bool     `json:"render_in_emails"`
}

type UpdateSnippetInput struct {
	ID             guid.GUID `json:"id"`
	Name           string    `json:"name"`
	Content        string    `json:"content"`
	RenderInEmails *bool     `json:"render_in_emails"`
}

type DeleteSnippetInput struct {
	ID guid.GUID `json:"id"`
}

type ListSnippetsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type GetAssetDataOptions struct {
	Range *string
}
