package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"markdown.ninja/stdx/log/slogx"
)

type Migration struct {
	ID   int64
	Name string
	Up   func(ctx context.Context, tx Queryer) (err error)
	Down func(ctx context.Context, tx Queryer) (err error)
}

func Migrate(ctx context.Context, logger *slog.Logger, db DB, migrations []Migration) (err error) {
	if logger == nil {
		err = errors.New("migrate.Migrate: logger is null")
		return
	}

	logger.Debug("migrate: Creating/checking migrations table...")

	err = createMigrationTable(ctx, db)
	if err != nil {
		return err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("migrate.Migrate: Starting DB transaction: %w", err)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(ctx, "LOCK TABLE migrations IN ACCESS EXCLUSIVE MODE")
	if err != nil {
		err = fmt.Errorf("migrate.Migrate: Locking table: %w", err)
		return
	}

	for _, migration := range migrations {
		var found string

		err = tx.Get(ctx, &found, "SELECT id FROM migrations WHERE id = $1 FOR UPDATE", migration.ID)
		switch err {
		case sql.ErrNoRows:
			logger.Info("migrate: Running migration", slog.Int64("migrations.id", migration.ID), slog.String("migration.name", migration.Name))
			// we need to run the migration so we continue to code below
		case nil:
			logger.Debug("migrate: Skipping migration", slog.Int64("migrations.id", migration.ID), slog.String("migration.name", migration.Name))
			continue
		default:
			err = fmt.Errorf("migrate.Migrate: looking up migration by id: %w", err)
			return
		}

		_, err = tx.Exec(ctx, "INSERT INTO migrations (id) VALUES ($1)", migration.ID)
		if err != nil {
			err = fmt.Errorf("migrate.Migrate: inserting migration: %w", err)
			return
		}

		err = migration.Up(ctx, tx)
		if err != nil {
			err = fmt.Errorf("migrate.Migrate: executing migration (migration id = %d): %w", migration.ID, err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("migrate: Committing transaction: %w", err)
		return
	}

	return
}

// Rollback undo the latest migration
func Rollback(ctx context.Context, db DB, migrations []Migration, numberToRollback int64) (err error) {
	logger := slogx.FromCtx(ctx)
	if logger == nil {
		err = errors.New("migrate.Rollback: logger is missing from context")
		return
	}

	logger.Info("migrate: Creating/checking migrations table...")

	err = createMigrationTable(ctx, db)
	if err != nil {
		return err
	}

	// reverse migration
	sort.SliceStable(migrations, func(i, j int) bool {
		return migrations[i].ID > migrations[j].ID
	})

	tx, err := db.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("migrate: Starting DB transaction: %w", err)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(ctx, "LOCK TABLE migrations IN ACCESS EXCLUSIVE MODE")
	if err != nil {
		err = fmt.Errorf("migrate.Rollback: Locking table: %w", err)
		return
	}

	for i := int64(0); i < numberToRollback; i += 1 {
		migration := migrations[i]

		var found string
		err = tx.Get(ctx, &found, "SELECT id FROM migrations WHERE id = $1", migration.ID)
		switch err {
		case sql.ErrNoRows:
			logger.Info("migrate: Skipping rollback", slog.Int64("migration.id", migration.ID), slog.String("migration.name", migration.Name))

			continue
		case nil:
			logger.Info("migrate: Running rollback", slog.Int64("migration.id", migration.ID), slog.String("migration.name", migration.Name))

			// we need to run the rollback so we continue to code below
		default:
			err = fmt.Errorf("migrate.Rollback: looking up rollback by id: %w", err)
			return
		}

		_, err = tx.Exec(ctx, "DELETE FROM migrations WHERE id=$1", migration.ID)
		if err != nil {
			err = fmt.Errorf("migrate.Rollback: deleting migration: %w", err)
			return
		}

		err = migration.Down(ctx, tx)
		if err != nil {
			err = fmt.Errorf("migrate.Rollback: executing rollback (migration id = %d): %w", migration.ID, err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("migrate.Rollback: Committing transaction: %w", err)
		return
	}

	return
}

func createMigrationTable(ctx context.Context, db DB) error {
	_, err := db.Exec(ctx, "CREATE TABLE IF NOT EXISTS migrations (id BIGINT PRIMARY KEY )")
	if err != nil {
		return fmt.Errorf("migrate: Creating migrations table: %w", err)
	}
	return nil
}
