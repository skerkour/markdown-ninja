package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/feeds"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/memorycache"
	"markdown.ninja/stdx/timex"
)

func (service *SiteService) serveFeed(ctx context.Context, res http.ResponseWriter, website websites.Website,
	feedType websites.FeedType, hostname, url string) {
	host := service.httpConfig.WebsitesBaseUrl.Scheme + "://" + website.PrimaryDomain + service.httpConfig.WebsitesPort
	cacheControl := cachecontrol.WebsiteFeed
	httpCtx := httpctx.FromCtx(ctx)
	modifiedAt := website.ModifiedAt.Truncate(time.Second)
	logger := slogx.FromCtx(ctx)

	// handle caching
	lastPost, err := service.contentService.FindLastPublishedPost(ctx, service.db, website.ID)
	if err != nil {
		if !errs.IsNotFound(err) {
			service.serveInternalError(ctx, res, err, hostname, url)
			return
		}
		err = nil
	} else {
		modifiedAt = timex.Max(lastPost.ModifiedAt(), modifiedAt).Truncate(time.Second)
	}

	// ContentType:  websites.MediaTypeRSS,
	mediaType := httpx.MediaTypeXml // Otherwise it prevents browsers from opening the file
	switch feedType {
	case websites.FeedTypeJson:
		mediaType = httpx.MediaTypeJson
	}

	etag := generateFeedEtag(&website, modifiedAt)
	res.Header().Set(httpx.HeaderCacheControl, cacheControl)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))
	res.Header().Set(httpx.HeaderContentType, mediaType)

	if httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	cacheKey := string(feedType) + "_" + etag
	if cachedFeed := service.feedsCache.Get(cacheKey); cachedFeed != nil {
		logger.Debug("site.serveFeed: memory cache hit")
		decompressedCachedData, err := service.cacheZstdDecompressor.DecodeAll(cachedFeed.Value(), nil)
		if err != nil {
			err = fmt.Errorf("site.serveFeed: uncompressing cached data: %w", err)
			service.serveInternalError(ctx, res, err, hostname, url)
			return
		}

		res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(decompressedCachedData)), 10))
		res.WriteHeader(http.StatusOK)
		res.Write([]byte(decompressedCachedData))
		return
	}

	posts, err := service.contentService.FindPublishedPagesMetadata(ctx, service.db, website.ID, []content.PageType{content.PageTypePost}, 50)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	feed := &feeds.Feed{
		Title:       website.Name,
		Link:        &feeds.Link{Href: host},
		Description: website.Description,
		// Author:      &feeds.Author{Name: "TODO", Email: "TODO"},
		Created:  website.CreatedAt.Truncate(time.Hour),
		Language: website.Language,
	}
	feed.Items = make([]*feeds.Item, len(posts))

	for i, page := range posts {
		// We don't need to reveal the actual ID of the page. We only need a stable identifier
		// for feed readers, so a hash is fine
		pageIdHash := blake3.Sum256(page.ID.Bytes())
		feed.Items[i] = &feeds.Item{
			Id:          hex.EncodeToString(pageIdHash[:]),
			Title:       page.Title,
			Link:        &feeds.Link{Href: host + page.Path},
			Description: page.Description,
			// Author:      &feeds.Author{Name: "TODO", Email: "TODO"},
			Created: page.Date,
			Updated: page.ModifiedAt().UTC().Truncate(time.Minute),
		}
	}

	var feedContent []byte

	switch feedType {
	case websites.FeedTypeRss:
		feedContent, err = feed.ToRss()
		if err != nil {
			err = fmt.Errorf("error encoding feed to RSS: %w", err)
			break
		}

	case websites.FeedTypeJson:
		feedContent, err = feed.ToJSON()
		if err != nil {
			err = fmt.Errorf("error encoding feed to JSON: %w", err)
		}

	default:
		err = fmt.Errorf("site.serveFeed: unknown feed type: %s", feedType)
	}
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	compressedContent := service.cacheZstdCompressor.EncodeAll(feedContent, make([]byte, 0, len(feedContent)/4))
	service.feedsCache.Set(cacheKey, compressedContent, memorycache.DefaultTTL)

	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(feedContent)), 10))
	res.WriteHeader(http.StatusOK)
	res.Write(feedContent)

}

func generateFeedEtag(website *websites.Website, modifiedAt time.Time) string {
	var hash [32]byte

	hasher := blake3.New()
	binary.Write(hasher, binary.LittleEndian, modifiedAt.Unix())
	hasher.Write(website.ID[:])
	hasher.Sum(hash[:0])

	return base64.RawURLEncoding.EncodeToString(hash[:])
}
