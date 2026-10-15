package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"markdown.ninja/stdx/httpx"
)

type TurnstileVerifyRequest struct {
	Secret string `json:"secret"`
	// The token from the frontend widget
	Response       string `json:"response"`
	RemoteIp       string `json:"remoteip"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type TurnstileVerifyResponse struct {
	Success     bool      `json:"success"`
	ChallengeTs time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []string  `json:"error-codes"`
	Action      string    `json:"action"`
	CData       string    `json:"cdata"`
}

// https://developers.cloudflare.com/turnstile/get-started/server-side-validation/
func (client *Client) VerifyTurnstileToken(ctx context.Context, input TurnstileVerifyRequest) (res TurnstileVerifyResponse, err error) {
	err = client.challengeRequest(ctx, requestParams{
		Payload: input,
		Method:  http.MethodPost,
		URL:     "https://challenges.cloudflare.com/turnstile/v0/siteverify",
	}, &res)
	if err != nil {
		return
	}

	if len(res.ErrorCodes) != 0 {
		err = errors.New(res.ErrorCodes[0])
	}

	return
}

func (client *Client) challengeRequest(ctx context.Context, params requestParams, dst any) error {
	if params.Payload == nil {
		return errors.New("cloudflare: request body is empty")
	}
	if dst == nil {
		return errors.New("cloudflare: destination is null")
	}

	requestBody, err := json.Marshal(params.Payload)
	if err != nil {
		return fmt.Errorf("cloudflare: encoding request's body ot JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("cloudflare: preparing request: %w", err)
	}

	req.Header.Add(httpx.HeaderAccept, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderContentType, httpx.MediaTypeJson)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	err = json.NewDecoder(res.Body).Decode(dst)
	if err != nil {
		return fmt.Errorf("cloudflare: decoding response's body: %w", err)
	}

	return nil
}
