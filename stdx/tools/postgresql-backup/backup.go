package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func createS3Client(s3Config S3Config) (*minio.Client, error) {
	u, err := url.Parse(s3Config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid s3 endpoint: %w", err)
	}

	client, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(s3Config.AccessKeyID, s3Config.SecretAccessKey, ""),
		Secure: u.Scheme == "https",
		Region: s3Config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("creating s3 client: %w", err)
	}

	return client, nil
}

func createBackup(ctx context.Context, s3Client *minio.Client, s3Config S3Config, db DatabaseConfig) error {
	slog.Info("starting backup", "folder", db.Folder)

	compressed, err := runPgDumpCompressed(db.URL)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	encrypted, err := encrypt(compressed, db.PublicKey)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	now := time.Now().UTC()
	retainLockUntil := now.Add(14 * 24 * time.Hour)
	key := fmt.Sprintf("%s/%04d/%02d/%s.sql.gz.enc", db.Folder, now.Year(), now.Month(), now.Format(time.RFC3339))

	_, err = s3Client.PutObject(ctx, s3Config.Bucket, key,
		bytes.NewReader(encrypted), int64(len(encrypted)),
		minio.PutObjectOptions{ContentType: "application/octet-stream", RetainUntilDate: retainLockUntil})
	if err != nil {
		return fmt.Errorf("s3 upload failed: %w", err)
	}

	slog.Info("backup completed", "folder", db.Folder, "key", key)
	return nil
}

func runPgDumpCompressed(databaseURL string) ([]byte, error) {
	cmd := exec.Command("pg_dump", "--no-password", "-d", databaseURL)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	var buf bytes.Buffer
	gzipCompressor := gzip.NewWriter(&buf)
	if _, err := io.Copy(gzipCompressor, stdout); err != nil {
		cmd.Wait()
		return nil, fmt.Errorf("gzip: %w", err)
	}
	gzipCompressor.Close()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("wait: %w", err)
	}

	if err = gzipCompressor.Flush(); err != nil {
		return nil, fmt.Errorf("gzip flush: %w", err)
	}

	return buf.Bytes(), nil
}

func decompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
