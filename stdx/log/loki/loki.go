package loki

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/retry"
)

type lokiPushRequest struct {
	Streams []lokiPushStream `json:"streams"`
}

type lokiPushStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// https://grafana.com/docs/loki/latest/reference/api/#push-log-entries-to-loki
func (writer *Writer) flushLogs() (err error) {
	ctx := context.Background()

	writer.recordsBufferMutex.Lock()

	if len(writer.recordsBuffer) == 0 || writer.lokiEndpoint == "" {
		writer.recordsBufferMutex.Unlock()
		return
	}

	recordsBufferCopy := make([]record, len(writer.recordsBuffer))
	copy(recordsBufferCopy, writer.recordsBuffer)
	writer.recordsBuffer = make([]record, 0, writer.defaultRecordsBufferSize)
	writer.recordsBufferMutex.Unlock()

	lokiRecords := convertRecords(writer.streams, recordsBufferCopy)
	requestBody, err := encodeRecords(lokiRecords)
	if err != nil {
		return
	}

	err = retry.Do(func() error {
		return writer.pushLogs(ctx, requestBody)
	},
		retry.Context(ctx),
		retry.Attempts(20),
		retry.Delay(1*time.Second),
		retry.DelayType(retry.CombineDelay(retry.FixedDelay, retry.RandomDelay)),
	)

	return
}

func (writer *Writer) pushLogs(ctx context.Context, requestBody []byte) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, writer.lokiEndpoint, bytes.NewReader(requestBody))
	if err != nil {
		err = fmt.Errorf("loki: flushing logs: error creating HTTP request: %w", err)
		return
	}

	req.Header.Add(httpx.HeaderContentType, httpx.MediaTypeJson)
	req.Header.Add(httpx.HeaderContentEncoding, "gzip")

	res, err := writer.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("loki: flushing logs: making HTTP request: %w", err)
		return err
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return
}

func convertRecords(streams map[string]string, records []record) lokiPushRequest {
	ret := lokiPushRequest{
		Streams: []lokiPushStream{
			{
				Stream: streams,
				Values: make([][]string, 0, len(records)),
			},
		},
	}

	for _, record := range records {
		lokiRecord := make([]string, 2)
		lokiRecord[0] = strconv.Itoa(int(record.timestamp.UnixNano()))
		lokiRecord[1] = record.message
		ret.Streams[0].Values = append(ret.Streams[0].Values, lokiRecord)
	}

	return ret
}

// encodeRecords to gzipped JSON
func encodeRecords(input lokiPushRequest) (ret []byte, err error) {
	// we expect the average log line to be 90 bytes after compression
	buffer := bytes.NewBuffer(make([]byte, 0, len(input.Streams[0].Values)*90))
	gzipWriter := gzip.NewWriter(buffer)
	jsonEncoder := json.NewEncoder(gzipWriter)
	err = jsonEncoder.Encode(input)
	if err != nil {
		err = fmt.Errorf("loki: error encoding logs to JSON: %w", err)
		return
	}
	err = gzipWriter.Close()
	if err != nil {
		err = fmt.Errorf("loki: error closing the Gzip writer: %w", err)
		return
	}

	return buffer.Bytes(), nil
}
