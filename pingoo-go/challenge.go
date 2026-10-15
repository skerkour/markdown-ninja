package pingoo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"

	"markdown.ninja/stdx/uuid"
)

type challengeInitInput struct {
	ProjectID uuid.UUID  `json:"project_id"`
	Ip        netip.Addr `json:"ip"`
	UserAgent string     `json:"user_agent"`
	Hostname  string     `json:"hostname"`
}

type challengeInitOutput struct {
	Challenge  string   `json:"challenge"`
	Difficulty uint64   `json:"difficulty"`
	Cookies    []string `json:"cookies"`
}

type challengeInitResponseBody struct {
	Challenge  string `json:"challenge"`
	Difficulty uint64 `json:"difficulty"`
}

type challengeVerifyInput struct {
	ProjectID uuid.UUID  `json:"project_id"`
	Ip        netip.Addr `json:"ip"`
	UserAgent string     `json:"user_agent"`
	Hostname  string     `json:"hostname"`
	Token     string     `json:"token"`

	Hash  string `json:"hash"`
	Nonce string `json:"nonce"`
}

type challengeVerifyRequestBody struct {
	Hash  string `json:"hash"`
	Nonce string `json:"nonce"`
}

type challengeVerifyOutput struct {
	Cookies []string `json:"cookies"`
}

func (client *Client) handleChallengeInitRequest(ctx context.Context, ip netip.Addr, res http.ResponseWriter, req *http.Request) {
	logger := client.getLogger(ctx)

	var result challengeInitOutput
	input := challengeInitInput{
		Ip:        ip,
		ProjectID: client.projectId,
		UserAgent: req.UserAgent(),
		Hostname:  req.Host,
	}
	err := client.request(ctx, requestParams{
		Method:  http.MethodPost,
		Route:   "/challenge/init",
		Payload: input,
	}, &result)
	if err != nil {
		logger.Error("pingoo.handleChallengeInitRequest: error sending init API request: " + err.Error())
		// TODO: correct error handling
		client.serveInternalError(res)
		return
	}

	for _, cookie := range result.Cookies {
		res.Header().Add("Set-Cookie", cookie)
	}

	res.Header().Set("Content-Type", "application/json")

	res.WriteHeader(200)

	responseBody := challengeInitResponseBody{
		Challenge:  result.Challenge,
		Difficulty: result.Difficulty,
	}
	err = json.NewEncoder(res).Encode(responseBody)
	if err != nil {
		logger.Error("pingoo.handleChallengeInitRequest: error encoding response to JSON: " + err.Error())
		// TODO: correct error handling
		client.serveInternalError(res)
		return
	}
}

func (client *Client) handleChallengeVerifyRequest(ctx context.Context, ip netip.Addr, res http.ResponseWriter, req *http.Request) {
	logger := client.getLogger(ctx)

	bodyBuffer := bytes.NewBuffer(make([]byte, 0, 2_000))
	limitedBodyReader := http.MaxBytesReader(res, req.Body, 5_000_000) // 5 MB
	_, err := io.Copy(bodyBuffer, limitedBodyReader)
	if err != nil {
		logger.Error("pingoo.handleChallengeVerifyRequest: error reading request body: " + err.Error())
		// TODO: correct error handling
		client.serveInternalError(res)
		return
	}

	var requestBody challengeVerifyRequestBody
	err = json.Unmarshal(bodyBuffer.Bytes(), &requestBody)
	if err != nil {
		logger.Error("pingoo.handleChallengeVerifyRequest: error parsing request body: " + err.Error())
		// TODO: correct error handling
		client.serveInternalError(res)
		return
	}

	if len(requestBody.Hash) > 256 || len(requestBody.Nonce) > 256 {
		client.serveInternalError(res)
		return
	}

	var challengeJwt string
	challengeCookie, _ := req.Cookie("__pingoo_challenge")
	if challengeCookie != nil {
		challengeJwt = challengeCookie.Value
	}

	var result challengeVerifyOutput
	input := challengeVerifyInput{
		ProjectID: client.projectId,
		Ip:        ip,
		UserAgent: req.UserAgent(),
		Hostname:  req.Host,
		Token:     challengeJwt,
		Hash:      requestBody.Hash,
		Nonce:     requestBody.Nonce,
	}
	err = client.request(ctx, requestParams{
		Method:  http.MethodPost,
		Route:   "/challenge/verify",
		Payload: input,
	}, &result)
	if err != nil {
		logger.Error("pingoo.handleChallengeVerifyRequest: error sending API request: " + err.Error())
		// TODO: correct error handling
		client.serveInternalError(res)
		return
	}

	for _, cookie := range result.Cookies {
		res.Header().Add("Set-Cookie", cookie)
	}

	res.WriteHeader(200)
}
