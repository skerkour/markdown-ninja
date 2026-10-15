package service

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/httpx"
)

func (service *SiteService) ServePreview(res http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	// (ctx context.Context, input site.ServePreviewInput) (output site.ServeContentOutput) {
	contact := service.contactsService.CurrentContact(ctx)
	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname
	url := httpCtx.Url.Path
	pageIDStr := chi.URLParam(req, "page_id")
	pageID, err := guid.Parse(pageIDStr)
	if err != nil {
		err = content.ErrPageNotFound
		// TODO: send page error instead of API error
		apiutil.SendError(ctx, res, err)
		return
	}

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		if errs.IsNotFound(err) {
			service.serveSiteNotFoundError(ctx, res)
			return
		}
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	page, err := service.contentService.FindPageByID(ctx, service.db, pageID)
	if err != nil {
		if errs.IsNotFound(err) {
			service.servePageNotFoundError(ctx, res, website, hostname, url)
			return
		}
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}
	if !page.WebsiteID.Equal(website.ID) {
		err = content.ErrPageNotFound
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

	templateData, err := service.convertPageTemplateData(website, &sitePage, tags, contact, httpCtx.Client.CountryCode)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	contentBuffer := bytes.NewBuffer(make([]byte, 0, 100_000))
	err = service.themes[website.Theme].IndexTemplate.Execute(contentBuffer, templateData)
	if err != nil {
		err = fmt.Errorf("executing template: %w", err)
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	contentBytes := contentBuffer.Bytes()

	res.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeHtmlUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(contentBytes)), 10))
	res.WriteHeader(http.StatusOK)
	res.Write(contentBytes)
}
