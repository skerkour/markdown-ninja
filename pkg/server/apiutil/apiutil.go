package apiutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/cachecontrol"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
)

const MaxBodySize int64 = 300_000

type apiErrorCode string

type apiOk struct {
	Ok bool `json:"ok"`
}

type ApiError struct {
	Message string       `json:"message"`
	Code    apiErrorCode `json:"code"`
}

const (
	ErrorCodeNotFound               apiErrorCode = "NOT_FOUND"
	ErrorInvalidArgument            apiErrorCode = "INVALID_ARGUMENT"
	ErrorCodeInternal               apiErrorCode = "INTERNAL"
	ErrorCodePermissionDenied       apiErrorCode = "PERMISSION_DENIED"
	ErrorCodeAuthenticationRequired apiErrorCode = "AUTHENTICATION_REQUIRED"
	ErrorCodeTooManyRequests        apiErrorCode = "TOO_MANY_REQUESTS"
)

// var jsonEncodingOptions = json.JoinOptions(json.FormatNilMapAsNull(true))
// var jsonDecodingOptions = json.JoinOptions(json.RejectUnknownMembers(true))

func DecodeRequest(w http.ResponseWriter, req *http.Request, dest any) (err error) {
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodySize)
	contentType := strings.TrimSpace(strings.Split(req.Header.Get("Content-type"), ";")[0])
	contentType = strings.ToLower(contentType)

	if contentType != httpx.MediaTypeJson {
		err = errs.InvalidArgument("Content-Type header is not application/json")
		return
	}

	jsonDecoder := json.NewDecoder(req.Body)
	jsonDecoder.DisallowUnknownFields()
	err = jsonDecoder.Decode(dest)
	if err != nil {
		ctx := req.Context()
		logger := slogx.FromCtx(ctx)
		logger.Debug("apiutil.DecodeRequest: Decoding JSON", slogx.Err(err))
		errMessage := strings.TrimPrefix(err.Error(), "json:")
		err = errs.InvalidArgument("Input is not valid: " + errMessage)
		return
		// var unmarshalErr *json.UnmarshalTypeError

		// if errors.As(err, &unmarshalErr) {
		// 	err = errs.InvalidArgument("Input is not valid")
		// 	return
		// }
		// err = errs.Internal("apiutil.DeserializeRequest", err)
		// return
	}

	return
}

func SendResponse(ctx context.Context, w http.ResponseWriter, statusCode int, data any) {
	logger := slogx.FromCtx(ctx)
	httpCtx := httpctx.FromCtx(ctx)

	for header, values := range httpCtx.Response.Headers {
		w.Header().Del(header)
		for _, value := range values {
			if value != "" {
				w.Header().Add(header, value)
			}
		}
	}

	if len(httpCtx.Response.Cookies) != 0 {
		for _, cookie := range httpCtx.Response.Cookies {
			http.SetCookie(w, &cookie)
		}
	}

	w.Header().Set(httpx.HeaderContentType, httpx.MediaTypeJson)

	if httpCtx.Response.CacheHit != nil && httpCtx.Response.CacheHit.CacheControl != "" && httpCtx.Response.CacheHit.ETag != "" {
		w.Header().Set(httpx.HeaderCacheControl, httpCtx.Response.CacheHit.CacheControl)
		w.Header().Set(httpx.HeaderETag, strconv.Quote(httpCtx.Response.CacheHit.ETag))
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(statusCode)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		logger.Error("apiutil.SendResponse: encoding JSON", slogx.Err(err))
		SendError(ctx, w, err)
	}
}

func SendOk(ctx context.Context, w http.ResponseWriter) {
	SendResponse(ctx, w, http.StatusOK, apiOk{Ok: true})
}

func SendError(ctx context.Context, w http.ResponseWriter, err error) {
	var code apiErrorCode
	var statusCode int
	logger := slogx.FromCtx(ctx)

	message := err.Error()

	// TODO: other error types
	switch err.(type) {
	case *errs.NotFoundError:
		code = ErrorCodeNotFound
		statusCode = http.StatusNotFound
	case *errs.InvalidArgumentError:
		code = ErrorInvalidArgument
		statusCode = http.StatusBadRequest
	case *errs.PermissionDeniedError:
		code = ErrorCodePermissionDenied
		statusCode = http.StatusForbidden
	case *errs.AuthenticationRequiredError:
		code = ErrorCodeAuthenticationRequired
		statusCode = http.StatusUnauthorized
	case *errs.TooManyRequestsError:
		code = ErrorCodeTooManyRequests
		statusCode = http.StatusTooManyRequests
	default:
		code = ErrorCodeInternal
		statusCode = http.StatusInternalServerError
		message = "Internal Error. Please try again and contact support if the problem persists."
		if !errors.Is(err, context.Canceled) {
			logger.Error(err.Error())
		}
	}

	// make sure that the error is not cached
	w.Header().Del(httpx.HeaderCacheControl)
	w.Header().Del(httpx.HeaderETag)
	w.Header().Set(httpx.HeaderContentType, httpx.MediaTypeJson)
	w.Header().Set(httpx.HeaderCacheControl, cachecontrol.NoCache)
	w.WriteHeader(statusCode)

	apiErr := ApiError{
		Message: message,
		Code:    code,
	}
	err = json.NewEncoder(w).Encode(apiErr)
	if err != nil {
		logger.Error("apiutil.SendError: encoding error", slogx.Err(err))
		http.Error(w, `{"message":"Internal Error","code":"INTERNAL"}`, http.StatusInternalServerError)
	}
}
