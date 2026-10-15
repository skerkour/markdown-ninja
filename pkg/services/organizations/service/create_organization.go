package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	stripecustomer "github.com/stripe/stripe-go/v81/customer"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *OrganizationsService) CreateOrganization(ctx context.Context, input organizations.CreateOrganizationInput) (ret organizations.CreateOrganizationOutput, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	name := strings.TrimSpace(input.Name)
	err = service.validateOrganizationName(name)
	if err != nil {
		return
	}

	planID := input.Plan
	err = validatePlan(planID, true)
	if err != nil {
		return
	}

	if planID != kernel.PlanFree.ID && service.isSelfHosted {
		err = errs.InvalidArgument("Only the free plan can be used when self-hosting")
		return
	}

	billingEmail := strings.TrimSpace(input.BillingEmail)
	err = service.kernel.ValidateEmail(ctx, billingEmail, true)
	if err != nil {
		return
	}

	if planID == kernel.PlanFree.ID {
		orgs, err := service.repo.FindOrganizationsForUser(ctx, service.db, actorID)
		if err != nil {
			return ret, err
		}

		freeOrgs := 0
		for _, org := range orgs {
			if org.Plan == kernel.PlanFree.ID {
				freeOrgs += 1
			}
		}

		if freeOrgs != 0 {
			return ret, errs.InvalidArgument("Free organization limit reached. Please upgrade your plan to create more organizations.")
		}
	}

	billingInformation := organizations.BillingInformation{
		Name:         input.Name,
		Email:        billingEmail,
		AddressLine1: "",
		AddressLine2: "",
		PostalCode:   "",
		City:         "",
		State:        "",
		CountryCode:  httpCtx.Client.CountryCode,
		TaxID:        nil,
	}

	now := time.Now().UTC()
	organization := organizations.Organization{
		ID:                    guid.NewTimeBased(),
		CreatedAt:             now,
		UpdatedAt:             now,
		Name:                  name,
		Plan:                  kernel.PlanFree.ID,
		BillingInformation:    billingInformation,
		StripeCustomerID:      nil,
		StripeSubscriptionID:  nil,
		SubscriptionStartedAt: nil,
		ExtraSlots:            0,
		PaymentDueSince:       nil,
		UsageLastInvoicedAt:   nil,
	}

	staff := organizations.Staff{
		CreatedAt:      now,
		UpdatedAt:      now,
		Role:           organizations.StaffRoleAdministrator,
		UserID:         actorID,
		OrganizationID: organization.ID,
	}

	if planID != kernel.PlanFree.ID {
		// create stripe customer
		var stripeCustomer *stripe.Customer
		stripeCustomerParams := service.generateStripeCustomerParams(organization)
		stripeCustomer, err = stripecustomer.New(stripeCustomerParams)
		if err != nil {
			err = fmt.Errorf("organizations.CreateOrganization: creating Stripe customer: %w", err)
			return
		}

		organization.StripeCustomerID = &stripeCustomer.ID

		var stripeCheckoutSession *stripe.CheckoutSession
		var stripeCheckoutSessionLineItems []*stripe.CheckoutSessionLineItemParams
		stripeCheckoutSessionLineItems, err = service.getStripeCheckoutSessionLineItemsForPlan(planID, organization.ExtraSlots)
		if err != nil {
			err = fmt.Errorf("organizations.CreateOrganization: %w", err)
			return
		}

		// billingAnchor := timeutil.GetFirstDayOfNextMonth(time.Now().UTC()).UTC()
		checkoutParams := service.generateStripeCheckoutSessionParams(organization, planID, stripeCheckoutSessionLineItems)
		stripeCheckoutSession, err = session.New(checkoutParams)
		if err != nil {
			err = fmt.Errorf("organizations.CreateOrganization: error creating stripe checkout session: %w", err)
			return
		}

		ret.StripeCheckoutSessionUrl = &stripeCheckoutSession.URL
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreateOrganization(ctx, tx, organization)
		if txErr != nil {
			return
		}

		txErr = service.repo.CreateStaff(ctx, tx, staff)
		return txErr
	})
	if err != nil {
		return
	}

	if httpCtx.AccessToken.IsAdmin {
		organization.OrganizationAdminFields = &organizations.OrganizationAdminFields{
			StripeCustomerID:     organization.StripeCustomerID,
			StripeSubscriptionID: organization.StripeSubscriptionID,
		}
	}

	ret.Organization = organization
	return
}
