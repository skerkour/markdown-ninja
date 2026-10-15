package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"

	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx"
)

func (service *SiteService) serveEmptyPage(ctx context.Context, res http.ResponseWriter, website websites.Website, hostname, url string) {
	contact := service.contactsService.CurrentContact(ctx)
	contentBuffer := bytes.NewBuffer(make([]byte, 0, 50_000))
	httpCtx := httpctx.FromCtx(ctx)

	templateData, err := service.convertPageTemplateData(website, nil, nil, contact, httpCtx.Client.CountryCode)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	err = service.themes[website.Theme].IndexTemplate.Execute(contentBuffer, templateData)
	if err != nil {
		err = fmt.Errorf("executing index template: %w", err)
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	contentBytes := contentBuffer.Bytes()
	res.Header().Set(httpx.HeaderCacheControl, cachecontrol.Dynamic)
	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeHtmlUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(contentBytes)), 10))
	res.WriteHeader(http.StatusOK)
	res.Write(contentBytes)
}
