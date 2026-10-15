package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *ContactsRepository) CreateSession(ctx context.Context, db db.Queryer, session contacts.Session) (err error) {
	const query = `INSERT INTO contacts_sessions
	(id, created_at, updated_at, secret_hash, code_hash, failed_login_attempts, verified, contact_id, website_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(ctx, query, session.ID, session.CreatedAt, session.UpdatedAt, session.SecretHash,
		session.CodeHash, session.FailedLoginAttempts, session.Verified, session.ContactID, session.WebsiteID)
	if err != nil {
		err = fmt.Errorf("contacts.CreateSession: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) DeleteSession(ctx context.Context, db db.Queryer, sessionID guid.GUID) (err error) {
	const query = `DELETE FROM contacts_sessions WHERE id = $1`

	_, err = db.Exec(ctx, query, sessionID)
	if err != nil {
		err = fmt.Errorf("contacts.DeleteSession: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) FindSessionByID(ctx context.Context, db db.Queryer, sessionID guid.GUID) (session contacts.Session, err error) {
	const query = "SELECT * FROM contacts_sessions WHERE id = $1"

	err = db.Get(ctx, &session, query, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = contacts.ErrSessionNotFound
		} else {
			err = fmt.Errorf("contacts.FindSessionByID: %w", err)
		}
		return
	}

	return
}

func (repo *ContactsRepository) UpdateSession(ctx context.Context, db db.Queryer, session contacts.Session) (err error) {
	const query = `UPDATE contacts_sessions
		SET updated_at = $1, failed_login_attempts = $2, secret_hash = $3, code_hash = $4, verified = $5
		WHERE id = $6`

	_, err = db.Exec(ctx, query, session.UpdatedAt, session.FailedLoginAttempts, session.SecretHash,
		session.CodeHash, session.Verified,
		session.ID,
	)
	if err != nil {
		err = fmt.Errorf("contacts.UpdateSession: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) FindContactWithSession(ctx context.Context, db db.Queryer, sessionID guid.GUID) (contactAndSession contacts.ContactAndSession, err error) {
	const query = `SELECT contacts.*,
			contacts_sessions.id AS "session.id",
			contacts_sessions.created_at AS "session.created_at",
			contacts_sessions.updated_at AS "session.updated_at",
			contacts_sessions.secret_hash AS "session.secret_hash",
			contacts_sessions.code_hash AS "session.code_hash",
			contacts_sessions.failed_login_attempts AS "session.failed_login_attempts",
			contacts_sessions.verified AS "session.verified",
			contacts_sessions.contact_id AS "session.contact_id",
			contacts_sessions.website_id AS "session.website_id"
		FROM contacts_sessions
		INNER JOIN contacts ON contacts_sessions.contact_id = contacts.id
		WHERE contacts_sessions.id = $1
		`

	err = db.Get(ctx, &contactAndSession, query, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = contacts.ErrSessionNotFound
		} else {
			err = fmt.Errorf("contacts.FindContactWithSession: %w", err)
		}
		return
	}

	return
}

func (repo *ContactsRepository) DeleteOldUnverifiedSessions(ctx context.Context, db db.Queryer, before time.Time) (err error) {
	const query = `DELETE FROM contacts_sessions
		WHERE verified = $1 AND created_at <= $2`

	_, err = db.Exec(ctx, query, false, before)
	if err != nil {
		err = fmt.Errorf("contacts.DeleteOldUnverifiedSessions: %w", err)
		return
	}

	return
}

// DeleteOlderVerifiedSessionsForContact deletes the oldest N sessions for the given user to avoid
// having too much active sessions and avoid spam
func (repo *ContactsRepository) DeleteOlderVerifiedSessionsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID, moreThan int64) (err error) {
	const query = `DELETE FROM contacts_sessions
		WHERE id = ANY (
			SELECT id FROM contacts_sessions
				WHERE contact_id = $1 AND verified = $2
				ORDER BY id DESC OFFSET $3
		)
		`

	_, err = db.Exec(ctx, query, contactID, true, moreThan)
	if err != nil {
		err = fmt.Errorf("contacts.DeleteOlderVerifiedSessionsForContact: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) DeleteSessionsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (err error) {
	const query = `DELETE FROM contacts_sessions WHERE contact_id = $1`

	_, err = db.Exec(ctx, query, contactID)
	if err != nil {
		err = fmt.Errorf("contacts.DeleteSessionsForContact: %w", err)
		return
	}

	return
}
