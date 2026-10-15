package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx"
)

// This robots.txt directive is separate from others as it's always happended to the robots.txt file
// and can't be modified / removed
const robotsTxtTemplate = `
# Copying, scraping or crawling this website without explicit written permission is forbidden.
# Please contact hello[ at ]markdown.ninja if you want your crawler to be unblocked or for API access.
# ======================================================================================

%s

User-agent: *
Crawl-delay: 1
Disallow: /__markdown_ninja


Sitemap: %s
`

func (service *SiteService) serveRobotsTxt(ctx context.Context, res http.ResponseWriter, website websites.Website, domain, url string) {
	// modifiedAt := timex.Max(site.ModifiedAt, config.UTCBuildTime).Truncate(time.Second)
	modifiedAt := website.ModifiedAt.Truncate(time.Second)
	cacheControl := cachecontrol.WebsiteRobotsTxt
	httpCtx := httpctx.FromCtx(ctx)

	etag := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(modifiedAt.UnixMilli(), 10)))
	res.Header().Set(httpx.HeaderCacheControl, cacheControl)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))

	if httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	sitemapUrl := service.httpConfig.WebsitesBaseUrl.Scheme + "://" + website.PrimaryDomain + service.httpConfig.WebsitesPort + "/sitemap.xml"
	robotsTxtContent := fmt.Sprintf(robotsTxtTemplate, website.RobotsTxt, sitemapUrl)

	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeTextUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(robotsTxtContent)), 10))
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(robotsTxtContent))
}
