package emails

import (
	"time"

	"markdown.ninja/stdx/guid"
)

type JobDeleteWebsiteConfigurationData struct {
	Domain string `json:"domain"`
}

func (JobDeleteWebsiteConfigurationData) JobType() string {
	return "emails.delete_website_configuration_data"
}

type JobSendNewsletter struct {
	NewsletterID guid.GUID `json:"newsletter_id"`
	Test         bool      `json:"test"`
	TestEmails   []string  `json:"test_emails"`
	SentAt       time.Time `json:"sent_at"`
}

func (JobSendNewsletter) JobType() string {
	return "emails.send_newsletter"
}

type JobSendPostAsNewsletter struct {
	PostID        guid.GUID `json:"pos_id"`
	WebsiteDomain string    `json:"website_domain"`
}

func (JobSendPostAsNewsletter) JobType() string {
	return "emails.send_post_as_newsletter"
}

type JobSendEmail struct {
	Type EmailType `json:"type"`
	// FromAddress and FromName are ignored when Type == transactional
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	ToAddress   string `json:"to_address"`
	ToName      string `json:"to_name"`

	Subject  string              `json:"subject"`
	BodyHtml string              `json:"body_html"`
	BodyText *string             `json:"body_text"`
	Headers  map[string][]string `json:"headers"`

	WebsiteID      *guid.GUID `json:"website_id"`
	ContactID      *guid.GUID `json:"contact_id"`
	NewsletterID   *guid.GUID `json:"newsletter_id"`
	OrganizationID *guid.GUID `json:"organization_id"`
}

func (JobSendEmail) JobType() string {
	return "emails.send_email"
}
