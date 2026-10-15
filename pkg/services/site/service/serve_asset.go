package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/memorycache"
)

func (service *SiteService) serveAsset(ctx context.Context, res http.ResponseWriter, req *http.Request,
	website websites.Website, hostname, url string) {
	var err error
	contact := service.contactsService.CurrentContact(ctx)
	assetPath := url
	cacheControl := cachecontrol.WebsiteAsset
	httpCtx := httpctx.FromCtx(ctx)
	logger := slogx.FromCtx(ctx)

	getAssetInput := content.GetAssetInput{
		WebsiteID: &website.ID,
		Path:      &assetPath,
	}

	if url == "/assets" {
		idQueryParam := strings.TrimSpace(httpCtx.Url.Query().Get("id"))
		if idQueryParam == "" {
			service.serveAssetNotFoundError(ctx, res)
			return
		}

		var assetID guid.GUID
		assetID, err = guid.Parse(strings.TrimSpace(idQueryParam))
		if err != nil {
			service.serveAssetNotFoundError(ctx, res)
			return
		}
		getAssetInput.ID = &assetID
		getAssetInput.Path = nil
	}

	asset, err := service.contentService.GetAsset(ctx, getAssetInput)
	if err != nil {
		if errs.IsNotFound(err) {
			service.serveAssetNotFoundError(ctx, res)
			return
		}
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}

	if asset.ProductID != nil {
		err = service.storeService.CheckProductAccess(ctx, service.db, *asset.ProductID)
		if err != nil {
			// TODO: handle internal errors
			service.serveAssetNotFoundError(ctx, res)
			return
		}
	}

	if asset.Type == content.AssetTypeFolder {
		service.serveAssetNotFoundError(ctx, res)
		return
	}

	rangeHeader := strings.TrimSpace(req.Header.Get(httpx.HeaderRange))

	etag := generateAssetEtag(&asset, rangeHeader)
	res.Header().Set(httpx.HeaderCacheControl, cacheControl)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))

	if contact == nil &&
		httpCtx.Request.IfNoneMatch != nil && *httpCtx.Request.IfNoneMatch == etag {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	// handle Range requests
	if rangeHeader != "" {
		err = service.validateRangeRequest(rangeHeader)
		if err != nil {
			service.serveError(ctx, res, []byte(err.Error()), http.StatusBadRequest)
			return
		}

		service.servePartialAsset(ctx, res, website, hostname, url, asset, rangeHeader)
		return
	}

	contentDisposition := fmt.Sprintf("filename=%s", strconv.Quote(asset.Name))

	if httpCtx.Url.Query().Has("download") {
		// TODO: handle error and return error to client if downloadQueryParam is not valid
		contentDisposition = "attachment; " + contentDisposition
	}

	res.Header().Set(httpx.HeaderAcceptRanges, httpx.AcceptRangesBytes)
	res.Header().Set(httpx.HeaderContentDisposition, contentDisposition)
	res.Header().Set(httpx.HeaderContentType, asset.MediaType)
	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(asset.Size, 10))

	if cachedAsset := service.assetsCache.Get(etag); cachedAsset != nil {
		logger.Debug("site.serveAsset: memory cache hit")
		res.WriteHeader(http.StatusOK)
		res.Write(cachedAsset.Value())
		return
	}

	assetData, err := service.contentService.GetAssetData(ctx, asset, nil)
	if err != nil {
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}
	defer assetData.Close()

	// cache assets smaller or equal to 1 MB
	if asset.Size <= 1_000_000 {
		buffer := bytes.NewBuffer(make([]byte, 0, asset.Size))
		io.Copy(buffer, assetData)

		assetBytes := buffer.Bytes()
		service.assetsCache.Set(etag, assetBytes, memorycache.DefaultTTL)

		res.WriteHeader(http.StatusOK)
		res.Write(assetBytes)
		return
	} else {
		res.WriteHeader(http.StatusOK)
		io.Copy(res, assetData)
		return
	}
}

func generateAssetEtag(asset *content.Asset, rangeHeader string) string {
	var hash [32]byte

	hasher := blake3.New()
	hasher.Write(asset.Hash)
	hasher.Write([]byte(rangeHeader))
	hasher.Sum(hash[:0])

	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// TODO: content type?
// See https://www.rfc-editor.org/rfc/rfc9110.html#name-range
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Range_requests
// https://stackoverflow.com/questions/3303029/http-range-header
// https://bunny.net/academy/http/what-are-http-range-and-byte-range-requests
// https://http.dev/range-request
func (service *SiteService) servePartialAsset(ctx context.Context, res http.ResponseWriter,
	website websites.Website, hostname, url string, asset content.Asset, rangeHeader string) {
	logger := slogx.FromCtx(ctx)

	matches := site.RangeHeaderRegexp.FindAllStringSubmatch(rangeHeader, -1)
	// make sur that the matches are in the form [["xxx", "xxx", "xxx"]]
	if len(matches) != 1 || len(matches[0]) != 3 {
		service.serveError(ctx, res, []byte(site.ErrRangeRequestIsNotValid.Error()), http.StatusBadRequest)
		return
	}

	requestedRange := matches[0][0]
	contentRangeFromStr := matches[0][1]
	contentRangeToStr := matches[0][2]
	// the size of the requested part
	partSize, contentRangeFrom, contentRangeTo, err := computeRangeSize(asset.Size, contentRangeFromStr, contentRangeToStr)
	if err != nil {
		service.serveError(ctx, res, []byte(err.Error()), http.StatusBadRequest)
		return
	}

	getAssetDataOptions := content.GetAssetDataOptions{
		Range: &requestedRange,
	}
	assetData, err := service.contentService.GetAssetData(ctx, asset, &getAssetDataOptions)
	if err != nil {
		logger.Error("content.serveAsset: getting asset data with range", slogx.Err(err),
			slog.String("website.id", website.ID.String()), slog.String("asset.id", asset.ID.String()),
			slog.String("range", requestedRange),
		)
		err = fmt.Errorf("error getting asset data with range [%s]: %w", requestedRange, err)
		service.serveInternalError(ctx, res, err, hostname, url)
		return
	}
	defer assetData.Close()

	res.Header().Set(httpx.HeaderContentLength, strconv.FormatInt(partSize, 10))
	res.Header().Set(httpx.HeaderContentRange, formatContentRangeHeader(contentRangeFrom, contentRangeTo, asset.Size))
	res.WriteHeader(http.StatusPartialContent)
	io.Copy(res, assetData)
}

func computeRangeSize(fileSize int64, fromInput, toInput string) (partSize, from, to int64, err error) {
	from, err = strconv.ParseInt(fromInput, 10, 64)
	if err != nil {
		err = site.ErrRangeRequestIsNotValid
		return
	}

	if toInput == "" {
		to = fileSize - 1
	} else {
		to, err = strconv.ParseInt(toInput, 10, 64)
		if err != nil {
			err = site.ErrRangeRequestIsNotValid
			return
		}
	}

	if to >= fileSize || to < from {
		err = site.ErrRangeRequestIsNotValid
		return
	}
	if from < 0 {
		err = site.ErrRangeRequestIsNotValid
		return
	}

	return to - from + 1, from, to, nil
}

func formatContentRangeHeader(from, to, totalSize int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", from, to, totalSize)
}
