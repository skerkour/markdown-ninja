package organizations

import (
	"context"

	"github.com/stripe/stripe-go/v81"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

type Service interface {
	GetAdminStatistics(ctx context.Context, _ kernel.EmptyInput) (stats AdminStatistics, err error)

	// Organizations
	GetOrganization(ctx context.Context, input GetOrganizationInput) (org Organization, err error)
	GetOrganizationsForUser(ctx context.Context, input GetOrganizationsForUserInput) (orgs []Organization, err error)
	CreateOrganization(ctx context.Context, input CreateOrganizationInput) (ret CreateOrganizationOutput, err error)
	DeleteOrganization(ctx context.Context, input DeleteOrganizationInput) (err error)
	ListOrganizations(ctx context.Context, input ListOrganizationsInput) (orgs kernel.PaginatedResult[Organization], err error)
	UpdateOrganization(ctx context.Context, input UpdateOrganizationInput) (org Organization, err error)
	ListOrganizationsForUser(ctx context.Context, db db.Queryer, userID uuid.UUID) (orgs []Organization, err error)

	// Staffs
	CheckUserIsStaff(ctx context.Context, db db.Queryer, userID uuid.UUID, organizationID guid.GUID) (staff Staff, err error)
	ListStaffInvitationsForOrganization(ctx context.Context, input ListStaffInvitationsForOrganizationInput) (ret kernel.PaginatedResult[StaffInvitation], err error)
	ListUserInvitations(ctx context.Context, _ kernel.EmptyInput) (ret kernel.PaginatedResult[UserInvitation], err error)
	InviteStaffs(ctx context.Context, input InviteStaffsInput) (invitations []StaffInvitation, err error)
	AcceptStaffInvitation(ctx context.Context, input AcceptStaffInvitationInput) (err error)
	DeleteStaffInvitation(ctx context.Context, input DeleteStaffInvitationInput) (err error)
	RemoveStaff(ctx context.Context, input RemoveStaffInput) (err error)
	FindStaffsForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (staffs []StaffWithDetails, err error)
	AddStaffs(ctx context.Context, input AddStaffsInput) (ret []StaffWithDetails, err error)

	// ApiKeys
	CheckCurrentApiKey(ctx context.Context, organizationID guid.GUID) (apiKey ApiKey, err error)
	CreateApiKey(ctx context.Context, input CreateApiKeyInput) (newApiKey ApiKeyWithToken, err error)
	VerifyApiKey(ctx context.Context, tokenStr string) (apiKey ApiKey, err error)
	DeleteApiKey(ctx context.Context, input DeleteApiKeyInput) (err error)
	UpdateApiKey(ctx context.Context, input UpdateApiKeyInput) (apiKey ApiKey, err error)

	// Billing
	HandleStripeEvent(ctx context.Context, stripeEvent stripe.Event) (err error)
	UpdateSubscription(ctx context.Context, input UpdateSubscriptionInput) (ret UpdateSubscriptionOutput, err error)
	// CancelSubscription(ctx context.Context, input UpdateSubscriptionInput) (err error)
	GetStripeCustomerPortalUrl(ctx context.Context, input GetStripeCustomerPortalUrlInput) (ret GetStripeCustomerPortalUrlOutput, err error)
	SyncStripe(ctx context.Context, input SyncStripeInput) (err error)
	GetBillingUsage(ctx context.Context, input GetBillingUsageInput) (usage BillingUsage, err error)
	CheckBillingGatedAction(ctx context.Context, db db.Queryer, organizationID guid.GUID, action BillingGatedAction) (err error)

	// Jobs
	JobSendStaffInvitations(ctx context.Context, input JobSendStaffInvitations) (err error)
	JobInvoiceMonthlyUsage(ctx context.Context, input JobInvoiceMonthlyUsage) (err error)
	JobDispatchInvoiceMonthlyUsage(ctx context.Context, input JobDispatchInvoiceMonthlyUsage) (err error)
}
