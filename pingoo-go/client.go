package pingoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/tetratelabs/wazero"
	"markdown.ninja/pingoo-go/wasm"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/retry"
	"markdown.ninja/stdx/uuid"
)

const userAgent = "Pingoo/GoSDK (https://pingoo.io)"

var dnsServers = []string{
	"8.8.8.8:53",
	"1.0.0.1:53",
	"8.8.4.4:53",
	"1.1.1.1:53",
	// "9.9.9.9:53",
}

type ClientConfig struct {
	Url        *string
	HttpClient *http.Client
	Logger     *slog.Logger
	GetLogger  func(context.Context) *slog.Logger
}

type Client struct {
	pingooURL  string
	projectId  uuid.UUID
	apiKey     string
	apiBaseUrl string
	httpClient *http.Client

	dnsResolver *net.Resolver

	jwksRefreshInterval time.Duration
	// jwks                jwt.Jwks
	// jwksMutex           sync.RWMutex

	wasmRuntime wazero.Runtime
	// compiledWasmModule wazero.CompiledModule
	// wasmModulePool     *sync.Pool
	wasmModule          *wasm.Module
	wasmModuleMutex     sync.Mutex
	wasmRefreshInterval time.Duration
	wasmEtag            atomic.Pointer[string]

	geoipDBRefreshInterval time.Duration
	geoipDBEtag            atomic.Pointer[string]

	getLogger func(context.Context) *slog.Logger
}

type ApiError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (err ApiError) Error() string {
	return err.Message
}

// TODO: wrap errors with errs
func NewClient(ctx context.Context, apiKey string, projectID string, config *ClientConfig) (client *Client, err error) {
	url := "https://pingoo.io"
	httpClient := httpx.DefaultClient()

	if config != nil {
		if config.Url != nil && *config.Url != "" {
			url = *config.Url
		}
		if config.HttpClient != nil {
			httpClient = config.HttpClient
		}
	}

	projectId, err := uuid.Parse(projectID)
	if err != nil {
		return nil, errors.New("pingoo: projectID is not valid")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	dnsResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{
				Timeout: 5 * time.Second,
			}
			dialer.SetMultipathTCP(false)
			dnsServer := dnsServers[rand.IntN(len(dnsServers))]
			return dialer.DialContext(ctx, network, dnsServer)
		},
	}

	getLoggerInner := config.GetLogger
	if getLoggerInner == nil {
		getLoggerInner = func(context.Context) *slog.Logger {
			return logger
		}
	}
	getLogger := func(context.Context) *slog.Logger {
		loggerFromCtx := getLoggerInner(ctx)
		if loggerFromCtx != nil {
			return loggerFromCtx
		}
		return logger
	}

	client = &Client{
		pingooURL:           url,
		apiBaseUrl:          url + "/api",
		projectId:           projectId,
		apiKey:              apiKey,
		httpClient:          httpClient,
		jwksRefreshInterval: time.Minute,
		dnsResolver:         dnsResolver,

		wasmRuntime: nil,
		// compiledWasmModule:  nil,
		wasmModule:          nil,
		wasmModuleMutex:     sync.Mutex{},
		wasmRefreshInterval: time.Minute,
		wasmEtag:            atomic.Pointer[string]{},

		geoipDBRefreshInterval: 12 * time.Hour,
		geoipDBEtag:            atomic.Pointer[string]{},

		getLogger: getLogger,
	}

	// WASM

	err = retry.Do(func() error {
		retryErr := client.refreshPingooWasm(ctx)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return nil, fmt.Errorf("pingoo: error downloading pingoo.wasm: %w", err)
	}

	// go client.refreshPingooWasmInBackground(ctx)

	// JWKS

	err = retry.Do(func() error {
		retryErr := client.refreshJwks(ctx)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return nil, fmt.Errorf("pingoo: error downloading JWKS: %w", err)
	}

	go client.refreshJwksInBackground(ctx)

	// GeoIP

	err = retry.Do(func() error {
		retryErr := client.refreshGeoipDatabase(ctx)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return nil, fmt.Errorf("pingoo: error downloading geoip database: %w", err)
	}

	go client.refreshGeoipDatabaseInBackground(ctx)

	return client, nil
}

type requestParams struct {
	Method  string
	Route   string
	Payload any
}

func (client *Client) request(ctx context.Context, params requestParams, dst any) error {
	url := client.apiBaseUrl + params.Route

	req, err := http.NewRequestWithContext(ctx, params.Method, url, nil)
	if err != nil {
		return err
	}

	if params.Payload != nil {
		jsonPayload, err := json.Marshal(params.Payload)
		if err != nil {
			return fmt.Errorf("client.request: marshaling JSON: %w", err)
		}

		// zstdCompressor, err := zstd.NewWriter(nil,
		// 	zstd.WithEncoderCRC(true),
		// 	zstd.WithEncoderConcurrency(1),
		// )
		// if err != nil {
		// 	return fmt.Errorf("client.request: error instantiating zstd encoder: %w", err)
		// }

		// compressedPayload := zstdCompressor.EncodeAll(jsonPayload, nil)
		// zstdCompressor.Close()

		// req.Body = io.NopCloser(bytes.NewReader(compressedPayload))
		req.Body = io.NopCloser(bytes.NewReader(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "zstd")
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "zstd")
	req.Header.Set("Authorization", "ApiKey "+client.apiKey)

	res, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client.request: Doing HTTP request: %w", err)
	}
	defer res.Body.Close()

	var body []byte

	resContentEncoding := res.Header.Get("Content-Encoding")
	if resContentEncoding != "" {
		if resContentEncoding != "zstd" {
			return fmt.Errorf("client.request: Content-Encoding \"%s\" not supported. Only zstd is currently supported", resContentEncoding)
		}

		bodyBuffer := bytes.NewBuffer(make([]byte, 0, 1000))

		zstdDecompressor, err := zstd.NewReader(res.Body,
			zstd.WithDecoderConcurrency(1),
		)
		if err != nil {
			return fmt.Errorf("client.request: error instantiating zstd encoder: %w", err)
		}

		_, err = io.Copy(bodyBuffer, zstdDecompressor)
		if err != nil {
			zstdDecompressor.Close()
			return fmt.Errorf("client.request: error decompressing response's body: %w", err)
		}
		zstdDecompressor.Close()
		body = bodyBuffer.Bytes()
	} else {
		body, err = io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("client.request: Reading body: %w", err)
		}
	}

	if res.StatusCode >= 400 {
		var apiErr ApiError
		err = json.Unmarshal(body, &apiErr)
		if err != nil {
			return fmt.Errorf("pingoo: error decoding API response: %w", err)
		}
		return apiErr
	}

	if dst != nil {
		err = json.Unmarshal(body, &dst)
		if err != nil {
			return fmt.Errorf("pingoo: error decoding API response: %w", err)
		}
	}

	return nil
}
