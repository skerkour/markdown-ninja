package db

import (
	"context"
	"fmt"
)

type currentDB struct {
	CurrentDatabase string `db:"current_database"`
}

func InitTimescale(ctx context.Context, db DB) (err error) {
	_, err = db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS timescaledb")
	if err != nil {
		err = fmt.Errorf("timescaledb: Initializing the Postgres extension: %w", err)
		return
	}

	// var currentDbName currentDB
	// err = db.Get(ctx, &currentDbName, "SELECT current_database()")
	// if err != nil {
	// 	err = fmt.Errorf("timescaledb: getting current ddatabase name: %w", err)
	// 	return
	// }

	// // https://docs.timescale.com/self-hosted/latest/configuration/telemetry/
	// // _, err = db.Exec(ctx, "ALTER SYSTEM SET timescaledb.telemetry_level=off")
	// // DANGER: this pattern is vulnerable to SQL injections.
	// // here it's okay because we use controlled database name
	// // TODO: improve
	// _, err = db.Exec(ctx, fmt.Sprintf("ALTER DATABASE %s SET timescaledb.telemetry_level=off", currentDbName.CurrentDatabase))
	// if err != nil {
	// 	err = fmt.Errorf("timescaledb: Turning off telemetry: %w", err)
	// 	return
	// }

	return
}
