package slogx

import (
	"log/slog"

	"markdown.ninja/stdx/timex"
)

func Err(err error) slog.Attr {
	if err == nil {
		return slog.Any("error", nil)
	}

	return slog.String("error", err.Error())
}

func Time(key string, t timex.Time) slog.Attr {
	return slog.String(key, t.String())
}
