package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *OrganizationsRepository) CreateApiKey(ctx context.Context, dbConn db.Queryer, apiKey organizations.ApiKey) (err error) {
	const query = `INSERT INTO api_keys
			(id, created_at, updated_at, expires_at, name, version, hash, organization_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = dbConn.Exec(ctx, query, apiKey.ID, apiKey.CreatedAt, apiKey.UpdatedAt, apiKey.ExpiresAt, apiKey.Name,
		apiKey.Version, apiKey.Hash, apiKey.OrganizationID)
	if err != nil {
		if db.IsErrAlreadyExists(err) {
			err = organizations.ErrApiKeyAlreadyExists(apiKey.Name)
			return
		}

		err = fmt.Errorf("organizations.CreateApiKey: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) UpdateApiKey(ctx context.Context, dbConn db.Queryer, apiKey organizations.ApiKey) (err error) {
	const query = `UPDATE api_keys SET updated_at = $1, expires_at = $2, name = $3
		WHERE id = $4`

	_, err = dbConn.Exec(ctx, query, apiKey.UpdatedAt, apiKey.ExpiresAt, apiKey.Name, apiKey.ID)
	if err != nil {
		if db.IsErrAlreadyExists(err) {
			err = organizations.ErrApiKeyAlreadyExists(apiKey.Name)
			return
		}

		err = fmt.Errorf("organizations.UpdateApiKey: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) DeleteApiKey(ctx context.Context, db db.Queryer, apiKeyID guid.GUID) (err error) {
	const query = `DELETE FROM api_keys WHERE id = $1`

	_, err = db.Exec(ctx, query, apiKeyID)
	if err != nil {
		err = fmt.Errorf("organizations.DeleteApiKey: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindApiKeyByID(ctx context.Context, db db.Queryer, apiKeyID guid.GUID) (apiKey organizations.ApiKey, err error) {
	const query = "SELECT * FROM api_keys WHERE id = $1"

	err = db.Get(ctx, &apiKey, query, apiKeyID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = organizations.ErrApiKeyNotFound
		} else {
			err = fmt.Errorf("organizations.FindApiKeyByID: %w", err)
		}
		return
	}

	return
}

func (repo *OrganizationsRepository) FindApiKeysForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (apiKeys []organizations.ApiKey, err error) {
	apiKeys = []organizations.ApiKey{}
	const query = `SELECT * FROM api_keys
		WHERE organization_id = $1
		ORDER BY name`

	err = db.Select(ctx, &apiKeys, query, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.FindApiKeysForOrganization: %w", err)
		return
	}

	return
}
