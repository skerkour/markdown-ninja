package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/memorycache"
	"markdown.ninja/stdx/sitemap"
	"markdown.ninja/stdx/timex"
)

// TODO: improve location with site's primary domain
func (service *SiteService) serveSitemap(ctx context.Context, res http.ResponseWriter, website websites.Website,
	hostname, url string) {
	host := service.httpConfig.WebsitesBaseUrl.Scheme + "://" + website.PrimaryDomain + service.httpConfig.WebsitesPort
	cacheControl := cachecontrol.WebsiteSitemap
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

	etag := generateSitemapEtag(&website, modifiedAt)
	res.Header().Set(httpx.HeaderCacheControl, cacheControl)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))

	if httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	cacheKey := "sitemap-" + etag
	if cachedSitemap := service.sitemapsCache.Get(cacheKey); cachedSitemap != nil {
		logger.Debug("site.serveSitemap: memory cache hit")
		decompressedCachedData, err := service.cacheZstdDecompressor.DecodeAll(cachedSitemap.Value(), nil)
		if err != nil {
			err = fmt.Errorf("site.serveSitemap: uncompressing cached data: %w", err)
			service.serveInternalError(ctx, res, err, hostname, url)
			return
		}

		res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeXml)
		res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(decompressedCachedData)), 10))
		res.WriteHeader(http.StatusOK)
		res.Write([]byte(decompressedCachedData))
		return
	}

	pages, err := service.contentService.FindPublishedPagesMetadata(ctx, service.db, website.ID, []content.PageType{content.PageTypePage, content.PageTypePost}, math.MaxInt32)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	tags, err := service.contentService.FindTags(ctx, service.db, website.ID)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	sitemapFile := sitemap.New(false)
	for _, page := range pages {
		pageModifiedAt := page.ModifiedAt().UTC().Truncate(time.Minute)
		sitemapFile.Add(sitemap.URL{
			Loc:     host + page.Path,
			LastMod: &pageModifiedAt,
		})
	}
	sitemapFile.Add(sitemap.URL{
		Loc:     host + "/tags",
		LastMod: new(modifiedAt.UTC().Truncate(time.Minute)),
	})
	for _, tag := range tags {
		sitemapFile.Add(sitemap.URL{
			Loc:     host + "/tags/" + tag.Name,
			LastMod: new(tag.UpdatedAt.UTC().Truncate(time.Minute)),
		})
	}

	sitemapXML, err := sitemapFile.String()
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	compressedSitemap := service.cacheZstdCompressor.EncodeAll([]byte(sitemapXML), make([]byte, 0, len(sitemapXML)/4))
	service.sitemapsCache.Set(cacheKey, compressedSitemap, memorycache.DefaultTTL)

	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeXml)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(sitemapXML)), 10))
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(sitemapXML))
}

func generateSitemapEtag(website *websites.Website, modifiedAt time.Time) string {
	var hash [32]byte

	hasher := blake3.New()
	binary.Write(hasher, binary.LittleEndian, modifiedAt.Unix())
	hasher.Write(website.ID[:])
	hasher.Sum(hash[:0])

	return base64.RawURLEncoding.EncodeToString(hash[:])
}
