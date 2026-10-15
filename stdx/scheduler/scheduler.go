package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"markdown.ninja/stdx/cron"
)

type Scheduler struct {
	logger     *slog.Logger
	cronParser cron.Parser
	tasks      map[string]task
	verbose    bool
}

type task struct {
	cronExpression string
	handler        TaskHandler
}

type TaskHandler = func(ctx context.Context)

type ScheulderOptions struct {
	// default: false
	WithSeconds bool
	// default: false
	Verbose bool
	Logger  *slog.Logger
}

func NewScheduler(options *ScheulderOptions) *Scheduler {
	defaultOptions := defaultOptions()
	cronParser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	if options == nil {
		options = defaultOptions
	}

	if options.WithSeconds {
		cronParser = cron.NewParser(
			cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		)
	}

	return &Scheduler{
		cronParser: cronParser,
		tasks:      map[string]task{},
		verbose:    options.Verbose,

		logger: options.Logger,
	}
}

func defaultOptions() *ScheulderOptions {
	return &ScheulderOptions{
		Logger:      nil,
		WithSeconds: false,
		Verbose:     false,
	}
}

func (scheduler *Scheduler) Schedule(taskName, cronExpression string, handler TaskHandler) (err error) {
	_, err = scheduler.cronParser.Parse(cronExpression)
	if err != nil {
		err = fmt.Errorf("scheduler: cron expression is not valid: %w", err)
		return
	}

	if _, taskAlreadyExists := scheduler.tasks[taskName]; taskAlreadyExists {
		err = fmt.Errorf("scheduler: task already exists: %s", taskName)
		return
	}

	scheduler.tasks[taskName] = task{
		cronExpression,
		handler,
	}
	return
}

func (scheduler *Scheduler) Start(ctx context.Context) (err error) {
	cron := cron.New(cron.WithParser(scheduler.cronParser))

	for taskName, task := range scheduler.tasks {
		err = scheduler.scheduleTask(ctx, cron, taskName, task)
		if err != nil {
			return
		}
	}

	cron.Start()

	<-ctx.Done()

	if scheduler.logger != nil {
		scheduler.logger.Info("scheduler: Shutting down")
	}

	cronCtx := cron.Stop()
	cronCtx, cancel := context.WithTimeout(cronCtx, 10*time.Second)
	defer cancel()

	<-cronCtx.Done()

	return
}

func (scheduler *Scheduler) scheduleTask(ctx context.Context, cron *cron.Cron, taskName string, task task) (err error) {
	_, err = cron.AddFunc(task.cronExpression, func() {
		if scheduler.verbose && scheduler.logger != nil {
			scheduler.logger.Info("scheduler: running task", slog.String("task", taskName))
		}
		task.handler(ctx)
	})
	if err != nil {
		err = fmt.Errorf("scheduler: error scheduling  task %s: %w", taskName, err)
		return
	}

	return
}
