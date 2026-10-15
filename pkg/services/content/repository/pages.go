package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *ContentRepository) CreatePage(ctx context.Context, db db.Queryer, page content.Page) (err error) {
	const query = `INSERT INTO pages
			(id, created_at, updated_at, date, type, title, path,
			description, language, size, body_hash, metadata_hash, status, send_as_newsletter,
			newsletter_sent_at, body_markdown, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	_, err = db.Exec(ctx, query, page.ID, page.CreatedAt, page.UpdatedAt, page.Date,
		page.Type, page.Title, page.Path,
		page.Description, page.Language, page.Size, page.BodyHash, page.MetadataHash, page.Status,
		page.SendAsNewsletter, page.NewsletterSentAt, page.BodyMarkdown,
		page.WebsiteID)
	if err != nil {
		err = fmt.Errorf("content.CreatePage: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) UpdatePage(ctx context.Context, db db.Queryer, page content.Page) (err error) {
	const query = `UPDATE pages
		SET updated_at = $1, date = $2, type = $3, title = $4, path = $5,
			description = $6, language = $7, size = $8, body_hash = $9, status = $10,
			send_as_newsletter = $11, newsletter_sent_at = $12, body_markdown = $13, metadata_hash = $14
		WHERE id = $15`

	_, err = db.Exec(ctx, query, page.UpdatedAt, page.Date, page.Type, page.Title, page.Path,
		page.Description, page.Language,
		page.Size, page.BodyHash, page.Status, page.SendAsNewsletter,
		page.NewsletterSentAt, page.BodyMarkdown, page.MetadataHash,
		page.ID)
	if err != nil {
		err = fmt.Errorf("content.UpdatePage: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) DeletePage(ctx context.Context, db db.Queryer, pageID guid.GUID) (err error) {
	const query = `DELETE FROM pages WHERE id = $1`

	_, err = db.Exec(ctx, query, pageID)
	if err != nil {
		err = fmt.Errorf("content.DeletePage: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindPageByID(ctx context.Context, db db.Queryer, pageID guid.GUID) (page content.Page, err error) {
	const query = "SELECT * FROM pages WHERE id = $1"

	err = db.Get(ctx, &page, query, pageID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrPageNotFound
		} else {
			err = fmt.Errorf("content.FindPageByID: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindPageByPath(ctx context.Context, db db.Queryer, websiteID guid.GUID, path string) (page content.Page, err error) {
	const query = "SELECT * FROM pages WHERE website_id = $1 AND path = $2"

	err = db.Get(ctx, &page, query, websiteID, path)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrPageNotFound
		} else {
			err = fmt.Errorf("content.FindPageByPath: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindPagesMetadataByTypeForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	pageType content.PageType, limit int64) (pages []content.PageMetadata, err error) {
	pages = make([]content.PageMetadata, 0)
	const query = `SELECT pages.id, pages.created_at, pages.updated_at, pages.date, pages.type, pages.title,
				pages.description, pages.path, pages.size, pages.body_hash, pages.metadata_hash,
				pages.status, pages.language, pages.send_as_newsletter, pages.newsletter_sent_at
		FROM pages
		WHERE website_id = $1 AND type = $2
		ORDER BY date DESC
		LIMIT $3
		`

	err = db.Select(ctx, &pages, query, websiteID, pageType, limit)
	if err != nil {
		err = fmt.Errorf("content.FindPagesMetadataByTypeForWebsite: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindLastPublishedPageOrPostForWebsite(ctx context.Context, db db.Queryer,
	websiteID guid.GUID) (page content.Page, err error) {
	const query = `SELECT * FROM pages
		WHERE website_id = $1
			AND (type = $2 OR type = $3)
			AND status = $4
		ORDER BY date DESC
		LIMIT 1`

	err = db.Get(ctx, &page, query, websiteID, content.PageTypePost, content.PageTypePage, content.PageStatusPublished)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrPageNotFound
		} else {
			err = fmt.Errorf("content.FindLastPublishedPageOrPostForWebsite: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindLastPublishedPostForWebsite(ctx context.Context, db db.Queryer,
	websiteID guid.GUID) (post content.Page, err error) {
	const query = `SELECT * FROM pages
		WHERE website_id = $1
			AND type = $2
			AND status = $3
		ORDER BY date DESC
		LIMIT 1`

	err = db.Get(ctx, &post, query, websiteID, content.PageTypePost, content.PageStatusPublished)
	if err != nil {
		if err == sql.ErrNoRows {
			err = content.ErrPageNotFound
		} else {
			err = fmt.Errorf("content.FindLastPublishedPostForWebsite: %w", err)
		}
		return
	}

	return
}

func (repo *ContentRepository) FindPublishedPagesMetadataForTag(ctx context.Context, db db.Queryer, pageTypes []content.PageType, tagID guid.GUID) (pages []content.PageMetadata, err error) {
	pages = make([]content.PageMetadata, 0, 10)
	const query = `SELECT pages.id, pages.created_at, pages.updated_at, pages.date, pages.type, pages.title,
				pages.description, pages.path, pages.size, pages.body_hash, pages.metadata_hash,
				pages.status, pages.language, pages.send_as_newsletter, pages.newsletter_sent_at
				FROM pages
			INNER JOIN pages_tags ON pages_tags.page_id = pages.id
			WHERE pages_tags.tag_id = $1
			AND type = ANY($2)
			AND status = $3
		ORDER BY date DESC`

	err = db.Select(ctx, &pages, query, tagID, pageTypes, content.PageStatusPublished)
	if err != nil {
		err = fmt.Errorf("content.FindPublishedPagesMetadataForTag: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindPublishedPagesMetadataForWebsite(ctx context.Context, db db.Queryer,
	websiteID guid.GUID, pageTypes []content.PageType, limit int64) (pages []content.PageMetadata, err error) {
	pages = make([]content.PageMetadata, 0, 25)
	const query = `SELECT id, created_at, updated_at, date, type, title, description, path, size,
			body_hash, metadata_hash, status, language, send_as_newsletter, newsletter_sent_at
		FROM pages
		WHERE website_id = $1
			AND type = ANY($2)
			AND status = $3
		ORDER BY type, date DESC
		LIMIT $4`

	err = db.Select(ctx, &pages, query, websiteID, pageTypes, content.PageStatusPublished, limit)
	if err != nil {
		err = fmt.Errorf("content.FindPublishedPagesMetadataForWebsite: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) FindScheduledPagesToPublish(ctx context.Context, db db.Queryer, forUpdate bool) (pages []content.Page, err error) {
	pages = make([]content.Page, 0, 5)
	now := time.Now().UTC()
	query := `SELECT * FROM pages
		WHERE status = $1 AND date <= $2`
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Select(ctx, &pages, query, content.PageStatusScheduled, now)
	if err != nil {
		err = fmt.Errorf("content.FindScheduledPagesToPublish: %w", err)
		return
	}

	return
}

func (repo *ContentRepository) GetPagesCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error) {
	const query = `SELECT COUNT(*) FROM pages WHERE website_id = $1`

	err = db.Get(ctx, &count, query, websiteID)
	if err != nil {
		err = fmt.Errorf("content.GetPagesCountForWebsite: %w", err)
		return
	}

	return
}

// func (repo *ContentRepository) PublishPages(ctx context.Context, db db.Queryer, pageIds []guid.GUID) (err error) {
// 	// sqlx.In will error if len(pageIds) == 0
// 	if len(pageIds) == 0 {
// 		return nil
// 	}

// 	const query = "UPDATE pages SET status = ? WHERE id IN (?)"

// 	query, args, err := sqlx.In(query, content.PageStatusPublished, pageIds)
// 	if err != nil {
// 		logger := slogx.FromCtx(ctx)
// 		errMessage := "content.Publishpages: preparing IN SQL query"
// 		logger.Error(errMessage, slogx.Err(err))
// 		err = errs.Internal(errMessage, err)
// 		return
// 	}

// 	// we need rebind because PostgreSQL only supports $1, $2... variable while sqlx.In only support ?
// 	query = db.Rebind(query)
// 	_, err = db.Exec(ctx, query, args...)
// 	if err != nil {
// 		logger := slogx.FromCtx(ctx)
// 		errMessage := "content.Publishpages: updating pages"
// 		logger.Error(errMessage, slogx.Err(err))
// 		err = errs.Internal(errMessage, err)
// 	}

// 	return
// }
