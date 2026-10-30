package mdninja

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"markdown.ninja/pkg/server/apiutil"
)

type Client struct {
	markdownNinjaUrl string
	apiKey           string
	httpClient       *http.Client
	apiBaseUrl       string
}

// var jsonEncodingOptions = json.JoinOptions(json.FormatNilMapAsNull(true))
// var jsonDecodingOptions = json.JoinOptions()

// TODO: wrap errors with errs
func NewClient(markdownNinjaUrl, apiKey string) (client *Client, err error) {
	dialer := net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 60 * time.Second,
	}
	dialer.SetMultipathTCP(false)
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	// _, err = token.Parse(websites.ApiKeyPrefix, apiKey)
	// if err != nil {
	// 	err = websites.ErrApiKeyIsNotValid
	// 	return
	// }

	client = &Client{
		markdownNinjaUrl: markdownNinjaUrl,
		apiBaseUrl:       markdownNinjaUrl + "/api",
		apiKey:           apiKey,
		httpClient: &http.Client{
			Transport: transport,
		},
	}

	return
}

type requestParams struct {
	Method  string
	Route   string
	Payload any
}

func (client *Client) request(ctx context.Context, params requestParams, dst any) (err error) {
	url := client.apiBaseUrl + params.Route

	req, err := http.NewRequestWithContext(ctx, params.Method, url, nil)
	if err != nil {
		return err
	}

	if params.Payload != nil {
		var payloadData []byte

		payloadData, err = json.Marshal(params.Payload)
		if err != nil {
			return fmt.Errorf("client.request: marshaling JSON: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(payloadData))
		req.Header.Add("Content-Type", "application/json")
	}

	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "ApiKey "+client.apiKey)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client.request: Doing HTTP request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("client.request: Reading body: %w", err)
	}

	if res.StatusCode >= 400 {
		var apiErr apiutil.ApiError
		err = json.Unmarshal(body, &apiErr)
		if err != nil {
			return fmt.Errorf("decoding error API response: %w", err)
		}
		return errors.New(apiErr.Message)
	}

	if dst != nil {
		err = json.Unmarshal(body, &dst)
		if err != nil {
			return fmt.Errorf("decoding API response: %w", err)
		}
	}

	return nil
}
