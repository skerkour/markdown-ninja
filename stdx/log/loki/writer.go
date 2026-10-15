package loki

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"markdown.ninja/stdx/httpx"
)

type WriterOptions struct {
	LokiEndpoint string
	// ChildWriter is used to pass logs to another writer
	// default: os.Stdout
	ChildWriter io.Writer
	// DefaultRecordsBufferSize is your expected number of `(logs per second) / (1000 / FlushTimeout)`
	// default: 100
	DefaultRecordsBufferSize uint32
	// EmptyEndpointMaxBufferSize is the number of logs to buffer if LokiEndpoint == "".
	// it's useful if you log a few things before your config with the loki endpoint is full loaded
	// default: 200
	EmptyEndpointMaxBufferSize uint32
	// FlushTimeout in ms
	// default: 200
	FlushTimeout uint32
}

type Writer struct {
	lokiEndpoint               string
	streams                    map[string]string
	defaultRecordsBufferSize   uint32
	emptyEndpointMaxBufferSize uint32
	flushTimeout               uint32

	httpClient         *http.Client
	recordsBuffer      []record
	recordsBufferMutex sync.Mutex
	childWriter        io.Writer
}

type record struct {
	timestamp time.Time
	message   string
}

func NewWriter(ctx context.Context, lokiEndpoint string, streams map[string]string, options *WriterOptions) *Writer {
	if streams == nil {
		streams = map[string]string{}
	}

	defaultOptions := defaultOptions()
	if options == nil {
		options = defaultOptions
	} else {
		if options.ChildWriter == nil {
			options.ChildWriter = defaultOptions.ChildWriter
		}
		if options.DefaultRecordsBufferSize == 0 {
			options.DefaultRecordsBufferSize = defaultOptions.DefaultRecordsBufferSize
		}
		if options.EmptyEndpointMaxBufferSize == 0 {
			options.EmptyEndpointMaxBufferSize = defaultOptions.EmptyEndpointMaxBufferSize
		}
		if options.FlushTimeout == 0 {
			options.FlushTimeout = defaultOptions.FlushTimeout
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}

	writer := &Writer{
		lokiEndpoint:               lokiEndpoint,
		streams:                    streams,
		defaultRecordsBufferSize:   options.DefaultRecordsBufferSize,
		emptyEndpointMaxBufferSize: options.EmptyEndpointMaxBufferSize,
		flushTimeout:               options.FlushTimeout,

		httpClient:         httpx.DefaultClient(),
		recordsBuffer:      make([]record, 0, options.DefaultRecordsBufferSize),
		recordsBufferMutex: sync.Mutex{},
		childWriter:        options.ChildWriter,
	}

	go func() {
		done := false
		for {
			if done {
				// we sleep less to avoid losing logs
				time.Sleep(20 * time.Millisecond)
			} else {
				select {
				case <-ctx.Done():
					done = true
				case <-time.After(time.Duration(writer.flushTimeout) * time.Millisecond):
				}
			}

			go func() {
				// TODO: as of now, if the HTTP request fail after X retries, we discard/lose the logs
				err := writer.flushLogs()
				if err != nil {
					log.Println(err.Error())
					return
				}
				// if err != nil {
				// 		writer.recordsBufferMutex.Lock()
				// 		writer.recordsBuffer = append(writer.recordsBuffer, recordsBufferCopy...)
				// 		writer.recordsBufferMutex.Unlock()
				// 	}
			}()
		}
	}()

	return writer
}

func defaultOptions() *WriterOptions {
	return &WriterOptions{
		LokiEndpoint:               "",
		ChildWriter:                os.Stdout,
		DefaultRecordsBufferSize:   100,
		EmptyEndpointMaxBufferSize: 200,
		FlushTimeout:               200,
	}
}

// SetEndpoint sets the loki endpoint. This method IS NOT thread safe.
// It should be used just after config is loaded
func (writer *Writer) SetEndpoint(lokiEndpoint string) {
	writer.lokiEndpoint = lokiEndpoint
}

func (writer *Writer) Write(data []byte) (n int, err error) {
	// TODO: handle error?
	_, _ = writer.childWriter.Write(data)

	// if log finishes by '\n' we trim it
	data = bytes.TrimSuffix(data, []byte("\n"))

	record := record{
		timestamp: time.Now().UTC(),
		message:   string(data),
	}

	writer.recordsBufferMutex.Lock()
	if writer.lokiEndpoint != "" || len(writer.recordsBuffer) < int(writer.emptyEndpointMaxBufferSize) {
		writer.recordsBuffer = append(writer.recordsBuffer, record)
	}
	writer.recordsBufferMutex.Unlock()

	return
}
