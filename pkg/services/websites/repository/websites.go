package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *WebsitesRepository) CreateWebsite(ctx context.Context, db db.Queryer, website websites.Website) (err error) {
	const query = `INSERT INTO websites
			(id, created_at, updated_at, modified_at, blocked_at, blocked_reason,
				name, slug, header, footer, navigation, language, primary_domain,
				description, robots_txt, currency, custom_icon, custom_icon_hash, colors,
				theme, announcement, ad, logo, powered_by,
				organization_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25)`

	_, err = db.Exec(ctx, query, website.ID, website.CreatedAt, website.UpdatedAt, website.ModifiedAt,
		website.BlockedAt, website.BlockedReason, website.Name, website.Slug, website.Header, website.Footer,
		website.Navigation, website.Language, website.PrimaryDomain,
		website.Description, website.RobotsTxt, website.Currency, website.CustomIcon, website.CustomIconHash,
		website.Colors, website.Theme, website.Announcement, website.Ad, website.Logo, website.PoweredBy,
		website.OrganizationID)
	if err != nil {
		err = fmt.Errorf("websites.CreateWebsite: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) UpdateWebsite(ctx context.Context, db db.Queryer, website websites.Website) (err error) {
	const query = `UPDATE websites
		SET updated_at = $1, modified_at = $2, blocked_at = $3, blocked_reason = $4, name = $5,
			slug = $6, header = $7, footer = $8, navigation = $9, language = $10,
			primary_domain = $11, description = $12, robots_txt = $13, currency = $14,
			custom_icon = $15, custom_icon_hash = $16, colors = $17, theme = $18,
			announcement = $19, ad = $20, logo = $21, powered_by = $22
		WHERE id = $23`

	_, err = db.Exec(ctx, query, website.UpdatedAt, website.ModifiedAt, website.BlockedAt, website.BlockedReason, website.Name,
		website.Slug, website.Header, website.Footer, website.Navigation, website.Language,
		website.PrimaryDomain, website.Description, website.RobotsTxt, website.Currency,
		website.CustomIcon, website.CustomIconHash, website.Colors, website.Theme, website.Announcement,
		website.Ad, website.Logo, website.PoweredBy,
		website.ID)
	if err != nil {
		err = fmt.Errorf("websites.UpdateWebsite: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) UpdateWebsiteModifiedAt(ctx context.Context, db db.Queryer, websiteID guid.GUID, modifiedAt time.Time) (err error) {
	const query = `UPDATE websites
		SET modified_at = $1
		WHERE id = $2`

	_, err = db.Exec(ctx, query, modifiedAt, websiteID)
	if err != nil {
		err = fmt.Errorf("websites.UpdateWebsiteModifiedAt: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) DeleteWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error) {
	const query = "DELETE FROM websites WHERE id = $1"

	_, err = db.Exec(ctx, query, websiteID)
	if err != nil {
		err = fmt.Errorf("websites.DeleteWebsite: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) GetWebsitesCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (count int64, err error) {
	const query = "SELECT COUNT(*) FROM websites WHERE organization_id = $1"

	err = db.Get(ctx, &count, query, organizationID)
	if err != nil {
		err = fmt.Errorf("websites.GetWebsitesCountForOrganization: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) FindWebsiteByID(ctx context.Context, db db.Queryer, websiteID guid.GUID, forUpdate bool) (website websites.Website, err error) {
	query := "SELECT * FROM websites WHERE id = $1"
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Get(ctx, &website, query, websiteID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = websites.ErrWebsiteNotFound
		} else {
			err = fmt.Errorf("websites.FindWebsiteByID: %w", err)
		}
		return
	}

	return
}

func (repo *WebsitesRepository) FindWebsiteBySlug(ctx context.Context, db db.Queryer, slug string) (website websites.Website, err error) {
	const query = "SELECT * FROM websites WHERE slug = $1"

	err = db.Get(ctx, &website, query, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			err = websites.ErrWebsiteNotFound
		} else {
			err = fmt.Errorf("websites.FindWebsiteBySlug: %w", err)
		}
		return
	}

	return
}

func (repo *WebsitesRepository) FindWebsiteForDomain(ctx context.Context, db db.Queryer, domain string) (website websites.Website, err error) {
	const query = `SELECT websites.* FROM websites
			INNER JOIN domains ON domains.website_id = websites.id
			WHERE domains.hostname = $1`

	err = db.Get(ctx, &website, query, domain)
	if err != nil {
		if err == sql.ErrNoRows {
			err = websites.ErrWebsiteNotFound
		} else {
			err = fmt.Errorf("websites.FindWebsiteForDomain: %w", err)
		}
		return
	}

	return
}

func (repo *WebsitesRepository) FindWebsitesForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (ret []websites.Website, err error) {
	ret = make([]websites.Website, 0, 5)
	const query = "SELECT * FROM websites WHERE organization_id = $1 ORDER BY name"

	err = db.Select(ctx, &ret, query, organizationID)
	if err != nil {
		err = fmt.Errorf("websites.FindWebsitesForOrganization: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) FindWebsites(ctx context.Context, db db.Queryer, limit int64) (ret []websites.Website, err error) {
	ret = make([]websites.Website, 0, 5)
	const query = "SELECT * FROM websites ORDER BY id LIMIT $1"

	err = db.Select(ctx, &ret, query, limit)
	if err != nil {
		err = fmt.Errorf("websites.FindAllWebsites: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) SearchWebsites(ctx context.Context, db db.Queryer, searchQuery string, limit int64) (ret []websites.Website, err error) {
	ret = make([]websites.Website, 0, 5)
	const query = "SELECT * FROM websites WHERE slug LIKE $1 || '%' LIMIT $2"

	err = db.Select(ctx, &ret, query, searchQuery, limit)
	if err != nil {
		err = fmt.Errorf("websites.FindAllWebsites: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) GetWebsitesCount(ctx context.Context, db db.Queryer) (count int64, err error) {
	const query = "SELECT COUNT(*) FROM websites"

	err = db.Get(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("websites.GetWebsitesCount: %w", err)
	}

	return count, nil
}
