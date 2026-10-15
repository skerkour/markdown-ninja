package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
)

func (service *SiteService) serveSiteNotFoundError(ctx context.Context, res http.ResponseWriter) {
	message := []byte("Not Found\n")
	service.serveError(ctx, res, message, http.StatusNotFound)
}

// TODO: use theme?
// err is used to log the error
func (service *SiteService) serveInternalError(ctx context.Context, res http.ResponseWriter, err error, hostname, path string) {
	logger := slogx.FromCtx(ctx)

	if !errors.Is(err, context.Canceled) {
		logger.Error("error serving content for website", slogx.Err(err),
			slog.String("domain", hostname), slog.String("path", path))
	}

	message := []byte("Internal Server Error\n")
	service.serveError(ctx, res, message, http.StatusInternalServerError)
}

func (service *SiteService) serveError(_ context.Context, res http.ResponseWriter, message []byte, statusCode int) {
	res.Header().Del(httpx.HeaderETag)
	res.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeHtmlUtf8)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(int64(len(message)), 10))
	res.WriteHeader(statusCode)
	res.Write(message)
}

func (service *SiteService) servePageNotFoundError(ctx context.Context, res http.ResponseWriter, website websites.Website, domain, url string) {
	now := time.Now().UTC()
	page := content.Page{
		ID:           guid.Empty,
		CreatedAt:    now,
		UpdatedAt:    now,
		Date:         now,
		Type:         content.PageTypePage,
		Title:        "Not Found",
		Path:         url,
		Description:  "Page Not Found",
		Language:     website.Language,
		Size:         0,
		BodyHash:     []byte{},
		MetadataHash: []byte{},
		Status:       content.PageStatusPublished,
		WebsiteID:    website.ID,
	}

	service.servePage(ctx, res, website, page, domain, url, http.StatusNotFound)
}

func (service *SiteService) serveAssetNotFoundError(ctx context.Context, res http.ResponseWriter) {
	message := []byte("Asset Not Found\n")
	service.serveError(ctx, res, message, http.StatusNotFound)
}

func (service *SiteService) serveVideoNotFoundError(ctx context.Context, res http.ResponseWriter) {
	message := []byte("Video Not Found\n")
	service.serveError(ctx, res, message, http.StatusNotFound)
}
