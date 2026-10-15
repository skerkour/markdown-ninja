package bunny

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type UploadFileInput struct {
	// Leave empty for Falkenstein, DE
	Region          string
	StorageZoneName string
	Filename        string
	Data            io.Reader
}

type DownloadFileInput struct {
	// Leave empty for Falkenstein, DE
	Region          string
	StorageZoneName string
	Filename        string
}

// See https://docs.bunny.net/reference/put_-storagezonename-path-filename
func (client *Client) UploadFile(ctx context.Context, input UploadFileInput) (err error) {
	if input.Region != "" {
		input.Region += "."
	}

	err = client.upload(ctx, uploadParams{
		Data:   input.Data,
		Method: http.MethodPut,
		URL:    fmt.Sprintf("https://%sbunnycdn.com/%s/%s", input.Region, input.StorageZoneName, input.Filename),
	}, nil)

	return
}

// See https://docs.bunny.net/reference/get_-storagezonename-path-filename
func (client *Client) DownloadFile(ctx context.Context, input DownloadFileInput) (data io.ReadCloser, err error) {
	if input.Region != "" {
		input.Region += "."
	}

	data, err = client.download(ctx, requestParams{
		Method:          http.MethodGet,
		URL:             fmt.Sprintf("https://%sbunnycdn.com/%s/%s", input.Region, input.StorageZoneName, input.Filename),
		useStreamApiKey: true,
	})

	return
}
