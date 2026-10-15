package pingoo

import (
	"context"
	"net/http"
	"net/netip"
	"time"

	"markdown.ninja/stdx/uuid"
)

type HttpLogRecord struct {
	Time         time.Time  `json:"time"`
	Duration     int64      `json:"duration"`
	Method       string     `json:"method"`
	Path         string     `json:"path"`
	Host         string     `json:"host"`
	ClientIP     netip.Addr `json:"client_ip"`
	UserAgent    string     `json:"user_agent"`
	StatusCode   uint16     `json:"status_code"`
	ResponseSize int64      `json:"response_size"`
	HTTPVersion  string     `json:"http_version"`
	// BotScore  int    `json:"http_version"`
	// RequestBodySize int
	// Referer         string
}

type PushHttpLogsInput struct {
	ProjectID uuid.UUID       `json:"project_id"`
	Logs      []HttpLogRecord `json:"logs"`
}

func (client *Client) PushHttpLogs(ctx context.Context, logs []HttpLogRecord) (err error) {
	apiInput := PushHttpLogsInput{
		ProjectID: client.projectId,
		Logs:      logs,
	}

	err = client.request(ctx, requestParams{
		Method:  http.MethodPost,
		Route:   "/http-logs/push",
		Payload: apiInput,
	}, nil)
	return
}
