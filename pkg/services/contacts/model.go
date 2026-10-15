package contacts

import (
	"time"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/guid"
)

// const (
// 	LabelNameMinSize        = 1
// 	LabelNameMaxSize        = 42
// 	LabelDescriptionMaxSize = 420
// 	LabelNameAlphabet       = "abcdefghijklmnopqrstuvwxyz0123456789-"
// )

const (
	KeyInfoUnsubscribe = "unsubscribe"
	KeyInfoUpdateEmail = "update_email"
)

const (
	AuthCookie        = "markdown_ninja_website_auth"
	AuthCookieTimeout = 30 * 24 * time.Hour // 30 days

	ContactNameMaxLength = 80
)

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

type Contact struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// TODO: first and last name?
	Name                         string     `db:"name" json:"name"`
	Email                        string     `db:"email" json:"email"`
	SubscribedToNewsletterAt     *time.Time `db:"subscribed_to_newsletter_at" json:"subscribed_to_newsletter_at"`
	SubscribedToProductUpdatesAt *time.Time `db:"subscribed_to_product_updates_at" json:"-"`
	Verified                     bool       `db:"verified" json:"-"`
	// 2-letter code of the country
	Country              string     `db:"country" json:"country"`
	FailedSignupAttempts int64      `db:"failed_signup_attempts" json:"-"`
	SignupCodeHash       string     `db:"signup_code_hash" json:"-"`
	BlockedAt            *time.Time `db:"blocked_at" json:"blocked_at"`

	StripeCustomerID *string `db:"stripe_customer_id" json:"stripe_customer_id"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`

	Products []store.Product `db:"-" json:"products"`
	Orders   []store.Order   `db:"-" json:"orders"`
}

// UpdatedAt is the last time a session has been refreshed
type Session struct {
	ID        guid.GUID `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	// BLAKE3
	SecretHash []byte `db:"secret_hash"`
	// The Argon2id hash of the temporary signup / login code
	CodeHash            string `db:"code_hash"`
	FailedLoginAttempts int64  `db:"failed_login_attempts"`
	Verified            bool   `db:"verified"`

	ContactID guid.GUID `db:"contact_id"`
	WebsiteID guid.GUID `db:"website_id"`
}

// type Label struct {
// 	ID        guid.GUID `db:"id" json:"id"`
// 	CreatedAt time.Time `db:"created_at" json:"created_at"`
// 	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

// 	Name        string `db:"name" json:"name"`
// 	Description string `db:"description" json:"description"`

// 	WebsiteID guid.GUID `db:"website_id" json:"-"`
// }

// type ContactLabelRelation struct {
// 	ContactID guid.GUID `db:"contact_id"`
// 	LabelID   guid.GUID `db:"label_id"`
// }

type PaymentMethod struct {
	Brand    string `db:"brand"`
	ExpMonth string `db:"exp_month"`
	ExpYear  string `db:"exp_year"`
	Last4    string `db:"last4"`

	ContactID guid.GUID `db:"contact_id"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type ContactAndSession struct {
	Contact Contact `db:""`
	Session Session `db:"session"`
}

type CreateSessionInput struct {
	Verified      bool
	LoginCodeHash string
	ContactID     guid.GUID
	WebsiteID     guid.GUID
}

type VerifySession struct {
	Verified bool
}

type CreateLabelInput struct {
	WebsiteID   guid.GUID `json:"website_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type UpdateLabelInput struct {
	ID          guid.GUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type CreateContactInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
}

type CreateContactInternalInput struct {
	Email    string
	Name     string
	Verified bool
	// 2-letter code of the country
	Country                string
	SubscribedToNewsletter bool
	WebsiteID              guid.GUID
	SignupCodeHash         string
}

type UpdateContactInput struct {
	ID                     guid.GUID `json:"id"`
	Email                  *string   `json:"email"`
	Name                   *string   `json:"name"`
	SubscribedToNewsletter *bool     `json:"subscribed_to_newsletter"`

	BillingAddress *kernel.Address `json:"billing_address"`

	// TODO
	Verified *bool `json:"-"`
	// 2-letter code of the country
	Country              *string `json:"country"`
	SignupCodeHash       *string `json:"-"`
	FailedSignupAttempts *int64  `json:"-"`
	StripeCustomerID     *string `json:"-"`
}

type DeleteContactInput struct {
	ID guid.GUID `json:"id"`
}

type ImportContactsInput struct {
	WebsiteID   guid.GUID `json:"website_id"`
	ContactsCsv string    `json:"contacts"`
}

type GetContactInput struct {
	ID guid.GUID `json:"id"`
}

type ListContactsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
	Query     string    `json:"query"`
}

type DeleteLabelInput struct {
	ID guid.GUID `json:"id"`
}

type GetLabelsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type ExportContactsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type ExportContactsOutput struct {
	Contacts string `json:"contacts"`
}

type ExportContactsForProductInput struct {
	ProductID guid.GUID `json:"product_id"`
}

type ExportContactsForProductOutput struct {
	Contacts string `json:"contacts"`
}

type VerifyEmailInput struct {
	Token string `json:"token"`
}

type BlockContactInput struct {
	ID guid.GUID `json:"id"`
}

type UnblockContactInput struct {
	ID guid.GUID `json:"id"`
}
