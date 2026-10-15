package postmark

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
	httpClient      *http.Client
	accountApiToken string
	baseURL         string
}

func NewClient(accountApiToken string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = httpx.DefaultClient()
	}

	return &Client{
		httpClient:      httpClient,
		accountApiToken: accountApiToken,
		baseURL:         "https://api.postmarkapp.com",
	}
}

type requestParams struct {
	Method      string
	URL         string
	Payload     interface{}
	ServerToken *string
}

func (client *Client) request(ctx context.Context, params requestParams, dst interface{}) error {
	url := client.baseURL + params.URL

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

	if params.ServerToken != nil {
		req.Header.Add("X-Postmark-Server-Token", *params.ServerToken)
	} else {
		req.Header.Add("X-Postmark-Account-Token", client.accountApiToken)
	}

	res, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if dst == nil || res.StatusCode > 399 {
		var apiRes APIError
		err = json.Unmarshal(body, &apiRes)
		if err != nil {
			return err
		}
		if apiRes.ErrorCode != 0 {
			err = errors.New(apiRes.Message)
			return err
		}
	} else {
		err = json.Unmarshal(body, dst)
	}

	return err
}

type APIError struct {
	ErrorCode int64
	Message   string
}

func (res APIError) Error() string {
	return res.Message
}
