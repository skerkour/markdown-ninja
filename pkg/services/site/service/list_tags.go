package service

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"

	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/httpx"
)

func (service *SiteService) ListTags(ctx context.Context, input kernel.EmptyInput) (ret kernel.PaginatedResult[site.Tag], err error) {
	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname
	cacheControl := cachecontrol.HeadlessApiTags
	contact := service.contactsService.CurrentContact(ctx)

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		return
	}

	trackEventInput := events.TrackPageViewInput{
		WebsitePrimaryDomain: website.PrimaryDomain,
		Path:                 "/tags",
		IpAddress:            httpCtx.Client.IPStr,
		HeaderReferrer:       httpCtx.Headers.Get(httpx.HeaderReferer),
		HeaderUserAgent:      httpCtx.Client.UserAgent,
		QueryParameterRef:    httpCtx.Url.Query().Get("ref"),
		WebsiteID:            website.ID,
	}
	service.eventsService.TrackPageView(ctx, trackEventInput)

	modifiedAt := website.ModifiedAt.Truncate(time.Second)
	etag := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(modifiedAt.UnixMilli(), 10)))
	if contact == nil && httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		httpCtx.Response.CacheHit = &httpctx.CacheHit{
			CacheControl: cacheControl,
			ETag:         etag,
		}
		return
	}

	tags, err := service.contentService.FindTags(ctx, service.db, website.ID)
	if err != nil {
		return
	}

	httpCtx.Response.Headers.Set(httpx.HeaderCacheControl, cacheControl)
	httpCtx.Response.Headers.Set(httpx.HeaderETag, strconv.Quote(etag))

	ret.Data = service.convertTags(tags)
	return ret, nil
}
