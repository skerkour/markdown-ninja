package cloudflare

import (
	"context"
	"fmt"
	"net/http"
)

type PurgeCacheRequest struct {
	PurgeEverything bool `json:"purge_everything"`
}

type PurgeCacheResponse struct {
	ID string `json:"id"`
}

func (client *Client) PurgeCache(ctx context.Context, zone string, input PurgeCacheRequest) (res PurgeCacheResponse, err error) {
	err = client.request(ctx, requestParams{
		Method:  http.MethodPost,
		URL:     fmt.Sprintf("/client/v4/zones/%s/purge_cache", zone),
		Payload: input,
	}, &res)
	return
}
