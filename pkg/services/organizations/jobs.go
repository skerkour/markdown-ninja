package organizations

import "markdown.ninja/stdx/guid"

type JobSendStaffInvitations struct {
	InvitationIDs []guid.GUID `json:"invitation_ids"`
}

func (JobSendStaffInvitations) JobType() string {
	return "organizations.send_staff_invitations"
}

type JobSendUsageData struct {
	OrganizationID guid.GUID `json:"organization_id"`
}

func (JobSendUsageData) JobType() string {
	return "organizations.send_usage_data"
}

type JobDispatchSendUsageData struct {
}

func (JobDispatchSendUsageData) JobType() string {
	return "organizations.dispatch_send_usage_data"
}

type JobInvoiceMonthlyUsage struct {
	OrganizationID guid.GUID `json:"organization_id"`
	IdempotencyKey string    `json:"idempotency_key"`
}

func (JobInvoiceMonthlyUsage) JobType() string {
	return "organizations.invoice_monthly_usage"
}

type JobDispatchInvoiceMonthlyUsage struct {
}

func (JobDispatchInvoiceMonthlyUsage) JobType() string {
	return "organizations.dispatch_invoice_monthly_usage"
}
