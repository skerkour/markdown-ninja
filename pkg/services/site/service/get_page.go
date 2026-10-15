package service

import (
	"context"
	"strconv"

	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/memorycache"
)

func (service *SiteService) GetPage(ctx context.Context, input site.GetPageInput) (ret site.Page, err error) {
	httpCtx := httpctx.FromCtx(ctx)
	var page content.Page
	hostname := httpCtx.Hostname
	// a dynamic cache control policy is used to avoid caching pages at the CDN level so we can track
	// page views.
	cacheControl := cachecontrol.Dynamic
	logger := slogx.FromCtx(ctx)

	if input.Slug == nil {
		err = content.ErrPageNotFound
		return
	}

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		return
	}

	page, err = service.contentService.FindPageByPath(ctx, service.db, website.ID, *input.Slug)
	if err != nil {
		return
	}

	if page.Status != content.PageStatusPublished {
		err = content.ErrPageNotFound
		return
	}

	trackEventInput := events.TrackPageViewInput{
		WebsitePrimaryDomain: website.PrimaryDomain,
		Path:                 page.Path,
		IpAddress:            httpCtx.Client.IPStr,
		HeaderReferrer:       httpCtx.Headers.Get(httpx.HeaderReferer),
		HeaderUserAgent:      httpCtx.Client.UserAgent,
		QueryParameterRef:    httpCtx.Url.Query().Get("ref"),
		WebsiteID:            website.ID,
	}
	service.eventsService.TrackPageView(ctx, trackEventInput)

	// handle caching
	// ThemeHash is not needed as it's an API call. There is no theme/frontend involved.
	// Nothing in the returned response depends on the data of the contact, so we don't need to check
	// if contact is present or not.
	etag := computePageEtag(&page, website.ModifiedAt, nil, httpCtx.Client.CountryCode, nil)
	if httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		httpCtx.Response.CacheHit = &httpctx.CacheHit{
			CacheControl: cacheControl,
			ETag:         etag,
		}
		return
	}

	if cachedValue := service.pagesCache.Get(etag); cachedValue != nil {
		logger.Debug("site.GetPage: page cache hit")
		return cachedValue.Value(), nil
	}

	tags, err := service.contentService.FindTagsForPage(ctx, service.db, page.ID)
	if err != nil {
		return
	}

	snippets, err := service.contentService.FindSnippets(ctx, service.db, website.ID)
	if err != nil {
		return
	}

	httpCtx.Response.Headers.Set(httpx.HeaderCacheControl, cacheControl)
	httpCtx.Response.Headers.Set(httpx.HeaderETag, strconv.Quote(etag))

	ret = service.convertPage(ctx, website, page, tags, snippets)
	service.pagesCache.Set(etag, ret, memorycache.DefaultTTL)

	return ret, nil
}
