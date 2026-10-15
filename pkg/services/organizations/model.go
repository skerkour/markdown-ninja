package organizations

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

const (
	OrganizationNameMinLength = 1
	OrganizationNameMaxLength = 80

	ApiKeyPrefix        = "MarkdownNinjaV1"
	ApiKeyNameMaxLength = 42
	ApiKeyNameMinLength = 1

	StripeMeterEmails = "emails"

	// TestTaxID is not verified when used by an administrator. Use it for devlopment purpose only
	TestTaxID = "FRXXX"
)

type StaffRole int64

const (
	StaffRoleUnknown StaffRole = iota
	StaffRoleAdministrator
)

// MarshalText implements encoding.TextMarshaler.
func (role StaffRole) MarshalText() (ret []byte, err error) {
	switch role {
	case StaffRoleAdministrator:
		ret = []byte("administrator")
	default:
		ret = []byte("unknown")
		err = fmt.Errorf("Unknown StaffRole: %d", role)
	}
	return
}

func (role StaffRole) String() string {
	ret, _ := role.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (role *StaffRole) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case "administrator":
		*role = StaffRoleAdministrator
	default:
		*role = StaffRoleUnknown
		err = fmt.Errorf("Unknown StaffRole: %s", string(data))
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

type Organization struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Name string `db:"name" json:"name"`

	Plan                  kernel.PlanID      `db:"plan" json:"plan"`
	BillingInformation    BillingInformation `db:"billing_information" json:"billing_information"`
	StripeCustomerID      *string            `db:"stripe_customer_id" json:"-"`
	StripeSubscriptionID  *string            `db:"stripe_subscription_id" json:"-"`
	SubscriptionStartedAt *time.Time         `db:"subscription_started_at" json:"subscription_started_at"`
	PaymentDueSince       *time.Time         `db:"payment_due_since" json:"-"`
	// LastInvoicedUsagePeriodEndsAt
	// the time at when the last invoiced usage period ends
	UsageLastInvoicedAt *time.Time `db:"usage_last_invoiced_at" json:"-"`
	ExtraSlots          int64      `db:"extra_slots" json:"extra_slots"`

	ApiKeys                  []ApiKey           `db:"-" json:"api_keys"`
	Staffs                   []StaffWithDetails `db:"-" json:"staffs"`
	*OrganizationAdminFields `json:",omitempty"`
}

func (organization Organization) MarshalJSON() ([]byte, error) {
	// We need a special type otherwise MarshalJSON will trigger infinite recursion
	type organizationJson Organization

	return json.Marshal(struct {
		organizationJson
		StripeCustomer bool `json:"stripe_customer"`
		PaymentDue     bool `json:"payment_due"`
	}{
		organizationJson: organizationJson(organization),
		StripeCustomer:   organization.StripeCustomerID != nil,
		PaymentDue:       organization.PaymentDueSince != nil,
	})
}

type OrganizationAdminFields struct {
	StripeCustomerID     *string `json:"stripe_customer_id"`
	StripeSubscriptionID *string `json:"stripe_subscription_id"`
}

type Staff struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Role StaffRole `db:"role" json:"role"`

	UserID         uuid.UUID `db:"user_id" json:"user_id"`
	OrganizationID guid.GUID `db:"organization_id" json:"organization_id"`
}

type ApiKey struct {
	ID        guid.GUID  `db:"id" json:"id"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at"`

	Name    string `db:"name" json:"name"`
	Version int16  `db:"version" json:"-"`
	// BLAKE3
	Hash []byte `db:"hash" json:"-"`

	OrganizationID guid.GUID `db:"organization_id" json:"-"`
}

type StaffInvitation struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Role         StaffRole `db:"role" json:"role"`
	InviteeEmail string    `db:"invitee_email" json:"invitee_email"`

	OrganizationID guid.GUID `db:"organization_id" json:"organization_id"`
	InviterID      uuid.UUID `db:"inviter_id" json:"inviter_id"`
}

type StaffInvitationWithOrganizationDetails struct {
	Invitation       StaffInvitation `db:"" json:""`
	OrganizationID   guid.GUID       `db:"organization_id" json:"organization_id"`
	OrganizationName string          `db:"organization_name" json:"organization_name"`
}

type UserInvitation struct {
	Invitation StaffInvitationWithOrganizationDetails `json:""`

	InviterName  string `json:"inviter_name"`
	InviterEmail string `json:"inviter_email"`
}

type BillingInformation struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	// Address line 1 (e.g., street, PO Box, or company name).
	AddressLine1 string `json:"address_line1"`
	// Address line 2 (e.g., apartment, suite, unit, or building).
	AddressLine2 string `json:"address_line2"`
	// ZIP or postal code.
	PostalCode string `json:"postal_code"`
	// City, district, suburb, town, or village.
	City string `json:"city"`
	// State, County, Region or Province
	State string `json:"state"`
	// Two-letter country code (ISO 3166-1 alpha-2).
	CountryCode string  `json:"country_code"`
	TaxID       *string `json:"tax_id"`
}

func (billingInfo *BillingInformation) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, billingInfo)
		return nil
	case string:
		json.Unmarshal([]byte(v), billingInfo)
		return nil
	default:
		return fmt.Errorf("BillingInformation.Scan: Unsupported type: %T", v)
	}
}

func (billingInfo *BillingInformation) Value() (driver.Value, error) {
	return json.Marshal(billingInfo)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type CreateOrganizationInput struct {
	Name string        `json:"name"`
	Plan kernel.PlanID `json:"plan"`
	// BillingInformation *BillingInformation `json:"billing_information"`
	BillingEmail string `json:"billing_email"`
}

type CreateOrganizationOutput struct {
	Organization             Organization `json:"organization"`
	StripeCheckoutSessionUrl *string      `json:"stripe_checkout_session_url"`
}

type UpdateOrganizationInput struct {
	ID                 guid.GUID           `json:"id"`
	Name               *string             `json:"name"`
	BillingInformation *BillingInformation `json:"billing_information"`

	// These fields can only be updated by a Markdown Ninja admin
	Plan       *kernel.PlanID `json:"plan"`
	ExtraSlots *int64         `json:"extra_slots"`
}

type GetOrganizationsForUserInput struct {
	UserID *uuid.UUID `json:"user_id"`
}

type GetOrganizationInput struct {
	ID      *guid.GUID `json:"id"`
	ApiKeys bool       `json:"api_keys"`
	Staffs  bool       `json:"staffs"`
}

type StaffWithDetails struct {
	Staff `db:"" json:""`
	Name  string `db:"name" json:"name"`
	Email string `db:"email" json:"email"`
}

type DeleteOrganizationInput struct {
	ID guid.GUID `json:"id"`
}

type ListOrganizationsInput struct {
}

type ApiKeyWithToken struct {
	ApiKey
	Token string `json:"token"`
}

// type ApiKeyAndOrganization struct {
// 	ApiKey  ApiKey
// 	Organization Organization
// }

type CreateApiKeyInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
	Name           string    `json:"name"`
}

type DeleteApiKeyInput struct {
	ID guid.GUID `json:"id"`
}

type UpdateApiKeyInput struct {
	ID   guid.GUID `json:"id"`
	Name string    `json:"name"`
}

type ListStaffInvitationsForOrganizationInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
}

type InviteStaffsInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
	Emails         []string  `json:"emails"`
}

type AcceptStaffInvitationInput struct {
	ID guid.GUID `json:"id"`
}

type DeleteStaffInvitationInput struct {
	ID guid.GUID `json:"id"`
}

type RemoveStaffInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
}

type AddStaffsInput struct {
	OrganizationID guid.GUID   `json:"organization_id"`
	UserIDs        []uuid.UUID `json:"user_ids"`
}

type UpdateSubscriptionInput struct {
	OrganizationID guid.GUID     `json:"organization_id"`
	Plan           kernel.PlanID `json:"plan"`
	ExtraSlots     int64         `json:"extra_slots"`
}

type UpdateSubscriptionOutput struct {
	StripeCheckoutSessionUrl *string `json:"stripe_checkout_session_url"`
}

type GetStripeCustomerPortalUrlInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
}

type GetStripeCustomerPortalUrlOutput struct {
	StripeCustomerPortalUrl string `json:"stripe_customer_portal_url"`
}

type SyncStripeInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
}

type GetBillingUsageInput struct {
	OrganizationID guid.GUID `json:"organization_id"`
}

type BillingUsage struct {
	UsedWebsites    int64 `json:"used_websites"`
	AllowedWebsites int64 `json:"allowed_websites"`
	UsedStorage     int64 `json:"used_storage"`
	// Allowed storage, in bytes
	AllowedStorage int64 `json:"allowed_storage"`
	UsedStaffs     int64 `json:"used_staffs"`
	AllowedStaffs  int64 `json:"allowed_staffs"`
	AllowedEmails  int64 `json:"allowed_emails"`
	UsedEmails     int64 `json:"used_emails"`
}

type AdminStatistics struct {
	Organizations       int64 `json:"organizations"`
	PayingOrganizations int64 `json:"paying_organizations"`
}
