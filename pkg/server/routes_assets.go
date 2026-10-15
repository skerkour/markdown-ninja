package server

import (
	"fmt"
	"net/http"
	"strings"

	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/guid"
)

func (server *server) uploadAsset(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// r.Body = http.MaxBytesReader(w, r.Body, files.MaxFileSize+512)
	// 10 MB for potential metadata
	// err := req.ParseMultipartForm(kernel.MaxAssetSize + 10_000_000)
	// req.MultipartForm.RemoveAll() is automatically called at the end of the request, so any potential
	// temporary file is deleted
	err := req.ParseMultipartForm(10_000_000)
	if err != nil {
		// err = files.ErrFileIsTooLarge(files.MaxFileSize)
		err = fmt.Errorf("uploadAsset: parsing multipart form: %w", err)
		apiutil.SendError(ctx, w, err)
		return
	}

	file, fileHeader, err := req.FormFile("file")
	if err != nil {
		err = fmt.Errorf("uploadAsset: reading form file: %w", err)
		apiutil.SendError(ctx, w, err)
		return
	}
	defer file.Close()

	var productID *guid.GUID
	var folder *string

	siteIDStr := strings.TrimSpace(req.FormValue("website_id"))
	siteID, err := guid.Parse(siteIDStr)
	if err != nil {
		err = fmt.Errorf("uploadAsset: website_id is not valid: %w", err)
		apiutil.SendError(ctx, w, err)
		return
	}

	productIdStr := strings.TrimSpace(req.FormValue("product_id"))
	if productIdStr != "" {
		var productIDTmp guid.GUID
		productIDTmp, err = guid.Parse(productIdStr)
		if err != nil {
			err = fmt.Errorf("uploadAsset: product_id is not valid: %w", err)
			apiutil.SendError(ctx, w, err)
			return
		}
		productID = &productIDTmp
	}

	folderValue := strings.TrimSpace(req.FormValue("folder"))
	if folderValue != "" {
		folder = &folderValue
	}

	input := content.UploadAssetInput{
		WebsiteID: siteID,
		ProductID: productID,
		Data:      file,
		Name:      fileHeader.Filename,
		Folder:    folder,
	}
	asset, err := server.contentService.UploadAsset(ctx, input, false)
	if err != nil {
		apiutil.SendError(ctx, w, err)
		return
	}

	apiutil.SendResponse(ctx, w, http.StatusCreated, asset)
}
