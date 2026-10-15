package httpx

// import (
// 	"bytes"
// 	"crypto/sha256"
// 	"encoding/base64"
// 	"errors"
// 	"fmt"
// 	"io"
// 	"io/fs"
// 	"mime"
// 	"net/http"
// 	"path/filepath"
// 	"strconv"
// 	"strings"
// 	"sync"

// 	"markdown.ninja/stdx/log/slogx"
// )

// type webappFileInfo struct {
// 	hash [32]byte
// 	size int64
// }

// // webappFileInfoCache is used to cache the metadata about a file
// // these metdata are used to send StatusNotModified response if the request has an If-None-Match HTTP
// // header
// type webappFileInfoCache struct {
// 	files map[string]webappFileInfo
// 	mutex sync.RWMutex
// }

// func (cache *webappFileInfoCache) Get(path string) (record webappFileInfo, exists bool) {
// 	cache.mutex.RLock()
// 	record, exists = cache.files[path]
// 	cache.mutex.RUnlock()
// 	return
// }

// func (cache *webappFileInfoCache) Set(path string, info webappFileInfo) {
// 	cache.mutex.Lock()
// 	cache.files[path] = info
// 	cache.mutex.Unlock()
// }

// func WebappHandlerOld(folder fs.FS) func(w http.ResponseWriter, r *http.Request) {
// 	cache := webappFileInfoCache{
// 		files: make(map[string]webappFileInfo, 100),
// 		mutex: sync.RWMutex{},
// 	}
// 	return func(w http.ResponseWriter, req *http.Request) {
// 		ctx := req.Context()
// 		logger := slogx.FromCtx(ctx)

// 		if req.Method != http.MethodGet && req.Method != http.MethodHead {
// 			w.WriteHeader(http.StatusMethodNotAllowed)
// 			w.Write([]byte("Method not allowed.\n"))
// 			return
// 		}

// 		ok, err := tryRead(folder, req.URL.Path, &cache, w, req)
// 		if err != nil && !errors.Is(err, ErrDir) && !errors.Is(err, ErrInvalidPath) {
// 			logger.Error("httpx.WebappHandler: reading file", slogx.Err(err))
// 			w.Header().Set(HeaderCacheControl, CacheControlNoCache)
// 			handleError(http.StatusInternalServerError, ErrInternalError.Error(), w)
// 			return
// 		}
// 		if ok {
// 			return
// 		}

// 		_, err = tryRead(folder, "index.html", &cache, w, req)
// 		if err != nil {
// 			logger.Error("httpx.WebappHandler: reading index.html", slogx.Err(err))
// 			w.Header().Set(HeaderCacheControl, CacheControlNoCache)
// 			handleError(http.StatusInternalServerError, ErrInternalError.Error(), w)
// 			return
// 		}
// 	}
// }

// // alternatively, we could pre-load all the files with their metadata like here: https://github.com/go-chi/chi/issues/611
// func tryRead(fs fs.FS, path string, cache *webappFileInfoCache, w http.ResponseWriter, req *http.Request) (ok bool, err error) {
// 	// path = filepath.Clean(path)
// 	if path == "" || strings.Contains(path, "..") {
// 		err = ErrInvalidPath
// 		return
// 	}
// 	// logger := slogx.FromCtx(r.Context())

// 	// TrimLeft is efficient here as we only trim 1 character so only bytes comparison, no UTF-8
// 	path = strings.TrimLeft(path, "/")

// 	extension := filepath.Ext(path)
// 	contentType := mime.TypeByExtension(extension)

// 	cacheControl := CacheControlDynamic
// 	switch extension {
// 	case ".js", ".css", ".woff", ".woff2":
// 		// some webapp's assets files can be cached for very long time because they are versionned by
// 		// the webapp's bundler
// 		cacheControl = CacheControlImmutable
// 	}

// 	w.Header().Set(HeaderContentType, contentType)
// 	w.Header().Set(HeaderCacheControl, cacheControl)

// 	// first, we handle caching
// 	requestEtag := decodeEtag(strings.TrimSpace(req.Header.Get(HeaderIfNoneMatch)))

// 	cachedFileInfo, isCached := cache.Get(path)
// 	if isCached && bytes.Equal(requestEtag, cachedFileInfo.hash[:]) {
// 		// logger.Debug("httpx.WebappHandler: cache HIT")
// 		w.Header().Set(HeaderETag, encodeEtagOptimized(cachedFileInfo.hash))
// 		w.Header().Set(HeaderContentLength, strconv.FormatInt(cachedFileInfo.size, 10))
// 		w.WriteHeader(http.StatusNotModified)
// 		ok = true
// 		return
// 	}

// 	file, err := fs.Open(path)
// 	if err != nil {
// 		err = ErrInvalidPath
// 		return
// 	}
// 	defer file.Close()

// 	// use fs.Stat instead?
// 	// embed.FS does not implement FS.Stat, so the file need to be Open / closed anyway
// 	fileInfo, err := file.Stat()
// 	if err != nil {
// 		err = ErrInternalError
// 		return
// 	}
// 	if fileInfo.IsDir() {
// 		err = ErrDir
// 		return
// 	}

// 	seeker, isSeeker := file.(io.Seeker)
// 	if !isSeeker {
// 		err = ErrInternalError
// 		return
// 	}

// 	// we hash the file to get its Etag
// 	hasher := sha256.New()
// 	_, err = io.Copy(hasher, file)
// 	if err != nil {
// 		err = ErrInternalError
// 		return
// 	}
// 	fileHash := hasher.Sum(nil)

// 	cachedFileInfo.size = fileInfo.Size()
// 	cachedFileInfo.hash = [32]byte(fileHash)
// 	cache.Set(path, cachedFileInfo)

// 	w.Header().Set(HeaderETag, encodeEtagOptimized(cachedFileInfo.hash))
// 	w.Header().Set(HeaderContentLength, strconv.FormatInt(cachedFileInfo.size, 10))

// 	if bytes.Equal(requestEtag, cachedFileInfo.hash[:]) {
// 		// logger.Debug("httpx.WebappHandler: etag HIT")
// 		w.WriteHeader(http.StatusNotModified)
// 		ok = true
// 		return
// 	}

// 	// logger.Debug("httpx.WebappHandler: MISS")

// 	w.WriteHeader(http.StatusOK)
// 	// finally, we can send the file
// 	seeker.Seek(0, io.SeekStart)
// 	_, err = io.Copy(w, file)
// 	if err != nil {
// 		err = fmt.Errorf("httpx.tryRead: copying content to HTTP response: %w", err)
// 		return
// 	}

// 	ok = true

// 	return
// }

// func decodeEtag(requestEtag string) (etagBytes []byte) {
// 	// sometimes, a CDN may add the weak Etag prefix: W/
// 	requestEtag = strings.TrimPrefix(requestEtag, "W/")
// 	requestEtag = strings.TrimPrefix(requestEtag, `"`)
// 	requestEtag = strings.TrimSuffix(requestEtag, `"`)
// 	etagBytes, err := base64.RawURLEncoding.DecodeString(requestEtag)
// 	if err != nil {
// 		etagBytes = nil
// 		return
// 	}
// 	return
// }

// // func toEtagPlus(hash [32]byte) string {
// // 	etag := base64.RawURLEncoding.EncodeToString(hash[:])
// // 	return `"` + etag + `"`
// // }

// func encodeEtagOptimized(hash [32]byte) string {
// 	base64Len := base64.RawURLEncoding.EncodedLen(32)
// 	buf := make([]byte, base64Len+2)
// 	buf[0] = '"'
// 	base64.RawURLEncoding.Encode(buf[1:], hash[:])
// 	buf[base64Len+1] = '"'
// 	return string(buf)
// }
