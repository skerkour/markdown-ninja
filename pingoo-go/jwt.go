package pingoo

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"markdown.ninja/pingoo-go/wasm"
	"markdown.ninja/pkg/jwt"
	"markdown.ninja/stdx/retry"
)

type verifyJwtInput struct {
	// JWKS  jwt.Jwks `json:"jwks"`
	Token string `json:"token"`
}

type verifyJwtOutput[C any] struct {
	Claims C `json:"claims"`
	// header
	// signature
}

// verify that the given JWT has been signed by one of the key contained in Pingoo's JWKS.
// returns the unmarshalled Claims
func VerifyJWT[C any](ctx context.Context, client *Client, token string) (claims C, err error) {
	wasmInput := verifyJwtInput{
		// JWKS:  client.jwks,
		Token: token,
	}
	wasmOutput, err := callWasmGuestFunction[verifyJwtInput, verifyJwtOutput[C]](ctx, client, "jwt_verify", wasmInput)
	if err != nil && strings.Contains(err.Error(), "key not found") {
		// if key was not found, refresh keys and revalidate
		client.refreshJwks(ctx)
		wasmOutput, err = callWasmGuestFunction[verifyJwtInput, verifyJwtOutput[C]](ctx, client, "jwt_verify", wasmInput)
	}
	if err != nil {
		return claims, fmt.Errorf("pingoo.VerifyJWT: error calling jwt_verify wasm function: %w", err)
	}

	return wasmOutput.Claims, nil
}

func (client *Client) refreshJwksInBackground(ctx context.Context) {
	logger := client.getLogger(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(client.jwksRefreshInterval):
		}

		err := retry.Do(func() error {
			retryErr := client.refreshJwks(ctx)
			return retryErr
		}, retry.Context(ctx), retry.Attempts(5), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
		if err != nil {
			logger.Warn("pingoo: error refreshing JWKS: " + err.Error())
		}
	}
}

func (client *Client) refreshJwks(ctx context.Context) error {
	var jwksRes jwt.Jwks
	logger := client.getLogger(ctx)

	err := client.request(ctx, requestParams{
		Method: http.MethodGet,
		Route:  fmt.Sprintf("/jwks/%s", client.projectId.String()),
	}, &jwksRes)
	if err != nil {
		return err
	}

	// TODO: validate JWKS?
	client.setJwtKeys(ctx, jwksRes)

	logger.Debug("pingoo: JWKS successfully refreshed")

	return nil
}

func (client *Client) setJwtKeys(ctx context.Context, jwks jwt.Jwks) (err error) {
	_, err = callWasmGuestFunction[jwt.Jwks, wasm.Empty](ctx, client, "jwt_set_keys", jwks)
	if err != nil {
		return fmt.Errorf("pingoo.VerifyJWT: error calling jwt_set_keys wasm function: %w", err)
	}

	return nil
}
