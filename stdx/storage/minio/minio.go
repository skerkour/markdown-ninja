package minio

// import (
// 	"context"
// 	"fmt"
// 	"io"
// 	"net/http"
// 	"path/filepath"

// 	"github.com/minio/minio-go/v7"
// 	"github.com/minio/minio-go/v7/pkg/credentials"
// )

// type MinioStorage struct {
// 	basePath    string
// 	minioClient *minio.Client
// 	bucket      string
// }

// type Config struct {
// 	AccessKeyID     string
// 	SecretAccessKey string
// 	Endpoint        string
// 	Region          string
// 	BaseDirectory   string
// 	Bucket          string
// 	HttpClient      *http.Client
// }

// func NewMinioStorage(config Config) (*MinioStorage, error) {
// 	clientOptions := &minio.Options{
// 		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
// 		Secure: true,
// 	}
// 	if config.HttpClient != nil {
// 		clientOptions.Transport = config.HttpClient.Transport
// 	}

// 	minioClient, err := minio.New(config.Endpoint, clientOptions)
// 	if err != nil {
// 		err = fmt.Errorf("miniostorage: building minio client: %w", err)
// 		return nil, err
// 	}

// 	return &MinioStorage{
// 		basePath:    config.BaseDirectory,
// 		minioClient: minioClient,
// 		bucket:      config.Bucket,
// 	}, nil
// }

// func (storage *MinioStorage) BasePath() string {
// 	return storage.basePath
// }

// func (storage *MinioStorage) CopyObject(ctx context.Context, from string, to string) error {
// 	from = filepath.Join(storage.basePath, from)
// 	to = filepath.Join(storage.basePath, from)

// 	fromOptions := minio.CopySrcOptions{
// 		Bucket: storage.bucket,
// 		Object: from,
// 	}
// 	toOptions := minio.CopyDestOptions{
// 		Bucket: storage.bucket,
// 		Object: to,
// 	}
// 	_, err := storage.minioClient.CopyObject(ctx, toOptions, fromOptions)

// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (storage *MinioStorage) DeleteObject(ctx context.Context, key string) error {
// 	objectKey := filepath.Join(storage.basePath, key)

// 	err := storage.minioClient.RemoveObject(ctx, storage.bucket, objectKey, minio.RemoveObjectOptions{})
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (storage *MinioStorage) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
// 	objectKey := filepath.Join(storage.basePath, key)

// 	object, err := storage.minioClient.GetObject(ctx, storage.bucket, objectKey, minio.GetObjectOptions{})
// 	if err != nil {
// 		return nil, err
// 	}

// 	return object, nil
// }

// func (storage *MinioStorage) GetObjectSize(ctx context.Context, key string) (int64, error) {
// 	objectKey := filepath.Join(storage.basePath, key)

// 	info, err := storage.minioClient.StatObject(ctx, storage.bucket, objectKey, minio.GetObjectOptions{})
// 	if err != nil {
// 		return 0, err
// 	}

// 	return info.Size, nil
// }

// // func (storage *S3Storage) GetPresignedUploadUrl(ctx context.Context, key string, size uint64) (string, error) {
// // 	objectKey := filepath.Join(storage.basePath, key)

// // 	req, _ := storage.s3Client.PutObjectRequest(&s3.PutObjectInput{
// // 		Bucket:        aws.String(storage.bucket),
// // 		Key:           aws.String(objectKey),
// // 		ContentLength: aws.Int64(int64(size)),
// // 	})

// // 	url, err := req.Presign(2 * time.Hour)
// // 	if err != nil {
// // 		return "", err
// // 	}

// // 	return url, nil
// // }

// func (storage *MinioStorage) PutObject(ctx context.Context, key string, contentType string, size int64, object io.Reader) error {
// 	objectKey := filepath.Join(storage.basePath, key)

// 	_, err := storage.minioClient.PutObject(ctx, storage.bucket, objectKey, object, size, minio.PutObjectOptions{
// 		ContentType: contentType,
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (storage *MinioStorage) DeleteObjectsWithPrefix(ctx context.Context, prefix string) (err error) {
// 	s3Prefix := filepath.Join(storage.basePath, prefix)

// 	objectsChan := storage.minioClient.ListObjects(ctx, storage.bucket, minio.ListObjectsOptions{
// 		Prefix: s3Prefix,
// 	})

// 	removeObjectsErrors := storage.minioClient.RemoveObjects(ctx, storage.bucket, objectsChan, minio.RemoveObjectsOptions{})

// 	for removeObjectsError := range removeObjectsErrors {
// 		if removeObjectsError.Err != nil {
// 			err = removeObjectsError.Err
// 			return
// 		}
// 	}

// 	return
// }
