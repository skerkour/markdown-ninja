package httpx

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var ErrDir = errors.New("path is a folder")
var ErrInvalidPath = errors.New("path is not valid")
var ErrInternalError = errors.New("Internal Server Error")
var errFileIsMissing = func(file string) error { return fmt.Errorf("webappHandler: %s is missing", file) }

type fileMetadata struct {
	contentType string
	etag        string
	// we store the contentLength as a string to avoid the conversion to string for each request
	contentLength string
	cacheControl  string
}

type WebappHandlerConfig struct {
	// default: index.html
	NotFoundFile string
	// default: 200
	NotFoundStatus int
	// default: public, no-cache, must-revalidate
	NotFoundCacheControl string
	// default: ".js", ".css", ".woff", ".woff2"
	Cache []CacheRule
}

type CacheRule struct {
	Regexp         string
	compiledRegexp *regexp.Regexp
	CacheControl   string
}

// WebappHandler is an http.Handler that is designed to efficiently serve Single Page Applications.
// if a file is not found, it will return notFoundFile (default: index.html) with the stauscode statusNotFound
// WebappHandler sets the correct ETag header and cache the hash of files so that repeated requests
// to files return only StatusNotModified responses
// WebappHandler returns StatusMethodNotAllowed if the method is different than GET or HEAD
func WebappHandler(folder fs.FS, config *WebappHandlerConfig) (handler func(w http.ResponseWriter, r *http.Request), err error) {
	defaultConfig := defaultWebappHandlerConfig()
	if config == nil {
		config = defaultConfig
	} else {
		if config.NotFoundFile == "" {
			config.NotFoundFile = defaultConfig.NotFoundFile
		}
		if config.NotFoundStatus == 0 {
			config.NotFoundStatus = defaultConfig.NotFoundStatus
		}
		if config.NotFoundCacheControl == "" {
			config.NotFoundCacheControl = defaultConfig.NotFoundCacheControl
		}
		if config.Cache == nil {
			config.Cache = defaultConfig.Cache
		}
	}

	for i := range config.Cache {
		config.Cache[i].compiledRegexp, err = regexp.Compile(config.Cache[i].Regexp)
		if err != nil {
			err = fmt.Errorf("webappHandler: regexp is not valid: %s", config.Cache[i].Regexp)
			return
		}
	}

	filesMetadata, err := loadFilesMetdata(folder, config)
	if err != nil {
		return nil, err
	}

	handler = func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method not allowed.\n"))
			return
		}

		statusCode := http.StatusOK
		path := strings.TrimPrefix(req.URL.Path, "/")
		fileMetadata, fileExists := filesMetadata[path]
		cacheControl := fileMetadata.cacheControl
		if !fileExists {
			path = config.NotFoundFile
			fileMetadata = filesMetadata[path]
			statusCode = config.NotFoundStatus
			cacheControl = config.NotFoundCacheControl
		}

		w.Header().Set(HeaderETag, fileMetadata.etag)
		w.Header().Set(HeaderContentLength, fileMetadata.contentLength)
		w.Header().Set(HeaderContentType, fileMetadata.contentType)
		w.Header().Set(HeaderCacheControl, cacheControl)

		requestEtag := CleanIfNoneMatchHeader(req.Header.Get(HeaderIfNoneMatch))
		if (fileExists || config.NotFoundStatus == http.StatusOK) && requestEtag == fileMetadata.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.WriteHeader(statusCode)
		err = sendFile(folder, path, w)
		if err != nil {
			w.Header().Set(HeaderCacheControl, CacheControlNoCache)
			handleError(http.StatusInternalServerError, ErrInternalError.Error(), w)
			return
		}
	}
	return
}

func defaultWebappHandlerConfig() *WebappHandlerConfig {
	return &WebappHandlerConfig{
		NotFoundFile:         "index.html",
		NotFoundStatus:       http.StatusOK,
		NotFoundCacheControl: CacheControlDynamic,
		Cache: []CacheRule{
			{
				// some webapp's assets files can be cached for very long time because they are versionned by
				// the webapp's bundler
				Regexp:       ".*\\.(js|css|woff|woff2)$",
				CacheControl: CacheControlImmutable,
			},
			{
				Regexp:       ".*\\.(jpg|jpeg|png|webp|gif|svg|ico)$",
				CacheControl: "public, max-age=900, stale-while-revalidate=43200",
			},
		},
	}
}

func sendFile(folder fs.FS, path string, w http.ResponseWriter) (err error) {
	file, err := folder.Open(path)
	if err != nil {
		return
	}

	defer file.Close()

	_, err = io.Copy(w, file)
	return
}

func handleError(code int, message string, w http.ResponseWriter) {
	http.Error(w, message, code)
}

// CleanIfNoneMatchHeader removes the W/ and " from a If-None-Match header
func CleanIfNoneMatchHeader(requestEtag string) string {
	etag := strings.TrimSpace(requestEtag)
	etag = strings.TrimPrefix(etag, "W/")
	etag = strings.Trim(etag, `"`)
	return etag
}

func loadFilesMetdata(folder fs.FS, config *WebappHandlerConfig) (ret map[string]fileMetadata, err error) {
	ret = make(map[string]fileMetadata, 10)

	err = fs.WalkDir(folder, ".", func(path string, fileEntry fs.DirEntry, errWalk error) error {
		if errWalk != nil {
			return fmt.Errorf("webappHandler: error processing file %s: %w", path, errWalk)
		}

		if fileEntry.IsDir() || !fileEntry.Type().IsRegular() {
			return nil
		}

		fileInfo, errWalk := fileEntry.Info()
		if errWalk != nil {
			return fmt.Errorf("webappHandler: error getting info for file %s: %w", path, errWalk)
		}

		file, errWalk := folder.Open(path)
		if errWalk != nil {
			return fmt.Errorf("webappHandler: error opening file %s: %w", path, errWalk)
		}
		defer file.Close()

		// we hash the file to generate its Etag
		hasher := sha256.New()
		_, errWalk = io.Copy(hasher, file)
		if errWalk != nil {
			return fmt.Errorf("webappHandler: error hashing file %s: %w", path, errWalk)
		}
		fileHash := hasher.Sum(nil)

		etag := encodeEtag(fileHash)

		extension := filepath.Ext(path)
		contentType := mime.TypeByExtension(extension)

		// the cacheControl value depends on the type of the file
		cacheControl := CacheControlDynamic

		for _, cacheRule := range config.Cache {
			if cacheRule.compiledRegexp.Match([]byte(path)) {
				cacheControl = cacheRule.CacheControl
				break
			}
		}

		ret[path] = fileMetadata{
			contentType:   contentType,
			etag:          etag,
			contentLength: strconv.FormatInt(fileInfo.Size(), 10),
			cacheControl:  cacheControl,
		}

		return nil
	})

	if _, indexHtmlExists := ret[config.NotFoundFile]; !indexHtmlExists {
		err = errFileIsMissing(config.NotFoundFile)
		return
	}

	return
}

func encodeEtag(hash []byte) string {
	return `"` + base64.RawURLEncoding.EncodeToString(hash) + `"`
}
