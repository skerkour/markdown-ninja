package events

import "markdown.ninja/stdx/guid"

type JobDeleteWebsiteEvents struct {
	WebsiteID guid.GUID `json:"website_id"`
}

func (JobDeleteWebsiteEvents) JobType() string {
	return "events.delete_website_events"
}

type JobDeleteOrganizationEvents struct {
	OrganizationID guid.GUID `json:"organization_id"`
}

func (JobDeleteOrganizationEvents) JobType() string {
	return "events.delete_organization_events"
}

type JobRotateAnonymousIDSalt struct {
}

func (JobRotateAnonymousIDSalt) JobType() string {
	return "events.rotate_anonymous_id_salt"
}
