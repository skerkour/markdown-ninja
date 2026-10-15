package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
)

func (service *SiteService) servePage(ctx context.Context, res http.ResponseWriter, website websites.Website, page content.Page,
	hostname, url string, statusCode int) {
	logger := slogx.FromCtx(ctx)
	// modifiedAt := timex.Max(page.ModifiedAt(), website.ModifiedAt).Truncate(time.Second)
	contact := service.contactsService.CurrentContact(ctx)
	cacheControl := cachecontrol.WebsitePage
	httpCtx := httpctx.FromCtx(ctx)

	if page.Status != content.PageStatusPublished {
		service.servePageNotFoundError(ctx, res, website, hostname, url)
		return
	}

	etag := computePageEtag(&page, website.ModifiedAt, service.themes[website.Theme].Hash, httpCtx.Client.CountryCode, contact)
	if statusCode == http.StatusOK &&
		httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		res.Header().Set(httpx.HeaderCacheControl, cacheControl)
		res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))
		res.WriteHeader(http.StatusNotModified)
		return
	}

	cacheKey := "page-html-" + etag
	if cachedValue := service.pagesHtmlCache.Get(cacheKey); cachedValue != nil && statusCode == http.StatusOK {
		logger.Debug("site: page cache hit")

		cachedPage := cachedValue.Value()

		res.Header().Set(httpx.HeaderCacheControl, cacheControl)
		res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))
		res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeHtmlUtf8)
		res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(cachedPage)), 10))
		res.WriteHeader(statusCode)
		res.Write(cachedPage)
		return
	}

	tags, err := service.contentService.FindTagsForPage(ctx, service.db, page.ID)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	snippets, err := service.contentService.FindSnippets(ctx, service.db, website.ID)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	sitePage := service.convertPage(ctx, website, page, tags, snippets)

	contentBuffer := bytes.NewBuffer(make([]byte, 0, 50_000))
	template := service.themes[website.Theme].IndexTemplate
	// TODO: improve how we detect and serve page not found
	pageTemplateData := &sitePage
	if statusCode == http.StatusNotFound {
		pageTemplateData = nil
	}

	templateData, err := service.convertPageTemplateData(website, pageTemplateData, tags, contact, httpCtx.Client.CountryCode)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	if statusCode < 299 {
		// if there is a contact, we don't cache to avoid any data leak
		if contact != nil {
			cacheControl = cachecontrol.NoCache
		}

		res.Header().Set(httpx.HeaderCacheControl, cacheControl)
		res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))
	} else {
		res.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
	}

	err = template.Execute(contentBuffer, templateData)
	if err != nil {
		err = fmt.Errorf("executing template: %w", err)
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	contentBytes := contentBuffer.Bytes()

	// if there is a contact, we don't cache to avoid any data leak
	if statusCode == http.StatusOK && contact == nil {
		service.pagesHtmlCache.Set(cacheKey, contentBytes, 0)
	}

	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeHtmlUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(contentBytes)), 10))
	res.WriteHeader(statusCode)
	res.Write(contentBytes)
}

// the etag for a page is computed from its ID, the hash of its content, the last time the website has been modified
// and the hash of the theme files, so if any of these things has changed, the etag value will change.
// Page MUST NOT be null. If page is null, then, to avoid segfaulting, a random ETAG will be returned
// which defeats the purpose of generating an etag.
func computePageEtag(page *content.Page, siteModifiedAt time.Time, themeHash []byte, country string, contact *contacts.Contact) (etag string) {
	var hash [32]byte

	if page == nil {
		rand.Read(hash[:])
		return base64.RawURLEncoding.EncodeToString(hash[:])
	}

	hasher := blake3.New()
	binary.Write(hasher, binary.LittleEndian, page.UpdatedAt.Unix())
	binary.Write(hasher, binary.LittleEndian, siteModifiedAt.Unix())
	hasher.Write([]byte(page.Path))
	hasher.Write(page.ID.Bytes())
	// hasher.Write(pageHash)
	hasher.Write(themeHash)
	hasher.Write([]byte(country))
	if contact != nil {
		hasher.Write(contact.ID.Bytes())
	}
	hasher.Sum(hash[:0])

	return base64.RawURLEncoding.EncodeToString(hash[:])
}
