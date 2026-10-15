package dbx

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// Transaction is wrapper of `sqlx.Tx` which implements `Tx`
type Tx struct {
	sqlxTx *sqlx.Tx
}

// Commit commits the transaction.
func (tx *Tx) Commit() error {
	return tx.sqlxTx.Commit()
}

// Rollback aborts the transaction.
func (tx *Tx) Rollback() error {
	return tx.sqlxTx.Rollback()
}

// Exec executes a query without returning any rows. The args are for any placeholder parameters in the query.
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.sqlxTx.ExecContext(ctx, query, args...)
}

// Get a single record. Any placeholder parameters are replaced with supplied args. An `ErrNoRows`
// error is returned if the result set is empty.
func (tx *Tx) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return tx.sqlxTx.GetContext(ctx, dest, query, args...)
}

// QueryContext executes a query that returns rows, typically a SELECT. The args are for any placeholder
// parameters in the query.
func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.sqlxTx.QueryContext(ctx, query, args...)
}

// Select an array of records. Any placeholder parameters are replaced with supplied args.
func (tx *Tx) Select(ctx context.Context, dest any, query string, args ...any) error {
	return tx.sqlxTx.SelectContext(ctx, dest, query, args...)
}

func (tx *Tx) QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row {
	return tx.sqlxTx.QueryRowxContext(ctx, query, args...)
}

func (tx *Tx) QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	return tx.sqlxTx.QueryxContext(ctx, query, args...)
}
