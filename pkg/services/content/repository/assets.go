package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *ContentRepository) CreateAsset(ctx context.Context, db db.Queryer, asset content.Asset) (err error) {
	const query = `INSERT INTO assets
			(id, created_at, updated_at, type, name, folder, media_type, size, hash,
				website_id, product_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err = db.Exec(ctx, query, asset.ID, asset.CreatedAt, asset.UpdatedAt, asset.Type, asset.Name,
		asset.Folder, asset.MediaType, asset.Size, asset.Hash,
		asset.WebsiteID, asset.ProductID)
	if err != nil {
		err = fmt.Errorf("content.CreateAsset: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) UpdateAsset(ctx context.Context, db db.Queryer, asset content.Asset) (err error) {
	const query = `UPDATE assets
		SET updated_at = $1, type = $2, name = $3, folder = $4, media_type = $5, size = $6,
		hash = $7
		WHERE id = $8`

	_, err = db.Exec(ctx, query, asset.UpdatedAt, asset.Type, asset.Name, asset.Folder, asset.MediaType,
		asset.Size, asset.Hash,
		asset.ID)
	if err != nil {
		err = fmt.Errorf("content.UpdateAsset: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindAssetByID(ctx context.Context, db db.Queryer, assetID guid.GUID) (asset content.Asset, err error) {
	const query = "SELECT * FROM assets WHERE id = $1"

	err = db.Get(ctx, &asset, query, assetID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrAssetNotFound
		} else {
			err = fmt.Errorf("content.FindAssetByID: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindAssetByPath(ctx context.Context, db db.Queryer, websiteID guid.GUID, folder, name string) (asset content.Asset, err error) {
	const query = "SELECT * FROM assets WHERE website_id = $1 AND folder = $2 AND name = $3"

	err = db.Get(ctx, &asset, query, websiteID, folder, name)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrAssetNotFound
		} else {
			err = fmt.Errorf("content.FindAssetByPath: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) DeleteAsset(ctx context.Context, db db.Queryer, assetID guid.GUID) (err error) {
	const query = `DELETE FROM assets WHERE id = $1`

	_, err = db.Exec(ctx, query, assetID)
	if err != nil {
		err = fmt.Errorf("content.DeleteAsset: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) DeleteAssets(ctx context.Context, db db.Queryer, assetIDs []guid.GUID) (err error) {
	if len(assetIDs) == 0 {
		return
	}
	const query = "DELETE FROM assets WHERE id = ANY ($1)"

	_, err = db.Exec(ctx, query, assetIDs)
	if err != nil {
		err = fmt.Errorf("content.DeleteAssets: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) DeleteWebsiteAssets(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error) {
	const query = `DELETE FROM assets WHERE website_id = $1`

	_, err = db.Exec(ctx, query, websiteID)
	if err != nil {
		err = fmt.Errorf("content.DeleteWebsiteAssets: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) DeleteProductAssets(ctx context.Context, db db.Queryer, productID guid.GUID) (err error) {
	const query = `DELETE FROM assets WHERE product_id = $1`

	_, err = db.Exec(ctx, query, productID)
	if err != nil {
		err = fmt.Errorf("content.DeleteProductAssets: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindProductAssets(ctx context.Context, db db.Queryer, productID guid.GUID) (ret []content.Asset, err error) {
	ret = make([]content.Asset, 0)
	const query = `SELECT * FROM assets
		WHERE product_id = $1
		ORDER BY created_at
	`

	err = db.Select(ctx, &ret, query, productID)
	if err != nil {
		err = fmt.Errorf("content.FindProductAssets: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindAssetsDirectChildren(ctx context.Context, db db.Queryer, websiteID guid.GUID, folder string) (ret []content.Asset, err error) {
	ret = make([]content.Asset, 0)
	// we put folders first for better user experience
	const query = `SELECT * FROM assets
		WHERE website_id = $1 AND folder = $2
		ORDER BY type = $3 DESC, name
	`

	err = db.Select(ctx, &ret, query, websiteID, folder, content.AssetTypeFolder)
	if err != nil {
		err = fmt.Errorf("content.FindAssetsDirectChildren: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindAssetsAllChildren(ctx context.Context, db db.Queryer, websiteID guid.GUID, folder string) (ret []content.Asset, err error) {
	ret = make([]content.Asset, 0)
	const query = `SELECT * FROM assets
		WHERE website_id = $1
			AND (folder = $2 OR folder LIKE $2 || '/%')
	`

	err = db.Select(ctx, &ret, query, websiteID, folder)
	if err != nil {
		err = fmt.Errorf("content.FindAssetsAllChildren: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindWebsiteAssetsByType(ctx context.Context, db db.Queryer, websiteID guid.GUID, assetType content.AssetType) (ret []content.Asset, err error) {
	ret = make([]content.Asset, 0)
	const query = `SELECT * FROM assets
		WHERE website_id = $1 AND type = $2
		ORDER BY created_at
	`

	err = db.Select(ctx, &ret, query, websiteID, assetType)
	if err != nil {
		err = fmt.Errorf("content.FindWebsiteAssetsByType: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) GetUsedAssetsStorageForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (storage int64, err error) {
	const query = `SELECT COALESCE(SUM(size), 0) AS storage FROM assets WHERE website_id = ANY(
		SELECT id FROM websites WHERE organization_id = $1
	)`

	err = db.Get(ctx, &storage, query, organizationID)
	if err != nil {
		err = fmt.Errorf("content.GetUsedAssetsStorageForOrganization: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) GetAssetsCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error) {
	const query = `SELECT COUNT(*) FROM assets WHERE website_id = $1`

	err = db.Get(ctx, &count, query, websiteID)
	if err != nil {
		err = fmt.Errorf("content.GetAssetsCountForWebsite: %w", err)
		return
	}

	return
}
