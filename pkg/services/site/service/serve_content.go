package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/httpx"
)

func (service *SiteService) ServeContent(res http.ResponseWriter, req *http.Request) {
	var website websites.Website
	var err error
	ctx := req.Context()
	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname

	// TODO: cleanup input.Path
	path := strings.TrimSpace(req.URL.Path)
	if path == "" {
		path = "/"
	}

	website, err = service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		if errs.IsNotFound(err) {
			time.Sleep(500 * time.Millisecond)
			service.serveSiteNotFoundError(ctx, res)
			return
		}

		service.serveInternalError(ctx, res, err, hostname, path)
		return
	}

	trackPageEventInput := events.TrackPageViewInput{
		WebsitePrimaryDomain: website.PrimaryDomain,
		Path:                 httpCtx.Url.Path,
		IpAddress:            httpCtx.Client.IPStr,
		HeaderReferrer:       httpCtx.Headers.Get(httpx.HeaderReferer),
		HeaderUserAgent:      httpCtx.Client.UserAgent,
		QueryParameterRef:    httpCtx.Url.Query().Get("ref"),
		WebsiteID:            website.ID,
	}

	if website.BlockedAt != nil {
		service.serveSiteNotFoundError(ctx, res)
		return
	}

	if strings.Contains(path, "//") || strings.Contains(path, "..") || !utf8.ValidString(path) {
		service.servePageNotFoundError(ctx, res, website, hostname, path)
		return
	}

	// if domain is not primary and no redirect is found for this domain, we redirect to primary domain
	if hostname != website.PrimaryDomain {
		// TODO: path + query parameters
		res.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
		redirectToHostname := website.PrimaryDomain + service.httpConfig.WebsitesPort
		http.Redirect(res, req, fmt.Sprintf("%s://%s%s", service.httpConfig.WebsitesBaseUrl.Scheme, redirectToHostname, path), http.StatusMovedPermanently)
		return
	}

	// redirect requests with a trailing slash
	if len(path) > 1 && path[len(path)-1] == '/' {
		res.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
		http.Redirect(res, req, strings.TrimSuffix(path, "/"), http.StatusMovedPermanently)
		return
	}

	if strings.HasPrefix(path, websites.PreviewPrefix) {
		pageIdStr := strings.TrimPrefix(path, websites.PreviewPrefix)
		pageId, err := guid.Parse(pageIdStr)
		if err != nil {
			service.servePageNotFoundError(ctx, res, website, hostname, path)
			return
		}

		page, err := service.contentService.FindPageByID(ctx, service.db, pageId)
		if err != nil || (page.Type != content.PageTypePage && page.Type != content.PageTypePost) {
			service.servePageNotFoundError(ctx, res, website, hostname, path)
			return
		}

		service.servePage(ctx, res, website, page, hostname, path, http.StatusOK)
		return
	}

	// check if redirect exists
	redirects, err := service.websitesService.FindRedirects(ctx, service.db, website.ID)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, path)
		return
	}

	redirect := service.websitesService.MatchRedirect(ctx, hostname, path, redirects)
	if redirect != nil {
		// Sleep a little bit to avoid DoS in case of loop
		time.Sleep(100 * time.Millisecond)
		res.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
		http.Redirect(res, req, redirect.To, int(redirect.Status))
		return
	}

	if strings.HasPrefix(path, "/assets") {
		service.serveAsset(ctx, res, req, website, hostname, path)
		return
	}

	if strings.HasPrefix(path, "/theme/") {
		service.serveThemeAsset(ctx, res, website, hostname, path)
		return
	}

	// check if page is found at url
	page, err := service.contentService.FindPageByPath(ctx, service.db, website.ID, path)
	if err != nil {
		if !errs.IsNotFound(err) {
			service.serveInternalError(ctx, res, err, hostname, path)
			return
		}

		// if we are here, it means that it was a NotFound error
		err = nil

		for _, specialPage := range service.themes[website.Theme].SpecialPages {
			if specialPage.MatchString(path) {
				service.eventsService.TrackPageView(ctx, trackPageEventInput)
				service.serveEmptyPage(ctx, res, website, hostname, path)
				return
			}
		}

		// switch if faster than if / else if chains
		// https://stackoverflow.com/questions/29566229/go-switch-string-efficiency
		switch path {
		case "/sitemap.xml":
			service.serveSitemap(ctx, res, website, hostname, path)
			return

		case "/feed.xml":
			service.serveFeed(ctx, res, website, websites.FeedTypeRss, hostname, path)
			return

		case "/feed.json":
			service.serveFeed(ctx, res, website, websites.FeedTypeJson, hostname, path)
			return

		case "/rss", "/rss.xml":
			http.Redirect(res, req, "/feed.xml", http.StatusFound)
			return

		case "/robots.txt":
			service.serveRobotsTxt(ctx, res, website, hostname, path)
			return

		case "/favicon.ico", "/favicon.png", "/icon-32.png", "/icon-64.png", "/icon-128.png",
			"/icon-180.png", "/icon-192.png", "/icon-256.png", "/icon-512.png":
			service.serveIcon(ctx, res, req, website, hostname, path)
			return
		}

		service.servePageNotFoundError(ctx, res, website, hostname, path)
		return
	}

	service.eventsService.TrackPageView(ctx, trackPageEventInput)
	service.servePage(ctx, res, website, page, hostname, path, http.StatusOK)
}
