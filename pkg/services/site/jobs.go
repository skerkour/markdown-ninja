package site

import "markdown.ninja/stdx/guid"

type JobSendLoginEmail struct {
	ContactID     guid.GUID `json:"contact_id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Code          string    `json:"code"`
	SessionID     guid.GUID `json:"session_id"`
	WebsiteDomain string    `json:"website_domain"`
	WebsiteID     guid.GUID `json:"website_id"`
}

func (JobSendLoginEmail) JobType() string {
	return "site.send_login_email"
}

type JobSendSubscribeEmail struct {
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Code          string    `json:"code"`
	ContactID     guid.GUID `json:"contact_id"`
	WebsiteDomain string    `json:"website_domain"`
	WebsiteID     guid.GUID `json:"website_id"`
}

func (JobSendSubscribeEmail) JobType() string {
	return "site.send_subscribe_email"
}
