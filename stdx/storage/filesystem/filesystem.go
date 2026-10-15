package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FilesystemStorage struct {
	basePath string
}

type Config struct {
	BaseDirectory string
}

var (
	ErrKeyIsNotValid    = errors.New("storage key is not valid")
	ErrPrefixIsNotValid = errors.New("storage prefix is not valid")
)

func NewFilesystemStorage(config Config) *FilesystemStorage {
	return &FilesystemStorage{
		basePath: config.BaseDirectory,
	}
}

func (storage *FilesystemStorage) BasePath() string {
	return storage.basePath
}

func (storage *FilesystemStorage) CopyObject(ctx context.Context, from string, to string) (err error) {
	if strings.Contains(from, "..") || strings.Contains(to, "..") {
		err = ErrKeyIsNotValid
		return
	}

	from = filepath.Join(storage.basePath, from)
	source, err := os.Open(from)
	if err != nil {
		return
	}
	defer source.Close()

	to = filepath.Join(storage.basePath, to)
	destination, err := os.Create(to)
	if err != nil {
		return
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return
}

func (storage *FilesystemStorage) DeleteObject(ctx context.Context, key string) (err error) {
	if strings.Contains(key, "..") {
		err = ErrKeyIsNotValid
		return
	}

	filePath := filepath.Join(storage.basePath, key)
	return os.Remove(filePath)
}

func (storage *FilesystemStorage) GetObject(ctx context.Context, key string) (file io.ReadCloser, err error) {
	if strings.Contains(key, "..") {
		err = ErrKeyIsNotValid
		return
	}

	filePath := filepath.Join(storage.basePath, key)
	file, err = os.OpenFile(filePath, os.O_RDONLY, os.ModePerm)
	return
}

func (storage *FilesystemStorage) GetObjectSize(ctx context.Context, key string) (ret int64, err error) {
	if strings.Contains(key, "..") {
		err = ErrKeyIsNotValid
		return
	}

	filePath := filepath.Join(storage.basePath, key)

	fileStat, err := os.Stat(filePath)
	if err != nil {
		return
	}

	ret = fileStat.Size()
	return
}

// func (storage *FilesystemStorage) GetPresignedUploadUrl(ctx context.Context, key string, size uint64) (string, error) {
// 	panic("not implemented") // TODO: Implement
// }

func (storage *FilesystemStorage) PutObject(ctx context.Context, key string, contentType string, size int64, object io.Reader) (err error) {
	if strings.Contains(key, "..") {
		err = ErrKeyIsNotValid
		return
	}

	filePath := filepath.Join(storage.basePath, key)
	directory := filepath.Dir(filePath)

	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return
	}

	destination, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer destination.Close()

	_, err = io.Copy(destination, object)
	return
}

func (storage *FilesystemStorage) DeleteObjectsWithPrefix(ctx context.Context, prefix string) (err error) {
	if strings.Contains(prefix, "..") {
		err = ErrPrefixIsNotValid
		return
	}

	folder := filepath.Join(storage.basePath, prefix)

	err = os.RemoveAll(folder)
	return
}
