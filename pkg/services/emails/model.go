package emails

import (
	"time"

	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/guid"
)

const (
	// ReturnPathDomain = "markdown-ninja-bounces"

	NewsletterContentMarkdownMaxSize = 75_000 // 75_000 KB
	NewsletterSubjectMaxSize         = 200
	NewsletterSubjectMinSize         = 1

	// the number of emails / s to send for a single newsletter
	NewsletterRateLimit = 5
)

type EmailType string

const (
	EmailTypeTransactional EmailType = "transactional"
	EmailTypeBroadcast     EmailType = "broadcast"
)

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

// WebsiteConfiguration is the email configuration for a website
type WebsiteConfiguration struct {
	CreatedAt time.Time `db:"created_at" json:"-"`
	UpdatedAt time.Time `db:"updated_at" json:"-"`

	FromName       string            `db:"from_name" json:"from_name"`
	FromAddress    string            `db:"from_address" json:"from_address"`
	FromDomain     string            `db:"from_domain" json:"-"`
	DomainVerified bool              `db:"domain_verified" json:"domain_verified"`
	DnsRecords     mailer.DnsRecords `db:"dns_records" json:"dns_records"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`
}

type Newsletter struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	ScheduledFor *time.Time `db:"scheduled_for" json:"scheduled_for"`
	Subject      string     `db:"subject" json:"subject"`
	Size         int64      `db:"size" json:"size"`
	// BLAKE3 hash of the bodyMarkdown
	Hash           kernel.BytesHex `db:"hash" json:"hash"`
	SentAt         *time.Time      `db:"sent_at" json:"sent_at"`
	LastTestSentAt *time.Time      `db:"last_test_sent_at" json:"last_test_sent_at"`
	BodyMarkdown   string          `db:"body_markdown" json:"body_markdown"`

	PostID    *guid.GUID `db:"post_id" json:"post_id"`
	WebsiteID guid.GUID  `db:"website_id" json:"website_id"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type GetWebsiteConfigurationInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type UpdateWebsiteConfigurationInput struct {
	WebsiteID   guid.GUID `json:"website_id"`
	FromName    string    `json:"from_name"`
	FromAddress string    `json:"from_address"`
}

type VerifyDnsConfigurationInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type GetNewslettersInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type GetNewsletterInput struct {
	ID guid.GUID `json:"id"`
}

type DeleteNewsletterInput struct {
	ID guid.GUID `json:"id"`
}

type SendNewsletterInput struct {
	ID   guid.GUID `json:"id"`
	Test bool      `json:"test"`
}

type CreateNewsletterInput struct {
	WebsiteID    guid.GUID  `json:"website_id"`
	ScheduledFor *time.Time `json:"scheduled_for"`
	Subject      string     `json:"subject"`
	BodyMarkdown string     `json:"body_markdown"`
}

type UpdateNewsletterInput struct {
	ID           guid.GUID  `json:"id"`
	ScheduledFor *time.Time `json:"scheduled_for"`
	Subject      string     `json:"subject"`
	BodyMarkdown *string    `json:"body_markdown"`
}

type NewsletterMetadata struct {
	ID        guid.GUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ScheduledFor   *time.Time      `json:"scheduled_for"`
	Subject        string          `json:"subject"`
	Size           int64           `json:"size"`
	Hash           kernel.BytesHex `json:"hash"`
	SentAt         *time.Time      `json:"sent_at"`
	LastTestSentAt *time.Time      `json:"last_test_sent_at"`
}
