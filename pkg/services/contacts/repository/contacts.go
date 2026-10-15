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

func (repo *ContactsRepository) FindContactByEmail(ctx context.Context, db db.Queryer, websiteID guid.GUID, email string) (contact contacts.Contact, err error) {
	const query = "SELECT * FROM contacts WHERE website_id = $1 AND email = $2"

	err = db.Get(ctx, &contact, query, websiteID, email)
	if err != nil {
		if err == sql.ErrNoRows {
			err = contacts.ErrContactNotFound
		} else {
			err = fmt.Errorf("contacts.FindContactByEmail: %w", err)
		}
		return
	}

	return
}

func (repo *ContactsRepository) CreateContact(ctx context.Context, db db.Queryer, contact contacts.Contact) (err error) {
	const query = `INSERT INTO contacts
				(id, created_at, updated_at, email, subscribed_to_newsletter_at, subscribed_to_product_updates_at,
					verified, name, country, failed_signup_attempts, signup_code_hash,
					stripe_customer_id, blocked_at,
					website_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err = db.Exec(ctx, query, contact.ID, contact.CreatedAt, contact.UpdatedAt, contact.Email,
		contact.SubscribedToNewsletterAt, contact.SubscribedToProductUpdatesAt, contact.Verified,
		contact.Name, contact.Country, contact.FailedSignupAttempts, contact.SignupCodeHash,
		contact.StripeCustomerID,
		contact.BlockedAt,
		contact.WebsiteID)
	if err != nil {
		err = fmt.Errorf("contacts.CreateContact: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) FindContactByID(ctx context.Context, db db.Queryer, contactID guid.GUID) (contact contacts.Contact, err error) {
	const query = "SELECT * FROM contacts WHERE id = $1"

	err = db.Get(ctx, &contact, query, contactID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = contacts.ErrContactNotFound
		} else {
			err = fmt.Errorf("contacts.FindContactByID: %w", err)
		}
		return
	}

	return
}

func (repo *ContactsRepository) UpdateContact(ctx context.Context, db db.Queryer, contact contacts.Contact) (err error) {
	const query = `UPDATE contacts
		SET updated_at = $1, email = $2, subscribed_to_newsletter_at = $3, subscribed_to_product_updates_at = $4,
			verified = $5, name = $6, country = $7, failed_signup_attempts = $8, signup_code_hash = $9,
			stripe_customer_id = $10, blocked_at = $11
		WHERE id = $12`

	_, err = db.Exec(ctx, query, contact.UpdatedAt, contact.Email, contact.SubscribedToNewsletterAt,
		contact.SubscribedToProductUpdatesAt, contact.Verified, contact.Name, contact.Country,
		contact.FailedSignupAttempts, contact.SignupCodeHash,
		contact.StripeCustomerID, contact.BlockedAt,
		contact.ID)
	if err != nil {
		err = fmt.Errorf("contacts.UpdateContact: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) DeleteContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (err error) {
	const query = `DELETE FROM contacts WHERE id = $1`

	_, err = db.Exec(ctx, query, contactID)
	if err != nil {
		err = fmt.Errorf("contacts.DeleteContact: %w", err)
		return
	}

	return
}

// Find contacts for ANY website
func (repo *ContactsRepository) FindContactsByEmail(ctx context.Context, db db.Queryer, email string, forUpdate bool) (ret []contacts.Contact, err error) {
	ret = make([]contacts.Contact, 0)

	query := "SELECT * FROM contacts WHERE email = $1"
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Select(ctx, &ret, query, email)
	if err != nil {
		err = fmt.Errorf("contacts.FindContactsByEmail: %w", err)
		return ret, err
	}

	return ret, nil
}

func (repo *ContactsRepository) FindVerifiedContactsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID, searchQuery string, limit int64) (ret []contacts.Contact, err error) {
	ret = make([]contacts.Contact, 0)
	const query = `SELECT * FROM contacts
		WHERE website_id = $1 AND verified = $2 AND email LIKE $3 || '%'
		ORDER BY id DESC
		LIMIT $4
	`

	err = db.Select(ctx, &ret, query, websiteID, true, searchQuery, limit)
	if err != nil {
		err = fmt.Errorf("contacts.FindVerifiedContactsForWebsite: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) FindVerifiedAndSubscribedToNewsletterContacts(ctx context.Context, db db.Queryer, websiteID guid.GUID) (ret []contacts.Contact, err error) {
	ret = make([]contacts.Contact, 0)
	const query = `SELECT * FROM contacts
		WHERE website_id = $1
			AND verified = $2
			AND subscribed_to_newsletter_at IS NOT NULL
`

	err = db.Select(ctx, &ret, query, websiteID, true)
	if err != nil {
		err = fmt.Errorf("contacts.FindVerifiedAndSubscribedToNewsletterContacts: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) DeleteOldUnverifiedContacts(ctx context.Context, db db.Queryer, before time.Time) (err error) {
	const query = `DELETE FROM contacts
		WHERE verified = $1 AND created_at <= $2`

	_, err = db.Exec(ctx, query, false, before)
	if err != nil {
		err = fmt.Errorf("contacts.DeleteOldUnverifiedContacts: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) FindContactsByEmails(ctx context.Context, db db.Queryer, websiteID guid.GUID, emails []string) (ret []contacts.Contact, err error) {
	ret = make([]contacts.Contact, 0)
	if len(emails) == 0 {
		return
	}

	const query = `SELECT * FROM contacts
		WHERE website_id = $1 AND email = ANY ($2)
		ORDER BY id DESC
	`

	err = db.Select(ctx, &ret, query, websiteID, emails)
	if err != nil {
		err = fmt.Errorf("contacts.FindContactsByEmails: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) FindContactsWithAccessToProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (ret []contacts.Contact, err error) {
	ret = make([]contacts.Contact, 0)
	const query = `SELECT * FROM contacts WHERE id = ANY (
		SELECT contact_id FROM contact_product_access WHERE product_id = $1
	)`

	err = db.Select(ctx, &ret, query, productID)
	if err != nil {
		err = fmt.Errorf("contacts.FindContactsWithAccessToProduct: %w", err)
		return
	}

	return
}

func (repo *ContactsRepository) GetVerifiedAndSubscribedToNewsletterContactsCount(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error) {
	const query = `SELECT COUNT(*) FROM contacts
	WHERE website_id = $1
		AND verified = $2
		AND subscribed_to_newsletter_at IS NOT NULL`

	err = db.Get(ctx, &count, query, websiteID, true)
	if err != nil {
		err = fmt.Errorf("contacts.GetVerifiedAndSubscribedToNewsletterContactsCount: %w", err)
		return
	}

	return
}
