package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *EmailsRepository) CreateNewsletter(ctx context.Context, db db.Queryer, newsletter emails.Newsletter) (err error) {
	const query = `INSERT INTO newsletters
			(id, created_at, updated_at, scheduled_for, subject, size,
				hash, sent_at, last_test_sent_at, body_markdown, post_id, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err = db.Exec(ctx, query, newsletter.ID, newsletter.CreatedAt, newsletter.UpdatedAt,
		newsletter.ScheduledFor, newsletter.Subject, newsletter.Size,
		newsletter.Hash, newsletter.SentAt, newsletter.LastTestSentAt,
		newsletter.BodyMarkdown,
		newsletter.PostID, newsletter.WebsiteID)
	if err != nil {
		err = fmt.Errorf("emails.CreateNewsletter: %w", err)
		return
	}

	return
}

func (repo *EmailsRepository) FindNewsletterByID(ctx context.Context, db db.Queryer, newsletterID guid.GUID) (newsletter emails.Newsletter, err error) {
	const query = "SELECT * FROM newsletters WHERE id = $1"

	err = db.Get(ctx, &newsletter, query, newsletterID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = emails.ErrNewsletterNotFound
		} else {
			err = fmt.Errorf("emails.FindNewsletterByID: %w", err)
		}
		return
	}
	return
}

func (repo *EmailsRepository) DeleteNewsletter(ctx context.Context, db db.Queryer, newsletterID guid.GUID) (err error) {
	const query = `DELETE FROM newsletters WHERE id = $1`

	_, err = db.Exec(ctx, query, newsletterID)
	if err != nil {
		err = fmt.Errorf("emails.DeleteNewsletter: %w", err)
		return
	}

	return
}

func (repo *EmailsRepository) FindNewslettersByWebsiteID(ctx context.Context, db db.Queryer, websiteID guid.GUID) (newsletters []emails.Newsletter, err error) {
	newsletters = make([]emails.Newsletter, 0)
	const query = `SELECT * FROM newsletters
		WHERE website_id = $1
		ORDER BY created_at DESC
	`

	err = db.Select(ctx, &newsletters, query, websiteID)
	if err != nil {
		err = fmt.Errorf("emails.FindNewslettersByWebsiteID: %w", err)
		return
	}

	return
}

func (repo *EmailsRepository) UpdateNewsletter(ctx context.Context, db db.Queryer, newsletter emails.Newsletter) (err error) {
	const query = `UPDATE newsletters
		SET updated_at = $1, scheduled_for = $2, subject = $3, size = $4,
			hash = $5, sent_at = $6, last_test_sent_at = $7, body_markdown = $8
		WHERE id = $9`

	_, err = db.Exec(ctx, query, newsletter.UpdatedAt, newsletter.ScheduledFor, newsletter.Subject,
		newsletter.Size, newsletter.Hash, newsletter.SentAt,
		newsletter.LastTestSentAt, newsletter.BodyMarkdown,
		newsletter.ID)
	if err != nil {
		err = fmt.Errorf("emails.UpdateNewsletter: %w", err)
		return
	}

	return
}

func (repo *EmailsRepository) FindScheduledNewsletters(ctx context.Context, db db.Queryer, now time.Time) (newsletters []emails.Newsletter, err error) {
	newsletters = make([]emails.Newsletter, 0)
	const query = `SELECT * FROM newsletters
		WHERE sent_at IS NULL
			AND scheduled_for IS NOT NULL
			AND scheduled_for <= $1
	`

	err = db.Select(ctx, &newsletters, query, now)
	if err != nil {
		err = fmt.Errorf("emails.FindScheduledNewsletters: %w", err)
		return
	}

	return
}
