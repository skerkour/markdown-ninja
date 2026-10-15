package repository

import (
	"context"
	"fmt"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *WebsitesRepository) CreateRedirect(ctx context.Context, db db.Queryer, redirect websites.Redirect) (err error) {
	const query = `INSERT INTO redirects
			(id, created_at, updated_at, pattern, domain, path_pattern, to_url, status, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(ctx, query, redirect.ID, redirect.CreatedAt, redirect.UpdatedAt,
		redirect.Pattern, redirect.Domain, redirect.PathPattern, redirect.To, redirect.Status,
		redirect.WebsiteID)
	if err != nil {
		err = fmt.Errorf("websites.CreateRedirect: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) FindRedirectsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (redirects []websites.Redirect, err error) {
	redirects = []websites.Redirect{}
	const query = `SELECT * FROM redirects
		WHERE website_id = $1
		ORDER BY created_at DESC
		`

	err = db.Select(ctx, &redirects, query, websiteID)
	if err != nil {
		err = fmt.Errorf("websites.FindRedirectsForWebsite: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) DeleteRedirect(ctx context.Context, db db.Queryer, redirectID guid.GUID) (err error) {
	const query = `DELETE FROM redirects WHERE id = $1`

	_, err = db.Exec(ctx, query, redirectID)
	if err != nil {
		err = fmt.Errorf("websites.DeleteRedirect: %w", err)
		return
	}

	return
}
