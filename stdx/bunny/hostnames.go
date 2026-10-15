package bunny

import (
	"context"
	"fmt"
	"net/http"
)

type AddCustomHostnameInput struct {
	Hostname string `json:"Hostname"`
}

type RemoveCustomHostnameInput struct {
	Hostname string `json:"Hostname"`
}

func (client *Client) AddCustomHostname(ctx context.Context, pullZone, hostname string) (err error) {
	err = client.request(ctx, requestParams{
		Payload: AddCustomHostnameInput{
			Hostname: hostname,
		},
		Method: http.MethodPost,
		URL:    fmt.Sprintf("%s/pullzone/%s/addHostname", client.apiBaseURL, pullZone),
	}, nil)

	return
}

func (client *Client) RemoveCustomHostname(ctx context.Context, pullZone, hostname string) (err error) {
	err = client.request(ctx, requestParams{
		Payload: RemoveCustomHostnameInput{
			Hostname: hostname,
		},
		Method: http.MethodDelete,
		URL:    fmt.Sprintf("%s/pullzone/%s/removeHostname", client.apiBaseURL, pullZone),
	}, nil)

	return
}

func (client *Client) LoadFreeCertificate(ctx context.Context, hostname string) (err error) {
	err = client.request(ctx, requestParams{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("%s/pullzone/loadFreeCertificate?hostname=%s", client.apiBaseURL, hostname),
	}, nil)

	return
}
