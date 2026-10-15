package kms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/retry"
)

type ScalewayKms struct {
	httpClient      *http.Client
	secretAccessKey string
	region          string
	masterKeyId     string
}

type scalewayEncryptInput struct {
	Plaintext []byte `json:"plaintext"`
	// associated_data Option<{
	//     value: String
	// }>
}

type scalewayEncryptOutput struct {
	Ciphertext []byte `json:"ciphertext"`
	// key_id: String,
}

type scalewayDecryptInput struct {
	Ciphertext []byte `json:"ciphertext"`
	// associated_data Option<{
	//     value: String
	// }>
}

type scalewayDecryptOutput struct {
	Plaintext []byte `json:"plaintext"`
	// key_id: String,
	//  "ciphertext": {
	//   "value": "string"
	// }
}

// https://github.com/scaleway/scaleway-sdk-go/blob/master/scw/errors.go
type sclewayErrorResponse struct {
	Message string `json:"message"`
}

func NewScalewayKms(httpClient *http.Client, secretAccessKey, region, masterKeyId string) *ScalewayKms {
	if httpClient == nil {
		httpClient = httpx.DefaultClient()
	}

	return &ScalewayKms{
		httpClient:      httpClient,
		secretAccessKey: secretAccessKey,
		region:          region,
		masterKeyId:     masterKeyId,
	}
}

func (kms *ScalewayKms) EncryptDataKey(ctx context.Context, keyId string, plaintext []byte) ([]byte, error) {
	var res scalewayEncryptOutput
	req := scalewayEncryptInput{
		Plaintext: plaintext,
	}
	url := fmt.Sprintf("https://api.scaleway.com/key-manager/v1alpha1/regions/%s/keys/%s/encrypt", kms.region, kms.masterKeyId)

	err := retry.Do(func() error {
		return kms.request(ctx, http.MethodPost, url, req, &res)
	}, retry.Context(ctx), retry.Attempts(3), retry.Delay(50*time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("scaleway: error encrypting data with KMS: %w", err)
	}

	return res.Ciphertext, nil
}

func (kms *ScalewayKms) DecryptDataKey(ctx context.Context, keyId string, ciphertext []byte) ([]byte, error) {
	var res scalewayDecryptOutput
	req := scalewayDecryptInput{
		Ciphertext: ciphertext,
	}
	url := fmt.Sprintf("https://api.scaleway.com/key-manager/v1alpha1/regions/%s/keys/%s/decrypt", kms.region, kms.masterKeyId)

	err := retry.Do(func() error {
		return kms.request(ctx, http.MethodPost, url, req, &res)
	}, retry.Context(ctx), retry.Attempts(3), retry.Delay(50*time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("scaleway: error decrypting data with KMS: %w", err)
	}

	return res.Plaintext, nil
}

func (kms *ScalewayKms) request(ctx context.Context, method string, url string, input any, output any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}

	if input != nil {
		var payloadData []byte

		payloadData, err = json.Marshal(input)
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(payloadData))
		req.Header.Add("Content-Type", "application/json")
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-Auth-Token", kms.secretAccessKey)

	res, err := kms.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending HTTP request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %w", err)
	}

	if res.StatusCode >= 400 {
		var apiErr sclewayErrorResponse
		err = json.Unmarshal(body, &apiErr)
		if err != nil {
			return fmt.Errorf("error decoding error API response: %w", err)
		}
		return errors.New(apiErr.Message)
	}

	if output != nil {
		err = json.Unmarshal(body, output)
		if err != nil {
			return fmt.Errorf("error decoding API response: %w", err)
		}
	}

	return nil
}
