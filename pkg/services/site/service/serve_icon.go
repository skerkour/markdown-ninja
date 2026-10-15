package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx"
)

func (service *SiteService) serveIcon(ctx context.Context, res http.ResponseWriter, req *http.Request, website websites.Website,
	hostname, url string) {
	if url == "/favicon.ico" || url == "/favicon.png" {
		http.Redirect(res, req, "/icon-64.png", http.StatusFound)
		return
	}

	httpCtx := httpctx.FromCtx(ctx)
	var iconSize int

	_, err := fmt.Sscanf(url, "/icon-%d.png", &iconSize)
	if err != nil {
		service.servePageNotFoundError(ctx, res, website, hostname, url)
		return
	}

	if !websites.WebsiteIconSizes.Contains(iconSize) {
		service.servePageNotFoundError(ctx, res, website, hostname, url)
		return
	}

	var etag string
	if website.CustomIcon {
		etag = base64.RawURLEncoding.EncodeToString(website.CustomIconHash)
	} else {
		defaultIcon := service.defaultIcons[iconSize]
		etag = defaultIcon.Etag
	}

	res.Header().Set(httpx.HeaderCacheControl, cachecontrol.WebsiteFavicon)
	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypePNG)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))

	if httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	var icon io.ReadCloser
	if website.CustomIcon {
		icon, err = service.websitesService.GetWebsiteIcon(ctx, website.ID, iconSize)
		if err != nil {
			service.serveInternalError(ctx, res, err, hostname, url)
			return
		}
	} else {
		defaultIcon := service.defaultIcons[iconSize]
		icon = io.NopCloser(bytes.NewReader(defaultIcon.Data))
		res.Header().Set(httpx.HeaderContentLength, strconv.Itoa(len(defaultIcon.Data)))
	}
	defer icon.Close()

	res.WriteHeader(http.StatusOK)
	io.Copy(res, icon)
}
