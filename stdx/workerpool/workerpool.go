package workerpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/retry"
)

type JobHandler[I queue.JobData] func(ctx context.Context, input I) (err error)

type internalJobHandler = func(ctx context.Context, payload []byte) (err error)

type WorkerPool struct {
	queue          queue.Queue
	concurrencyMax uint32
	jobHandlers    map[string]internalJobHandler
	logger         *slog.Logger
	onError        func(ctx context.Context, job queue.Job, err error)
}

type Options struct {
	// default: 200
	ConcurrencyMax uint32
	Logger         *slog.Logger
	// The default OnError handler is to log the error
	OnError func(ctx context.Context, job queue.Job, err error)
}

func NewPool(inputQueue queue.Queue, options *Options) (worker *WorkerPool, err error) {
	opts := Options{
		ConcurrencyMax: 200,
		Logger:         slog.New(slogx.NewDiscardHandler()),
	}

	if options.ConcurrencyMax != 0 {
		if options.ConcurrencyMax > math.MaxInt32 {
			err = fmt.Errorf("workerpool: concurrencyMax can't be > %d", math.MaxInt32)
			return
		}

		opts.ConcurrencyMax = options.ConcurrencyMax
	}

	if options.Logger != nil {
		opts.Logger = options.Logger
	}

	if options.OnError != nil {
		opts.OnError = options.OnError
	} else {
		// default error handler
		opts.OnError = func(ctx context.Context, job queue.Job, err error) {
			opts.Logger.Error("workerpool: job failed", slogx.Err(err),
				slog.Group("job",
					slog.String("job.id", job.ID.String()), slog.String("type", job.Type),
				),
			)
		}
	}

	worker = &WorkerPool{
		queue:       inputQueue,
		jobHandlers: make(map[string]internalJobHandler),

		logger:         options.Logger,
		concurrencyMax: opts.ConcurrencyMax,
		onError:        opts.OnError,
	}
	return
}

func AddHandler[T queue.JobData](workerPool *WorkerPool, handler JobHandler[T]) {
	var _jobData T
	jobType := _jobData.JobType()

	if _, exists := workerPool.jobHandlers[jobType]; exists {
		panic(fmt.Sprintf("workerpool: job handler already exists for %s", jobType))
	}

	workerPool.jobHandlers[jobType] = func(ctx context.Context, payload []byte) (err error) {
		var input T

		err = json.Unmarshal(payload, &input)
		// jsonDecoder.DisallowUnknownFields()
		if err != nil {
			err = fmt.Errorf("workerpool: error decoding job data: %w", err)
			return
		}
		return handler(ctx, input)
	}
}

func (workerPool *WorkerPool) Start(ctx context.Context) {
	jobsChan := make(chan queue.Job, workerPool.concurrencyMax)
	var wg sync.WaitGroup

	wg.Add(int(workerPool.concurrencyMax))

	// Start the background workers
	for i := uint32(0); i < workerPool.concurrencyMax; i += 1 {
		go func(ctx context.Context, jobs <-chan queue.Job) {
			defer wg.Done()
			for job := range jobs {
				workerPool.handleJob(ctx, job)
			}
		}(ctx, jobsChan)
	}

	workerPool.logger.Info("workerpool: Starting", slog.Uint64("concurrencyMax", uint64(workerPool.concurrencyMax)))

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			workerPool.logger.Info("workerpool: Shutting down")
			close(jobsChan)
			// workerPool.queue.Stop(ctx)
			wg.Wait()
			return
		case <-ticker.C:
			// we "sleep" a little bit to avoid consumming too much CPU
		}

		jobs, err := workerPool.queue.Pull(ctx, uint64(workerPool.concurrencyMax))
		if err != nil {
			workerPool.logger.Error("workerpool: error pulling jobs from queue", slog.String("err", err.Error()))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, job := range jobs {
			jobsChan <- job
		}
	}
}

func (workerPool *WorkerPool) handleJob(ctx context.Context, job queue.Job) {
	var err error

	jobHandler, jobHandlerExists := workerPool.jobHandlers[job.Type]
	if !jobHandlerExists {
		err = errors.New("workerpool: job handler not found")
		goto failjob
	}

	err = jobHandler(ctx, job.RawData)
	if err != nil {
		goto failjob
	}

	err = retry.Do(func() error {
		// We use a context.Background() instead of ctx to delete  the job fail even if the context is cancelled
		return workerPool.queue.DeleteJob(context.Background(), job.ID)
	}, retry.Context(context.Background()), retry.Attempts(3), retry.Delay(50*time.Millisecond), retry.MaxDelay(100*time.Millisecond))
	if err != nil {
		workerPool.logger.Error("workerpool: error deleting job", slog.String("job.id", job.ID.String()),
			slogx.Err(err))
	}
	return

failjob:
	workerPool.onError(ctx, job, err)
	// We use a context.Background() instead of ctx to let the job fail even if the context is cancelled
	err = workerPool.queue.FailJob(context.Background(), job)
	if err != nil {
		workerPool.logger.Error("workerpool: error marking job as failed", slog.String("job.id", job.ID.String()),
			slogx.Err(err))
		return
	}
}
