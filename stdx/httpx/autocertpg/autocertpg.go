package autocertpg

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/acme/autocert"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
)

type Cache struct {
	key []byte
	db  db.DB
}

type cert struct {
	Key           string `db:"key"`
	EncryptedData []byte `db:"encrypted_data"`
}

func NewCache(db db.DB, key []byte) *Cache {
	return &Cache{
		db:  db,
		key: key,
	}
}

func (cache *Cache) Get(ctx context.Context, key string) (data []byte, err error) {
	var cert cert
	query := "SELECT * FROM certs WHERE key = $1"
	logger := slogx.FromCtx(ctx)

	err = cache.db.Get(ctx, &cert, query, key)
	if err != nil {
		logger.Warn("autocertpg.Get: getting cert from db", slogx.Err(err))
		if err == sql.ErrNoRows {
			err = autocert.ErrCacheMiss
		}
		return
	}

	data, err = crypto.Decrypt(cache.key, cert.EncryptedData, []byte(cert.Key))
	if err != nil {
		logger.Warn("autocertpg.Get: decrypting data", slogx.Err(err))
		err = fmt.Errorf("autocertpg: decrypting data: %w", err)
		return
	}

	return
}

func (cache *Cache) Put(ctx context.Context, key string, data []byte) (err error) {
	query := `
	INSERT INTO certs (key, encrypted_data)
		VALUES ($1, $2)
		ON CONFLICT (key)
		DO UPDATE SET encrypted_data = $2
	`
	logger := slogx.FromCtx(ctx)

	encryptedData, err := crypto.Encrypt(cache.key, data, []byte(key))
	if err != nil {
		logger.Warn("autocertpg.Put: encrypting data", slogx.Err(err))
		err = fmt.Errorf("autocertpg: encrypting data: %w", err)
		return
	}

	_, err = cache.db.Exec(ctx, query, key, encryptedData)
	if err != nil {
		logger.Warn("autocertpg.Put: inserting cert in DB", slogx.Err(err))
		return
	}

	return
}

func (cache *Cache) Delete(ctx context.Context, key string) (err error) {
	logger := slogx.FromCtx(ctx)
	query := "DELETE FROM certs WHERE key = $1"

	_, err = cache.db.Exec(ctx, query, key)
	if err != nil {
		logger.Warn("autocertpg.Delete: deleting cert", slogx.Err(err))
		return
	}

	return
}
