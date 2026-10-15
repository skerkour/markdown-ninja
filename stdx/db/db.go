package db

import (
	"context"
	"database/sql"
	"time"

	// import pgx driver
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// Queryer allows to query a database.
type Queryer interface {
	Get(ctx context.Context, dest any, query string, args ...any) error
	Select(ctx context.Context, dest any, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Rebind(query string) (ret string)
}

// DB represents a pool of zero or more underlying connections. It must be safe for concurrent use
// by multiple goroutines.
type DB interface {
	Ping(ctx context.Context) error
	SetMaxIdleConns(n int)
	SetMaxOpenConns(n int)
	SetConnMaxIdleTime(d time.Duration)
	SetConnMaxLifetime(d time.Duration)
	Stats() sql.DBStats
	Acquire(ctx context.Context) (conn *sql.Conn, err error)
	Close() error
	Queryer
	Txer
}

// Txer is the ability to start transactions
type Txer interface {
	Begin(ctx context.Context) (Tx, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
	Transaction(ctx context.Context, fn func(tx Tx) error) (err error)
}

// Tx represents an in-progress database transaction.
type Tx interface {
	Commit() error
	Rollback() error
	Queryer
}

// Connect to a database and verify the connections with a ping.
// See https://www.alexedwards.net/blog/configuring-sqldb
// and https://making.pusher.com/production-ready-connection-pooling-in-go
// https://brandur.org/fragments/postgres-parameters
// for the details
func Connect(databaseURL string, poolSize int) (dbPool *Database, err error) {
	sqlxDB, err := sqlx.Connect("pgx", databaseURL)
	if err != nil {
		return
	}

	dbPool = &Database{
		sqlxDB: sqlxDB,
	}

	dbPool.SetMaxOpenConns(poolSize)
	dbPool.SetMaxIdleConns(poolSize)
	// dbPool.SetConnMaxIdleTime(30 * time.Minute)

	err = dbPool.Ping(context.Background())
	if err != nil {
		return
	}

	return dbPool, nil
}
