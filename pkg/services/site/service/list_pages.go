package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"net/url"
	"strconv"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/timex"
)

func (service *SiteService) ListPages(ctx context.Context, input site.ListPagesInput) (ret kernel.PaginatedResult[site.PageMetadata], err error) {
	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname
	cacheControl := cachecontrol.HeadlessApiPages
	contact := service.contactsService.CurrentContact(ctx)
	var pages []content.PageMetadata

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		return ret, err
	}

	if input.Tag != nil {
		_, err = service.contentService.FindTag(ctx, service.db, website.ID, *input.Tag)
		if err != nil {
			return ret, err
		}

		trackEventInput := events.TrackPageViewInput{
			WebsitePrimaryDomain: website.PrimaryDomain,
			Path:                 "/tags/" + *input.Tag,
			IpAddress:            httpCtx.Client.IPStr,
			HeaderReferrer:       httpCtx.Headers.Get(httpx.HeaderReferer),
			HeaderUserAgent:      httpCtx.Client.UserAgent,
			QueryParameterRef:    httpCtx.Url.Query().Get("ref"),
			WebsiteID:            website.ID,
		}
		service.eventsService.TrackPageView(ctx, trackEventInput)
	}

	modifiedAt := website.ModifiedAt.Truncate(time.Second)

	var pageTypes []content.PageType
	if input.Type != nil {
		if *input.Type != content.PageTypePage &&
			*input.Type != content.PageTypePost {
			err = content.ErrPageTypeIsNotValid
			return
		}
		pageTypes = []content.PageType{*input.Type}

		if *input.Type == content.PageTypePost && input.Tag == nil {
			trackEventInput := events.TrackPageViewInput{
				WebsitePrimaryDomain: website.PrimaryDomain,
				Path:                 "/blog",
				IpAddress:            httpCtx.Client.IPStr,
				HeaderReferrer:       httpCtx.Headers.Get(httpx.HeaderReferer),
				HeaderUserAgent:      httpCtx.Client.UserAgent,
				QueryParameterRef:    httpCtx.Url.Query().Get("ref"),
				WebsiteID:            website.ID,
			}
			service.eventsService.TrackPageView(ctx, trackEventInput)

			lastPost, err := service.contentService.FindLastPublishedPost(ctx, service.db, website.ID)
			if err != nil {
				if !errs.IsNotFound(err) {
					return ret, err
				}
				err = nil
			} else {
				modifiedAt = timex.Max(lastPost.ModifiedAt(), modifiedAt)
			}
		}
	} else {
		pageTypes = []content.PageType{content.PageTypePage, content.PageTypePost}
	}

	// handle caching
	etag := generateEtagForListPages(httpCtx.Url, modifiedAt)

	if contact == nil && httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		httpCtx.Response.CacheHit = &httpctx.CacheHit{
			CacheControl: cacheControl,
			ETag:         etag,
		}
		return ret, nil
	}

	if input.Tag != nil {
		pages, err = service.contentService.FindPublishedPagesMetadataForTag(ctx, service.db, website.ID, pageTypes, *input.Tag)
		if err != nil {
			return ret, err
		}
	} else {
		pages, err = service.contentService.FindPublishedPagesMetadata(ctx, service.db, website.ID, pageTypes, 20_000)
		if err != nil {
			return ret, err
		}
	}

	httpCtx.Response.Headers.Set(httpx.HeaderCacheControl, cacheControl)
	httpCtx.Response.Headers.Set(httpx.HeaderETag, strconv.Quote(etag))

	ret.Data = service.convertPageMetadatas(website, pages)

	return ret, nil
}

func generateEtagForListPages(url *url.URL, modifiedAt time.Time) string {
	var hash [32]byte

	hasher := blake3.New()
	hasher.Write([]byte(url.String()))
	binary.Write(hasher, binary.LittleEndian, modifiedAt.Unix())
	hasher.Sum(hash[:0])

	return base64.RawURLEncoding.EncodeToString(hash[:])
}
