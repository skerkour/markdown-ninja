package bunny

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"markdown.ninja/stdx/httpx"
)

type Client struct {
	httpClient       *http.Client
	acountApiKey     string
	apiBaseURL       string
	streamApiBaseUrl string
	streamApiKey     string
}

func NewClient(acountApiKey, streamApiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = httpx.DefaultClient()
	}

	return &Client{
		httpClient:       httpClient,
		acountApiKey:     acountApiKey,
		streamApiKey:     streamApiKey,
		apiBaseURL:       "https://api.bunny.net",
		streamApiBaseUrl: "https://video.bunnycdn.com",
	}
}

type requestParams struct {
	Method          string
	URL             string
	Payload         interface{}
	useStreamApiKey bool
}

type uploadParams struct {
	Method          string
	URL             string
	Data            io.Reader
	useStreamApiKey bool
}

type APIError struct {
	Message string
}

func (res APIError) Error() string {
	return res.Message
}

func (client *Client) request(ctx context.Context, params requestParams, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, nil)
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

	apiKey := client.acountApiKey
	if params.useStreamApiKey {
		apiKey = client.streamApiKey
	}

	req.Header.Add(httpx.HeaderAccept, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderContentType, httpx.MediaTypeJson)
	req.Header.Add("AccessKey", apiKey)

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

func (client *Client) upload(ctx context.Context, params uploadParams, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, params.Data)
	if err != nil {
		return err
	}

	apiKey := client.acountApiKey
	if params.useStreamApiKey {
		apiKey = client.streamApiKey
	}

	req.Header.Add(httpx.HeaderAccept, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderContentType, "application/octet-stream")
	req.Header.Add("AccessKey", apiKey)

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

func (client *Client) download(ctx context.Context, params requestParams) (data io.ReadCloser, err error) {
	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, nil)
	if err != nil {
		return
	}

	apiKey := client.acountApiKey
	if params.useStreamApiKey {
		apiKey = client.streamApiKey
	}

	req.Header.Add(httpx.HeaderAccept, "*/*")
	req.Header.Add("AccessKey", apiKey)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode == 404 {
		err = errors.New("File not found")
		res.Body.Close()
		return
	}

	data = res.Body

	return
}
