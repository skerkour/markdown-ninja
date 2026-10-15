package dbx

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type Queryer interface {
	sqlx.QueryerContext
	sqlx.ExecerContext
}

// Get single result or return an error.
func Get[T any](ctx context.Context, db Queryer, query string, args ...any) (result T, err error) {
	err = sqlx.GetContext(ctx, db, &result, query, args...)
	return
}

// Select creates slice of results based on SQL query. In case of zero results it will return non-nil empty slice.
func Select[T any](ctx context.Context, db Queryer, query string, args ...any) (results []T, err error) {
	results = make([]T, 0)

	err = sqlx.SelectContext(ctx, db, &results, query, args...)
	return
}

// Exec executes a query without returning any rows. The args are for any placeholder parameters in the query.
func Exec(ctx context.Context, db Queryer, query string, args ...any) (sql.Result, error) {
	return db.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows, typically a SELECT. The args are for any placeholder
// parameters in the query.
func Query(ctx context.Context, db Queryer, query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(ctx, query, args...)
}
