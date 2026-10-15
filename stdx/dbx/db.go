package dbx

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

// Database is wrapper of `sqlx.DB` which implements `DB`
type Database struct {
	sqlxDB *sqlx.DB
}

// Connect to a database and verify the connections with a ping.
// See https://www.alexedwards.net/blog/configuring-sqldb
// and https://making.pusher.com/production-ready-connection-pooling-in-go
// https://brandur.org/fragments/postgres-parameters
// for the details
func Connect(databaseURL string, poolSize int) (ret *Database, err error) {
	sqlxDB, err := sqlx.Connect("pgx", databaseURL)
	if err != nil {
		return
	}

	ret = &Database{
		sqlxDB: sqlxDB,
	}

	ret.SetMaxOpenConns(poolSize)
	ret.SetMaxIdleConns(int(poolSize / 2))
	ret.SetConnMaxLifetime(30 * time.Minute)
	return
}

// Begin starts a transaction. The default isolation level is dependent on the driver.
// The provided context is used until the transaction is committed or rolled back. If the context is
// canceled, the sql package will roll back the transaction. Tx.Commit will return an error if the
// context provided to BeginTx is canceled.
func (db *Database) Begin(ctx context.Context) (*Tx, error) {
	sqlxTx, err := db.sqlxDB.BeginTxx(ctx, nil)
	return &Tx{sqlxTx}, err
}

// BeginTx starts a transaction.
//
// The provided context is used until the transaction is committed or rolled back. If the context is
// canceled, the sql package will roll back the transaction. Tx.Commit will return an error if the
// context provided to BeginTx is canceled.
//
// The provided TxOptions is optional and may be nil if defaults should be used. If a non-default
// isolation level is used that the driver doesn't support, an error will be returned.
func (db *Database) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	sqlxTx, err := db.sqlxDB.BeginTxx(ctx, opts)
	return &Tx{sqlxTx}, err
}

// Ping verifies a connection to the database is still alive, establishing a connection if necessary.
func (db *Database) Ping(ctx context.Context) error {
	return db.sqlxDB.PingContext(ctx)
}

// SetConnMaxLifetime sets the maximum amount of time a connection may be reused.
func (db *Database) SetConnMaxLifetime(d time.Duration) {
	db.sqlxDB.SetConnMaxLifetime(d)
}

// SetMaxIdleConns sets the maximum number of connections in the idle connection pool.
func (db *Database) SetMaxIdleConns(n int) {
	db.sqlxDB.SetMaxIdleConns(n)
}

// SetMaxOpenConns sets the maximum number of open connections to the database.
func (db *Database) SetMaxOpenConns(n int) {
	db.sqlxDB.SetMaxOpenConns(n)
}

// Stats returns database statistics.
func (db *Database) Stats() sql.DBStats {
	return db.sqlxDB.Stats()
}

// Exec executes a query without returning any rows. The args are for any placeholder parameters in the query.
func (db *Database) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.sqlxDB.ExecContext(ctx, query, args...)
}

// Get a single record. Any placeholder parameters are replaced with supplied args. An `ErrNoRows`
// error is returned if the result set is empty.
func (db *Database) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return db.sqlxDB.GetContext(ctx, dest, query, args...)
}

// QueryContext executes a query that returns rows, typically a SELECT. The args are for any placeholder
// parameters in the query.
func (db *Database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.sqlxDB.QueryContext(ctx, query, args...)
}

// Select an array of records. Any placeholder parameters are replaced with supplied args.
func (db *Database) Select(ctx context.Context, dest any, query string, args ...any) error {
	return db.sqlxDB.SelectContext(ctx, dest, query, args...)
}

func (db *Database) QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row {
	return db.sqlxDB.QueryRowxContext(ctx, query, args...)
}

func (db *Database) QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	return db.sqlxDB.QueryxContext(ctx, query, args...)
}
