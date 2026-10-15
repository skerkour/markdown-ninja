package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *ContentRepository) CreateSnippet(ctx context.Context, db db.Queryer, snippet content.Snippet) (err error) {
	const query = `INSERT INTO snippets
			(id, created_at, updated_at, name, content, hash, render_in_emails, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = db.Exec(ctx, query, snippet.ID, snippet.CreatedAt, snippet.UpdatedAt, snippet.Name,
		snippet.Content, snippet.Hash, snippet.RenderInEmails,
		snippet.WebsiteID)
	if err != nil {
		err = fmt.Errorf("content.CreateSnippet: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) UpdateSnippet(ctx context.Context, db db.Queryer, snippet content.Snippet) (err error) {
	const query = `UPDATE snippets
		SET updated_at = $1, name = $2, content = $3, hash = $4, render_in_emails = $5
		WHERE id = $6`

	_, err = db.Exec(ctx, query, snippet.UpdatedAt, snippet.Name, snippet.Content, snippet.Hash,
		snippet.RenderInEmails,
		snippet.ID)
	if err != nil {
		err = fmt.Errorf("content.UpdateSnippet: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) DeleteSnippet(ctx context.Context, db db.Queryer, snippetID guid.GUID) (err error) {
	const query = `DELETE FROM snippets WHERE id = $1`

	_, err = db.Exec(ctx, query, snippetID)
	if err != nil {
		err = fmt.Errorf("content.DeleteSnippet: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindSnippetByID(ctx context.Context, db db.Queryer, snippetID guid.GUID) (snippet content.Snippet, err error) {
	const query = `SELECT * FROM snippets
		WHERE id = $1`

	err = db.Get(ctx, &snippet, query, snippetID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrSnippetNotFound
		} else {
			err = fmt.Errorf("content.FindSnippetByID: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindSnippetByName(ctx context.Context, db db.Queryer, websiteID guid.GUID, name string) (snippet content.Snippet, err error) {
	const query = `SELECT * FROM snippets
		WHERE website_id = $1 AND name = $2`

	err = db.Get(ctx, &snippet, query, websiteID, name)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrSnippetNotFound
		} else {
			err = fmt.Errorf("content.FindSnippetByName: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindSnippetsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (snippets []content.Snippet, err error) {
	snippets = make([]content.Snippet, 0)
	const query = `SELECT * FROM snippets
		WHERE website_id = $1
		ORDER BY name
`

	err = db.Select(ctx, &snippets, query, websiteID)
	if err != nil {
		err = fmt.Errorf("content.FindSnippetsForWebsite: %w", err)
		return
	}

	return
}
