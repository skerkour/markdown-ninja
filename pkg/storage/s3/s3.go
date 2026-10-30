package s3

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"markdown.ninja/pkg/storage"
)

// ensure that Client satisfies the Storage interface
var _ storage.Storage = (*Client)(nil)

type Client struct {
	basePath string
	s3Client *s3.Client
	bucket   string
}

type ClientConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string
	Region          string
	BaseDirectory   string
	Bucket          string
	Minio           bool
	HttpClient      *http.Client
}

func NewClient(config ClientConfig) (*Client, error) {
	var options []func(*awsconfig.LoadOptions) error = make([]func(*awsconfig.LoadOptions) error, 0)
	var endpointResolver aws.EndpointResolverWithOptions

	options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")))
	options = append(options, awsconfig.WithRegion(config.Region))

	if config.Endpoint != "" {
		// see https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/endpoints/
		// https://stackoverflow.com/questions/67575681/is-aws-go-sdk-v2-integrated-with-local-minio-server
		// https://stackoverflow.com/questions/71088064/how-can-i-use-the-aws-sdk-v2-for-go-with-digitalocean-spaces
		// https://developers.cloudflare.com/r2/examples/aws/aws-sdk-go/
		endpointResolver = aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				// PartitionID:       "aws",
				URL:               config.Endpoint,
				SigningRegion:     config.Region,
				HostnameImmutable: true,
			}, nil
		})
		options = append(options, awsconfig.WithEndpointResolverWithOptions(endpointResolver))
	}

	if config.HttpClient != nil {
		options = append(options, awsconfig.WithHTTPClient(config.HttpClient))
	} else {
		// optimized HTTP client for S3.
		// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/custom-http/
		// https://buddhike.substack.com/p/tune-http-connection-pool-for-s3
		dialer := net.Dialer{
			Timeout: 10 * time.Second,
			// KeepAlive: 15 * time.Second,
			// KeepAliveConfig: net.KeepAliveConfig{
			// 	Enable: true,
			// },
		}
		dialer.SetMultipathTCP(false)
		transport := &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			// ResponseHeaderTimeout: 10,
			ExpectContinueTimeout: 5 * time.Second,
			// IdleConnTimeout:       300 * time.Second,
			// maybe we should use something like number of CPU * 64
			MaxIdleConnsPerHost: 128,

			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		}

		httpClient := &http.Client{
			Transport: transport,
		}
		options = append(options, awsconfig.WithHTTPClient(httpClient))
	}

	s3Config, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		err = fmt.Errorf("s3: error loading config: %w", err)
		return nil, err
	}

	// Create S3 service client
	s3Client := s3.NewFromConfig(s3Config, func(options *s3.Options) {
		// For now we disable this due to the WARN message logged by the S3 SDK because it's not supported
		// byt 3rd-party S3-compatible services
		// WARN Response has no supported checksum. Not validating response payload.
		options.DisableLogOutputChecksumValidationSkipped = true
	})
	return &Client{
		basePath: config.BaseDirectory,
		s3Client: s3Client,
		bucket:   config.Bucket,
	}, nil
}

func (client *Client) BasePath() string {
	return client.basePath
}

func (client *Client) CopyObject(ctx context.Context, from string, to string) error {
	from = filepath.Join(client.bucket, client.basePath, from)
	to = filepath.Join(client.basePath, to)

	_, err := client.s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(client.bucket),
		Key:        aws.String(to),
		CopySource: aws.String(from),
	})
	if err != nil {
		return err
	}

	return nil
}

func (client *Client) DeleteObject(ctx context.Context, key string) error {
	objectKey := filepath.Join(client.basePath, key)

	_, err := client.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return err
	}

	return nil
}

func (client *Client) GetObject(ctx context.Context, key string, options *storage.GetObjectOptions) (io.ReadCloser, error) {
	objectKey := filepath.Join(client.basePath, key)
	var objectRange *string

	if options != nil {
		objectRange = options.Range
	}

	result, err := client.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(objectKey),
		Range:  objectRange,
	})
	if err != nil {
		return nil, err
	}

	return result.Body, nil
}

func (client *Client) GetObjectSize(ctx context.Context, key string) (int64, error) {
	objectKey := filepath.Join(client.basePath, key)

	result, err := client.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return 0, err
	}

	if result.ContentLength == nil {
		return 0, errors.New("s3: object size is null")
	}

	return *result.ContentLength, nil
}

// func (storage *S3Storage) GetPresignedUploadUrl(ctx context.Context, key string, size uint64) (string, error) {
// 	objectKey := filepath.Join(storage.basePath, key)

// 	req, _ := storage.s3Client.PutObjectRequest(&s3.PutObjectInput{
// 		Bucket:        aws.String(storage.bucket),
// 		Key:           aws.String(objectKey),
// 		ContentLength: aws.Int64(int64(size)),
// 	})

// 	url, err := req.Presign(2 * time.Hour)
// 	if err != nil {
// 		return "", err
// 	}

// 	return url, nil
// }

func (client *Client) PutObject(ctx context.Context, key string, size int64, object io.Reader, options *storage.PutObjectOptions) error {
	objectKey := filepath.Join(client.basePath, key)

	var checksumSHA256 *string
	if options == nil {
		options = defaultPutObjectStorageOptions()
	}
	if options.HashSha256 != nil {
		sha256Base64 := base64.StdEncoding.EncodeToString(options.HashSha256)
		checksumSHA256 = &sha256Base64
	}

	reqInput := &s3.PutObjectInput{
		Bucket:         aws.String(client.bucket),
		Key:            aws.String(objectKey),
		Body:           object,
		ContentLength:  aws.Int64(int64(size)),
		ChecksumSHA256: checksumSHA256,
	}
	// if options.ContentMd5 != nil {
	// 	reqInput.ContentMD5 = aws.String(base64.StdEncoding.EncodeToString(options.ContentMd5))
	// }

	// if the ChecksumAlgorithm field is set, then it seems that the AWS SDK will compute the checksum by
	// itself (maybe with this middleware) and append it to the request with an HTTP trailer header.
	// Unfortunately, some S3-compatible services don't support HTTP trailers and requests will fail.
	// On the other hand, only by specifying the `ChecksumSHA256`, the SDK will set both the
	// ` x-amz-checksum-algorithm` and `x-amz-checksum-sha256` headers.
	// https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html
	// https://github.com/aws/aws-sdk-go-v2/issues/1689
	// reqInput.ChecksumAlgorithm = checksumAlgorithm

	_, err := client.s3Client.PutObject(ctx, reqInput)
	if err != nil {
		return err
	}

	return nil
}

func defaultPutObjectStorageOptions() *storage.PutObjectOptions {
	return &storage.PutObjectOptions{
		HashSha256: nil,
	}
}

func (client *Client) DeleteObjectsWithPrefix(ctx context.Context, prefix string) (err error) {
	s3Prefix := filepath.Join(client.basePath, prefix)
	var continuationToken *string

	for {
		var res *s3.ListObjectsV2Output

		res, err = client.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(client.bucket),
			Prefix:            aws.String(s3Prefix),
			ContinuationToken: continuationToken,
		})

		for _, object := range res.Contents {
			err = client.DeleteObject(ctx, *object.Key)
			if err != nil {
				return
			}
		}

		continuationToken = res.ContinuationToken

		if continuationToken == nil {
			break
		}
	}

	return
}
