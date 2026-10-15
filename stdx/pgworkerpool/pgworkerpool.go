package pgworkerpool

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/pgqueue"
	"markdown.ninja/stdx/retry"
)

type JobHandler[I pgqueue.JobData] func(ctx context.Context, input I) (err error)

type internalJobHandler = func(ctx context.Context, payload []byte) (err error)

type WorkerPool struct {
	queue          pgqueue.Queue
	concurrencyMax uint32
	jobHandlers    map[string]internalJobHandler
	logger         *slog.Logger
	onError        func(ctx context.Context, job pgqueue.Job, err error)
}

type Options struct {
	// default: 200
	ConcurrencyMax uint32
	Logger         *slog.Logger
	// The default OnError handler is to log the error
	OnError func(ctx context.Context, job pgqueue.Job, err error)
}

func NewPool(inputQueue pgqueue.Queue, options *Options) (*WorkerPool, error) {
	opts := Options{
		ConcurrencyMax: 200,
		Logger:         slog.New(slogx.NewDiscardHandler()),
	}

	if options != nil {
		if options.ConcurrencyMax != 0 {
			if options.ConcurrencyMax > math.MaxInt32 {
				return nil, fmt.Errorf("pgworkerpool: concurrencyMax can't be > %d", math.MaxInt32)
			}

			opts.ConcurrencyMax = options.ConcurrencyMax
		}

		if options.Logger != nil {
			opts.Logger = options.Logger
		}

		if options.OnError != nil {
			opts.OnError = options.OnError
		}
	}

	if opts.OnError == nil {
		opts.OnError = func(ctx context.Context, job pgqueue.Job, err error) {
			opts.Logger.Error("pgworkerpool: job failed", slogx.Err(err),
				slog.Group("job",
					slog.String("job.id", job.ID.String()), slog.String("type", job.Type),
				),
			)
		}
	}

	workerPool := &WorkerPool{
		queue:          inputQueue,
		jobHandlers:    make(map[string]internalJobHandler),
		logger:         opts.Logger,
		concurrencyMax: opts.ConcurrencyMax,
		onError:        opts.OnError,
	}
	return workerPool, nil
}

func (workerPool *WorkerPool) AddHandler[T pgqueue.JobData](handler JobHandler[T]) {
	var _jobData T
	jobType := _jobData.JobType()

	if _, exists := workerPool.jobHandlers[jobType]; exists {
		panic(fmt.Sprintf("pgworkerpool: job handler already exists for %s", jobType))
	}

	workerPool.jobHandlers[jobType] = func(ctx context.Context, payload []byte) (err error) {
		var input T

		err = json.Unmarshal(payload, &input)
		if err != nil {
			return fmt.Errorf("pgworkerpool: error unmarshalling job data: %w", err)
		}
		return handler(ctx, input)
	}
}

func (workerPool *WorkerPool) Start(ctx context.Context) {
	jobsChan := make(chan pgqueue.Job, workerPool.concurrencyMax)
	var wg sync.WaitGroup

	wg.Add(int(workerPool.concurrencyMax))

	for i := uint32(0); i < workerPool.concurrencyMax; i += 1 {
		go func(ctx context.Context, jobs <-chan pgqueue.Job) {
			defer wg.Done()
			for job := range jobs {
				workerPool.handleJob(ctx, job)
			}
		}(ctx, jobsChan)
	}

	workerPool.logger.Info("pgworkerpool: Starting", slog.Uint64("concurrencyMax", uint64(workerPool.concurrencyMax)))

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			workerPool.logger.Info("pgworkerpool: Shutting down")
			close(jobsChan)
			workerPool.queue.Stop()
			wg.Wait()
			return
		case <-ticker.C:
		}

		jobs, err := workerPool.queue.Pull(ctx, uint64(workerPool.concurrencyMax))
		if err != nil {
			workerPool.logger.Error("pgworkerpool: error pulling jobs from queue", slog.String("err", err.Error()))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, job := range jobs {
			jobsChan <- job
		}
	}
}

func (workerPool *WorkerPool) handleJob(ctx context.Context, job pgqueue.Job) {
	jobHandler, jobHandlerExists := workerPool.jobHandlers[job.Type]
	if !jobHandlerExists {
		err := errors.New("pgworkerpool: job handler not found")
		workerPool.onError(ctx, job, err)
		err = retry.Do(func() error {
			// We use a context.Background() instead of ctx to delete  the job fail even if the context is cancelled
			return workerPool.queue.FailJob(context.Background(), job.ID)
		}, retry.Context(context.Background()), retry.Attempts(5), retry.Delay(50*time.Millisecond), retry.MaxDelay(time.Second))
		if err != nil {
			workerPool.logger.Error("pgworkerpool: error marking job as failed", slog.String("job.id", job.ID.String()),
				slogx.Err(err))
		}
		return
	}

	err := jobHandler(ctx, job.DataJson)
	if err != nil {
		workerPool.onError(ctx, job, err)
		err = retry.Do(func() error {
			// We use a context.Background() instead of ctx to delete  the job fail even if the context is cancelled
			return workerPool.queue.FailJob(context.Background(), job.ID)
		}, retry.Context(context.Background()), retry.Attempts(5), retry.Delay(50*time.Millisecond), retry.MaxDelay(time.Second))
		if err != nil {
			workerPool.logger.Error("pgworkerpool: error marking job as failed", slog.String("job.id", job.ID.String()),
				slogx.Err(err))
		}
		return
	}

	err = retry.Do(func() error {
		// We use a context.Background() instead of ctx to delete  the job fail even if the context is cancelled
		return workerPool.queue.DeleteJob(context.Background(), job.ID)
	}, retry.Context(context.Background()), retry.Attempts(5), retry.Delay(50*time.Millisecond), retry.MaxDelay(time.Second))
	if err != nil {
		workerPool.logger.Error("pgworkerpool: error deleting job", slog.String("job.id", job.ID.String()),
			slogx.Err(err))
	}
}
