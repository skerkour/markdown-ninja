package kernel

import (
	"context"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/uuid"
)

type Service interface {
	PublicService
	PrivateService
}

type PublicService interface {
	// The Init function is called by the frontend on load to load the data it needs
	Init(ctx context.Context, input EmptyInput) (ret InitData, err error)

	// Handlers
	Healthcheck(ctx context.Context, input EmptyInput) (err error)
	HandlePingooWebhook(ctx context.Context, event pingoo.Event) (err error)

	// Queue
	ListFailedBackgroundJobs(ctx context.Context, input EmptyInput) (jobs PaginatedResult[queue.Job], err error)
	DeleteBackgroundJob(ctx context.Context, input DeleteBackgroundJobInput) (err error)
}

type PrivateService interface {
	// utils
	CurrentUserID(ctx context.Context) (userID uuid.UUID, err error)
	ValidateEmail(ctx context.Context, emailAddress string, rejectBlockedDomains bool) (err error)
	// SleepAuth sleeps for a small random amount of time to prevent timing attacks
	// and bruteforce
	SleepAuth()
	// SleepAuthFailure sleeps for a random amount of time to prevent timing attacks
	// and bruteforce
	SleepAuthFailure()
	ValidateColor(color string) (err error)

	// Admin
	// AdminGetAllUsers(ctx context.Context) (users []User, err error)
	// AdminBlockUser(ctx context.Context, input AdminBlockUserInput) (err error)
	// AdminUnblockUser(ctx context.Context, input AdminUnblockUserInput) (err error)
	// AdminFindUser(ctx context.Context, userID guid.GUID) (user User, err error)

	// Jobs

	// Tasks
}
