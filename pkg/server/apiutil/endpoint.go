package apiutil

import (
	"context"
	"net/http"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/stdx/schema"
)

func JsonEndpoint[I any, O any](fn func(context.Context, I) (O, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		var input I
		var output O

		err := DecodeRequest(w, req, &input)
		if err != nil {
			SendError(ctx, w, err)
			return
		}

		output, err = fn(ctx, input)
		if err != nil {
			SendError(ctx, w, err)
			return
		}

		SendResponse(ctx, w, http.StatusOK, output)
	}
}

func JsonEndpointOk[I any](fn func(context.Context, I) error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		var input I

		err := DecodeRequest(w, req, &input)
		if err != nil {
			SendError(ctx, w, err)
			return
		}

		err = fn(ctx, input)
		if err != nil {
			SendError(ctx, w, err)
			return
		}

		SendOk(ctx, w)
	}
}

func GetEndpoint[I any, O any](fn func(context.Context, I) (O, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		var input I
		var output O
		var decoder = schema.NewDecoder()

		err := decoder.Decode(&input, req.URL.Query())
		if err != nil {
			err = errs.InvalidArgument("Error decoding query parameters")
			SendError(ctx, w, err)
			return
		}

		output, err = fn(ctx, input)
		if err != nil {
			SendError(ctx, w, err)
			return
		}

		SendResponse(ctx, w, http.StatusOK, output)
	}
}

func GetEndpointOk[I any](fn func(context.Context, I) error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		var input I
		var decoder = schema.NewDecoder()

		err := decoder.Decode(&input, req.URL.Query())
		if err != nil {
			err = errs.InvalidArgument("Error decoding query parameters")
			SendError(ctx, w, err)
			return
		}

		err = fn(ctx, input)
		if err != nil {
			SendError(ctx, w, err)
			return
		}

		SendOk(ctx, w)
	}
}
