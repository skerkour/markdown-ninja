package jwt

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"markdown.ninja/stdx/db"
)

type keyStorePostgres struct {
	db db.DB
}

func newPostgresKeyStore(db db.DB) *keyStorePostgres {
	return &keyStorePostgres{
		db,
	}
}

func (store *keyStorePostgres) GetLatestKey(ctx context.Context) (jwtKey, error) {
	const query = "SELECT * FROM jwt_keys ORDER BY created_at DESC LIMIT 1"

	var ret jwtKey
	err := store.db.Get(ctx, &ret, query)
	if err != nil {
		if err == sql.ErrNoRows {
			return ret, ErrKeyNotFound
		} else {
			return ret, fmt.Errorf("jwt.GetLatestKey: %w", err)
		}
	}

	return ret, nil
}

func (store *keyStorePostgres) GetKey(ctx context.Context, keyID string) (jwtKey, error) {
	const query = "SELECT * FROM jwt_keys WHERE id = $1"

	var ret jwtKey
	err := store.db.Get(ctx, &ret, query, keyID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ret, ErrKeyNotFound
		} else {
			return ret, fmt.Errorf("jwt.GetKey: %w", err)
		}
	}

	return ret, nil
}

func (store *keyStorePostgres) SetKey(ctx context.Context, key jwtKey) error {
	const query = `INSERT INTO jwt_keys (id, created_at, updated_at, algorithm, encrypted_secret_key)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET updated_at = $3, encrypted_secret_key = $5`

	_, err := store.db.Exec(ctx, query, key.ID, key.CreatedAt, key.UpdatedAt, key.Algorithm, key.EncryptedSecretKey)
	if err != nil {
		err = fmt.Errorf("jwt.SetKey[%s]: %w", key.ID, err)
		return err
	}

	return nil
}

func (store *keyStorePostgres) DeleteKey(ctx context.Context, keyId string) error {
	const query = `DELETE FROM jwt_keys WHERE id = $1`

	_, err := store.db.Exec(ctx, query, keyId)
	if err != nil {
		return fmt.Errorf("jwt.DeleteKey: %w", err)
	}

	return nil
}

func (store *keyStorePostgres) GetAllKeys(ctx context.Context) ([]jwtKey, error) {
	const query = `SELECT * FROM jwt_keys ORDER BY created_at DESC`

	ret := make([]jwtKey, 0)
	err := store.db.Select(ctx, &ret, query)
	if err != nil {
		return ret, fmt.Errorf("jwt.GetAllKeys: %w", err)
	}

	return ret, nil
}

func (store *keyStorePostgres) DeleteOlderKeys(ctx context.Context, olderThan time.Time) error {
	const query = `DELETE FROM jwt_keys WHERE created_at < $1`

	_, err := store.db.Exec(ctx, query, olderThan)
	if err != nil {
		return fmt.Errorf("jwt.DeleteOlderKeys: %w", err)
	}

	return nil
}
