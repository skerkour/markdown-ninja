package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"markdown.ninja/pkg/dbx"
	"markdown.ninja/stdx/db"
)

var (
	ErrSettingNotFound = errors.New("Setting not found")
)

type Setting interface {
	Key() string
}

// a settings, as storred in database
type setting[T Setting] struct {
	Key       string      `db:"key"`
	CreatedAt time.Time   `db:"created_at"`
	UpdatedAt time.Time   `db:"updated_at"`
	Value     dbx.Json[T] `db:"value"`
}

func Get[T Setting](ctx context.Context, db db.Queryer) (ret T, err error) {
	const query = "SELECT * FROM settings WHERE key = $1"

	var settingValue setting[T]

	err = db.Get(ctx, &settingValue, query, ret.Key())
	if err != nil {
		if err == sql.ErrNoRows {
			return ret, ErrSettingNotFound
		} else {
			return ret, fmt.Errorf("error finding setting [%s]: %w", ret.Key(), err)
		}
	}

	return settingValue.Value.Val, nil
}

func Set[T Setting](ctx context.Context, db db.Queryer, setting T) (err error) {
	const query = `INSERT INTO settings (key, created_at, updated_at, value) VALUES ($1, $2, $2, $3)
		ON CONFLICT (key) DO UPDATE SET updated_at = $2, value = $3`

	now := time.Now().UTC()
	_, err = db.Exec(ctx, query, setting.Key(), now, setting)
	if err != nil {
		return fmt.Errorf("error saving setting [%s]: %w", setting.Key(), err)
	}

	return nil
}
