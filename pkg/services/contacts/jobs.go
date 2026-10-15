package contacts

import "markdown.ninja/stdx/guid"

type JobDeleteOldUnverifiedContacts struct {
}

func (JobDeleteOldUnverifiedContacts) JobType() string {
	return "contacts.delete_old_unverified_contacts"
}

type JobDeleteOldUnverifiedSessions struct {
}

func (JobDeleteOldUnverifiedSessions) JobType() string {
	return "contacts.delete_old_unverified_sessions"
}

type JobUpdateStripeContact struct {
	ContactID guid.GUID `json:"contact_id"`
}

func (JobUpdateStripeContact) JobType() string {
	return "contacts.update_stripe_contact"
}

type JobSendVerifyEmailEmail struct {
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	WebsiteID       guid.GUID `json:"website_id"`
	ContactID       guid.GUID `json:"contact_id"`
	VerifyEmailLink string    `json:"verify_email_link"`
}

func (JobSendVerifyEmailEmail) JobType() string {
	return "contacts.send_verify_email_email"
}

type JobSyncUnsubscribedContacts struct {
}

func (JobSyncUnsubscribedContacts) JobType() string {
	return "contacts.sync_unsubscribed_contacts"
}
