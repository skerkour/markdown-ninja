package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/stripe/stripe-go/v81"
	stripecustomer "github.com/stripe/stripe-go/v81/customer"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
)

func (service *OrganizationsService) SyncStripe(ctx context.Context, input organizations.SyncStripeInput) (err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	// if current user is not a Markdown Ninja admin then it needs to be a staff of the organization
	if !httpCtx.AccessToken.IsAdmin {
		// var staff organizations.Staff
		_, err = service.repo.FindStaff(ctx, service.db, actorID, input.OrganizationID)
		if err != nil {
			return
		}

		// if staff.Role != organizations.StaffRoleAdministrator {
		// 	err = kernel.ErrPermissionDenied
		// 	return
		// }
	}

	_, err = service.syncOrganizationWithStripeCustomer(ctx, input.OrganizationID)
	return
}

func (service *OrganizationsService) syncOrganizationWithStripeCustomer(ctx context.Context, organizationID guid.GUID) (organization organizations.Organization, err error) {
	logger := slogx.FromCtx(ctx).With(slog.String("organization.id", organizationID.String()))
	now := time.Now().UTC()

	tx, err := service.db.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("organizations.syncOrganizationWithStripeCustomer: Starting DB transaction: %w", err)
		return
	}
	defer tx.Rollback()

	organization, err = service.repo.FindOrganizationByID(ctx, tx, organizationID, true)
	if err != nil {
		return
	}

	// Do nothing. Should we send / log an error?
	if organization.StripeCustomerID == nil {
		return
	}

	logger = logger.With(slog.String("organization.id", organization.ID.String()), slog.String("stripe.customer.id", *organization.StripeCustomerID))

	getCustomerParams := &stripe.CustomerParams{}
	getCustomerParams.AddExpand("tax_ids")
	getCustomerParams.AddExpand("subscriptions")
	getCustomerParams.AddExpand("subscriptions.data.items")
	stripeCustomer, err := stripecustomer.Get(*organization.StripeCustomerID, getCustomerParams)
	if err != nil {
		err = fmt.Errorf("organizations.syncOrganizationWithStripeCustomer: fetching stripe customer: %w", err)
		return
	}

	// Update organization from customer

	organization.BillingInformation.Name = stripeCustomer.Name
	organization.BillingInformation.Email = stripeCustomer.Email
	organization.BillingInformation.AddressLine1 = stripeCustomer.Address.Line1
	organization.BillingInformation.AddressLine2 = stripeCustomer.Address.Line2
	organization.BillingInformation.PostalCode = stripeCustomer.Address.PostalCode
	organization.BillingInformation.City = stripeCustomer.Address.City
	organization.BillingInformation.State = stripeCustomer.Address.State
	organization.BillingInformation.CountryCode = stripeCustomer.Address.Country

	if stripeCustomer.TaxIDs != nil && len(stripeCustomer.TaxIDs.Data) != 0 {
		for _, taxID := range stripeCustomer.TaxIDs.Data {
			if taxID.Type == stripe.TaxIDTypeEUVAT {
				organization.BillingInformation.TaxID = &taxID.Value
				break
			}
		}
	}

	// Update subscription if needed
	if stripeCustomer.Subscriptions != nil {
		nonCanceledSubscriptionsCount := 0

		if len(stripeCustomer.Subscriptions.Data) != 0 {
			for _, stripeSubscription := range stripeCustomer.Subscriptions.Data {
				// ignore canceled subscriptions
				if stripeSubscription.Status == stripe.SubscriptionStatusCanceled {
					continue
				}
				nonCanceledSubscriptionsCount += 1

				organization.StripeSubscriptionID = &stripeSubscription.ID

				if stripeSubscription.Items != nil && stripeSubscription.Items.Data != nil {
					for _, item := range stripeSubscription.Items.Data {
						if item.Price.ID == service.stripeConfig.Prices.Slots {
							organization.ExtraSlots = item.Quantity
						}
					}
				}

				planStr, planOk := stripeSubscription.Metadata["markdown_ninja_plan"]
				if planOk {
					organization.Plan = kernel.PlanID(planStr)
				} else {
					logger.Error("organizations.syncOrganizationWithStripeCustomer: markdown_ninja_plan metadata is missing for subscription")
				}

				stripeSubscriptionStartDate := time.Unix(stripeSubscription.StartDate, 0).UTC()
				if organization.SubscriptionStartedAt == nil ||
					!organization.SubscriptionStartedAt.Equal(stripeSubscriptionStartDate) {
					organization.SubscriptionStartedAt = &stripeSubscriptionStartDate
				}

				if stripeSubscription.Status == stripe.SubscriptionStatusActive {
					organization.PaymentDueSince = nil
				} else if organization.PaymentDueSince == nil &&
					(stripeSubscription.Status == stripe.SubscriptionStatusPastDue ||
						stripeSubscription.Status == stripe.SubscriptionStatusUnpaid) {
					organization.PaymentDueSince = &now
				}
			}
		}

		// if no active subscription
		if len(stripeCustomer.Subscriptions.Data) == 0 || nonCanceledSubscriptionsCount == 0 {
			organization.StripeSubscriptionID = nil
			organization.SubscriptionStartedAt = nil
			organization.Plan = kernel.PlanFree.ID
			organization.ExtraSlots = 0
			organization.UsageLastInvoicedAt = nil
			organization.PaymentDueSince = nil
		} else if nonCanceledSubscriptionsCount > 1 {
			logger.Error("organizations.syncOrganizationWithStripeCustomer: Organization has more than 1 non-canceled subscription")
		}
	}

	organization.UpdatedAt = now
	err = service.repo.UpdateOrganization(ctx, tx, organization)
	if err != nil {
		return
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("organizations.syncOrganizationWithStripeCustomer: Comitting DB transaction: %w", err)
		return
	}

	return
}
