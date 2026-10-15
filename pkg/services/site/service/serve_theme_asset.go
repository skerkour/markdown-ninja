package service

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx"
)

func (service *SiteService) serveThemeAsset(ctx context.Context, res http.ResponseWriter, website websites.Website, domain, assetUrl string) {
	httpCtx := httpctx.FromCtx(ctx)
	// theme assets are immutable
	cacheControl := cachecontrol.Immutable

	// as theme assets are immutable, we can use their URL as ETag
	assetUrlHash := blake3.Sum256([]byte(assetUrl))
	etag := base64.RawURLEncoding.EncodeToString(assetUrlHash[:])

	res.Header().Set(httpx.HeaderCacheControl, cacheControl)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))

	// as theme assets are immutable, if the Etag header is provided, we immedialty send back
	// a 304 Not Modified response
	if httpCtx.Request.IfNoneMatch != nil {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	file, err := service.themes[website.Theme].Assets.Open(strings.TrimPrefix(assetUrl, "/theme/"))
	if err != nil {
		// TODO: handle internal error
		service.servePageNotFoundError(ctx, res, website, domain, assetUrl)
		return
	}
	defer file.Close()

	var size int64
	fileInfo, err := file.Stat()
	if err == nil {
		size = fileInfo.Size()
	}

	// TODO: improve
	var mediaType string
	extension := filepath.Ext(assetUrl)
	if extension != "" {
		mediaType = mime.TypeByExtension(extension)
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	res.Header().Set(httpx.HeaderContentType, mediaType)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(size, 10))
	res.WriteHeader(http.StatusOK)
	io.Copy(res, file)
}
