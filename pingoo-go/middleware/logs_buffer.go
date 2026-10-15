package middleware

import (
	"context"
	"sync"
	"time"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/stdx/retry"
)

const (
	logsBufferSize = 5_000
)

type logsBuffer struct {
	mutex        sync.Mutex
	logs         []pingoo.HttpLogRecord
	pingooClient *pingoo.Client
}

func newlogsBuffer(pingooClient *pingoo.Client) logsBuffer {
	return logsBuffer{
		mutex:        sync.Mutex{},
		logs:         make([]pingoo.HttpLogRecord, 0, 10_000),
		pingooClient: pingooClient,
	}
}

func (buffer *logsBuffer) Push(log pingoo.HttpLogRecord) {
	buffer.mutex.Lock()
	buffer.logs = append(buffer.logs, log)
	buffer.mutex.Unlock()
}

func (buffer *logsBuffer) PushMany(logs []pingoo.HttpLogRecord) {
	buffer.mutex.Lock()
	buffer.logs = append(buffer.logs, logs...)
	buffer.mutex.Unlock()
}

func (buffer *logsBuffer) Flush() []pingoo.HttpLogRecord {
	buffer.mutex.Lock()
	if len(buffer.logs) == 0 {
		buffer.mutex.Unlock()
		return []pingoo.HttpLogRecord{}
	}
	ret := make([]pingoo.HttpLogRecord, len(buffer.logs))
	copy(ret, buffer.logs)
	buffer.logs = make([]pingoo.HttpLogRecord, 0, logsBufferSize)
	buffer.mutex.Unlock()
	return ret
}

func (buffer *logsBuffer) flushInBackground(ctx context.Context) {
	if buffer.pingooClient == nil {
		return
	}

	done := false
	for {
		if done {
			// we sleep less to avoid losing events
			time.Sleep(50 * time.Millisecond)
		} else {
			select {
			case <-ctx.Done():
				done = true
			case <-time.After(time.Second):
			}
		}

		logs := buffer.Flush()
		if len(logs) == 0 {
			continue
		}

		go func() {
			_ = retry.Do(func() error {
				return buffer.pingooClient.PushHttpLogs(context.Background(), logs)
			}, retry.Context(context.Background()), retry.Attempts(3), retry.Delay(100*time.Millisecond), retry.DelayType(retry.FixedDelay))

			// TODO: what to do?
			// if the backend returns an error because input is not valid then by re-pushing the logs
			// we are going tojust loop the error and "leak" our memory
			// if err != nil {
			// 	buffer.PushMany(logs)
			// }
		}()
	}
}
