package mailgun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"markdown.ninja/stdx/httpx"
)

type Client struct {
	httpClient    *http.Client
	apiKey        string
	globalBaseURL string
	euBaseURL     string
}

func NewClient(apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = httpx.DefaultClient()
	}

	return &Client{
		httpClient:    httpClient,
		apiKey:        apiKey,
		globalBaseURL: "https://api.mailgun.net",
		euBaseURL:     "https://api.eu.mailgun.net",
	}
}

type requestParams struct {
	Method  string
	URL     string
	Payload interface{}
}

func (client *Client) request(ctx context.Context, params requestParams, dst interface{}) error {
	url := client.euBaseURL
	// url := client.globalBaseURL
	// if !global {
	// 	url = client.euBaseURL
	// }
	url += params.URL

	req, err := http.NewRequestWithContext(ctx, params.Method, url, nil)
	if err != nil {
		return err
	}

	if params.Payload != nil {
		payloadData, err := json.Marshal(params.Payload)
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewBuffer(payloadData))
	}

	req.Header.Add(httpx.HeaderAccept, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderContentType, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderUserAgent, "mailgun-go/4.6.1")
	req.SetBasicAuth("api", client.apiKey)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode >= 400 {
		var apiErr APIError

		err = json.Unmarshal(body, &apiErr)
		if err != nil {
			return err
		}

		return apiErr
	} else if dst != nil {
		err = json.Unmarshal(body, dst)
	}

	return err
}

// See https://github.com/mailgun/mailgun.js/blob/master/lib/Types/Common/Error.ts
// and https://github.com/mailgun/mailgun.js/tree/master/lib/Classes
// and https://github.com/mailgun/mailgun.js/blob/master/lib/Types/Common/ApiResponse.ts
type APIError struct {
	Message string `json:"message"`
}

func (res APIError) Error() string {
	return res.Message
}
