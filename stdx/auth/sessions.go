package auth

import (
	"context"
	"time"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/uuid"
)

type Session struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time

	AccountID uuid.UUID
}

func CreateSession(ctx context.Context, db db.Queryer, accountID uuid.UUID) (token string, err error) {
	panic("TODO")
}

func GetSessionsForAccount(ctx context.Context, db db.Queryer, accountID uuid.UUID) (sessions []Session, err error) {
	panic("TODO")
}

func RefreshSession(ctx context.Context, db db.Queryer, oldToken string) (newToken string, err error) {
	panic("TODO")
}

func DeleteSession(ctx context.Context, db db.Queryer, token string) (err error) {
	panic("TODO")
}
