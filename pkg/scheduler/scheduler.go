package scheduler

import (
	"context"
	"database/sql"
	"time"

	"markdown.ninja/pkg/jwt"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/scheduler"
	"markdown.ninja/stdx/xxh3"
)

type Scheduler struct {
	queue queue.Queue
}

func Start(ctx context.Context, db db.DB, queue queue.Queue, jwtProvider *jwt.Provider, emailsService emails.Service, contactsService contacts.Service,
	siteService site.Service, kernelService kernel.PrivateService, contentService content.Service, storeService store.Service) error {
	logger := slogx.FromCtx(ctx)

	cronScheduler := scheduler.NewScheduler(&scheduler.ScheulderOptions{
		WithSeconds: true,
		Logger:      logger,
		// Verbose:     true,
	})

	scheduler := Scheduler{
		queue: queue,
	}

	postgresLockdId := int64(xxh3.Hash([]byte("scheduler")))

	leaderConnection := tryToBecomeLeader(ctx, db, postgresLockdId)
	if leaderConnection == nil {
		return nil
	}
	defer func() {
		leaderConnection.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", postgresLockdId)
		leaderConnection.Close()
	}()
	logger.Debug("scheduler: is leader")

	// every minutes
	err := cronScheduler.Schedule("emails.TaskSendScheduledNewsletters", "00 * * * * *", emailsService.TaskSendScheduledNewsletters)
	if err != nil {
		return err
	}

	// every minutes
	err = cronScheduler.Schedule("content.PublishPosts", "00 * * * * *", contentService.TaskPublishPages)
	if err != nil {
		return err
	}

	// every minutes
	err = cronScheduler.Schedule("emails.TaskSyncRefundsWithStripe", "00 * * * * *", storeService.TaskSyncRefundsWithStripe)
	if err != nil {
		return err
	}

	// every 6 hours
	err = cronScheduler.Schedule("contacts.TaskDeleteOldUnverifiedSessions", "00 01 */6 * * *", contactsService.TaskDeleteOldUnverifiedSessions)
	if err != nil {
		return err
	}

	// every hour at XX:10
	err = cronScheduler.Schedule("contacts.TaskSyncUnsubscribedContacts", "00 10 * * * *", contactsService.TaskSyncUnsubscribedContacts)
	if err != nil {
		return err
	}

	// every day at 00:00
	err = cronScheduler.Schedule("events.DispatchRotateAnonymousIDSalt", "0 0 0 * * *", scheduler.eventsDispatchRotateAnonymousIDSalt)
	if err != nil {
		return err
	}

	// every day at 01:00
	err = cronScheduler.Schedule("jwt.RotateKeys", "0 0 1 * * *", jwtProvider.RotateKeys)
	if err != nil {
		return err
	}

	// every day at 10:00
	err = cronScheduler.Schedule("organizations.DispatchInvoiceMonthlyUsage", "0 0 10 * * *", scheduler.organizationsDispatchInvoiceMonthlyUsage)
	if err != nil {
		return err
	}

	// every day at 02.00
	// _, err = cron.AddFunc("00 00 02 * * *", func() {
	// 	logger.Info(runningTaskMessage, slog.String("task", "contacts.TaskDeleteOldUnverifiedContacts"))
	// 	taskErr := cronScheduler.contactsService.TaskDeleteOldUnverifiedContacts(ctx)

	logger.Info("scheduler: Starting")
	err = cronScheduler.Start(ctx)
	if err != nil {
		return err
	}

	return nil
}

// tryToBecomeLeader returns a database connection if it successfully become leader. The connection
// MUST BE closed.
// It blocks until became leader or context is cancelled.
// It returns null if the context is cancelled and is not leader.
func tryToBecomeLeader(ctx context.Context, db db.DB, advisoryLockId int64) *sql.Conn {
	logger := slogx.FromCtx(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(1 * time.Second):
		}

		connection, err := db.Acquire(ctx)
		if err != nil {
			logger.Error("cronScheduler.tryToBecomeLeader: error acquiring connection", slogx.Err(err))
			if connection != nil {
				connection.Close()
			}
			continue
		}

		var isLeader bool
		err = connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, advisoryLockId).Scan(&isLeader)
		if err != nil {
			logger.Error("cronScheduler.tryToBecomeLeader: error querying advisory lock", slogx.Err(err))
			connection.Close()
			continue
		}

		if isLeader {
			return connection
		} else {
			connection.Close()
		}
	}
}
