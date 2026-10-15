package slogx

import (
	"log/slog"
	"os"
)

// TODO: use actual fatal level
// https://opentelemetry.io/docs/reference/specification/logs/data-model/#example-mappings
func Fatal(logger *slog.Logger, message string, args ...any) {
	logger.Error(message, args...)
	os.Exit(1)
}
