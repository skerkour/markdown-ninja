package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"log/slog"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
)

type Migration struct {
	ID   int64
	Up   func(ctx context.Context, tx db.Queryer) (err error)
	Down func(ctx context.Context, tx db.Queryer) (err error)
}

func Migrate(ctx context.Context, db db.DB, migrations []Migration) (err error) {
	logger := slogx.FromCtx(ctx)
	if logger == nil {
		err = errors.New("migrate.Migrate: logger is missing from context")
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
			logger.Info("migrate: Running migration", slog.Int64("migrations.id", migration.ID))
			// we need to run the migration so we continue to code below
		case nil:
			logger.Debug("migrate: Skipping migration", slog.Int64("migrations.id", migration.ID))
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
func Rollback(ctx context.Context, db db.DB, migrations []Migration, numberToRollback int64) (err error) {
	logger := slogx.FromCtx(ctx)
	if logger == nil {
		err = errors.New("migrate.Rollback: logger is missing from context")
		return
	}

	logger.Debug("migrate: Creating/checking migrations table...")
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
			logger.Debug("migrate: Skipping rollback", slog.Int64("migration.id", migration.ID))
			continue
		case nil:
			logger.Info("migrate: Running rollback", slog.Int64("migration.id", migration.ID))
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

func createMigrationTable(ctx context.Context, db db.DB) error {
	_, err := db.Exec(ctx, "CREATE TABLE IF NOT EXISTS migrations (id BIGINT PRIMARY KEY )")
	if err != nil {
		return fmt.Errorf("migrate: Creating migrations table: %w", err)
	}
	return nil
}

// Load all the migrations files for the given FS
func Load(migrationsFs fs.ReadDirFS) (migrations []db.Migration, err error) {
	migrations = make([]db.Migration, 0)

	var upFiles = make([]string, 0, 10)
	var downFiles = make([]string, 0, 10)

	err = fs.WalkDir(migrationsFs, ".", func(path string, dir fs.DirEntry, err error) error {
		if err != nil {
			err = fmt.Errorf("migrate: error walking file [%s]: %w", path, err)
			return err
		}

		if !dir.Type().IsRegular() {
			return nil
		}

		if strings.HasSuffix(path, ".up.sql") {
			upFiles = append(upFiles, path)
		} else if strings.HasSuffix(path, ".down.sql") {
			downFiles = append(downFiles, path)
		}

		return nil
	})

	if len(upFiles) != len(downFiles) {
		err = errors.New("migrations: each .up.sql file should have a corresponding .down.sql file")
		return
	}

	sort.Strings(upFiles)
	sort.Strings(downFiles)

	migrations = make([]db.Migration, len(upFiles))
	for i, upFile := range upFiles {
		downFile := downFiles[i]
		var upFileContent []byte
		var downFileContent []byte

		upParts := strings.Split(upFile, ".")
		downParts := strings.Split(upFile, ".")
		if len(upParts) != 3 || len(upParts) != len(downParts) ||
			upParts[0] != downParts[0] {
			err = fmt.Errorf("migrations: up file \"%s\" has no corresponding down file", upFile)
			return
		}

		upFileContent, err = fs.ReadFile(migrationsFs, upFile)
		if err != nil {
			err = fmt.Errorf("migrations: error reading file \"%s\": %w", upFile, err)
			return
		}

		downFileContent, err = fs.ReadFile(migrationsFs, downFile)
		if err != nil {
			err = fmt.Errorf("migrations: error reading file \"%s\": %w", upFile, err)
			return
		}

		migrations[i] = db.Migration{
			ID:   int64(i),
			Name: strings.TrimSuffix(upFile, ".up.sql"),
			Up: func(ctx context.Context, tx db.Queryer) (err error) {
				_, err = tx.Exec(ctx, string(upFileContent))
				return
			},
			Down: func(ctx context.Context, tx db.Queryer) (err error) {
				_, err = tx.Exec(ctx, string(downFileContent))
				return
			},
		}
	}

	return
}
