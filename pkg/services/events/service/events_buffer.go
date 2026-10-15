package service

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
)

const EVENTS_BUFFER_SIZE = 50_000

// drop events if buffer is larger than that
const EVENTS_BUFFER_MAX_SIZE = 500_000_000

const (
	eventBufferFlushTimeout = 50 * time.Millisecond
)

type eventsBuffer struct {
	mutex  sync.Mutex
	events []events.Event
}

func (buffer *eventsBuffer) Push(event events.Event) {
	buffer.mutex.Lock()
	if len(buffer.events) < EVENTS_BUFFER_MAX_SIZE {
		buffer.events = append(buffer.events, event)
	}
	buffer.mutex.Unlock()
}

func (buffer *eventsBuffer) PushMany(events []events.Event) {
	buffer.mutex.Lock()
	if len(buffer.events)+len(events) < EVENTS_BUFFER_MAX_SIZE {
		buffer.events = append(buffer.events, events...)
	} else if freeSpace := EVENTS_BUFFER_MAX_SIZE - len(buffer.events); freeSpace > 0 && freeSpace < len(events) {
		buffer.events = append(buffer.events, events[:freeSpace]...)
	}
	buffer.mutex.Unlock()
}

func (buffer *eventsBuffer) Flush() []events.Event {
	buffer.mutex.Lock()
	if len(buffer.events) == 0 {
		buffer.mutex.Unlock()
		return []events.Event{}
	}
	ret := make([]events.Event, len(buffer.events))
	copy(ret, buffer.events)
	buffer.events = make([]events.Event, 0, EVENTS_BUFFER_SIZE)
	buffer.mutex.Unlock()
	return ret
}

func newEventsBuffer() eventsBuffer {
	return eventsBuffer{
		mutex:  sync.Mutex{},
		events: make([]events.Event, 0, EVENTS_BUFFER_SIZE),
	}
}

func (service *Service) flushEventsBufferInBackground(ctx context.Context, logger *slog.Logger) {
	go func() {
		done := false
		for {
			if done {
				// we sleep less to avoid losing events
				time.Sleep(20 * time.Millisecond)
			} else {
				select {
				case <-ctx.Done():
					logger.Info("analytics: Shutting down events flushing")
					done = true
				case <-time.After(eventBufferFlushTimeout):
				}
			}

			events := service.eventsBuffer.Flush()
			if len(events) != 0 {
				go service.saveBufferedEvents(context.Background(), events)
			}
		}
	}()
}

func (service *Service) saveBufferedEvents(ctx context.Context, eventsInput []events.Event) {
	if len(eventsInput) == 0 {
		return
	}

	err := service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		for eventsChunk := range slices.Chunk(eventsInput, 25_000) {
			txErr = service.repo.SaveEvents(ctx, tx, eventsChunk)
			if txErr != nil {
				return txErr
			}
		}

		return nil
	})
	if err != nil {
		// if an error happened, we buffer back the events
		service.eventsBuffer.PushMany(eventsInput)
	}
}
