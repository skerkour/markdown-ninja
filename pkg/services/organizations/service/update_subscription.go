package service

import (
	"context"
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	stripecustomer "github.com/stripe/stripe-go/v81/customer"
	stripesubscription "github.com/stripe/stripe-go/v81/subscription"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/log/slogx"
)

func (service *OrganizationsService) UpdateSubscription(ctx context.Context, input organizations.UpdateSubscriptionInput) (ret organizations.UpdateSubscriptionOutput, err error) {
	httpCtx := httpctx.FromCtx(ctx)
	logger := slogx.FromCtx(ctx)

	if service.isSelfHosted {
		return ret, errs.InvalidArgument("Subscriptions can't be changed in self-hosted mode")
	}

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	actorIsAdmin := httpCtx.AccessToken.IsAdmin

	// if current user is not a Markdown Ninja admin then it needs to be an organization admin
	if !actorIsAdmin {
		var staff organizations.Staff
		staff, err = service.repo.FindStaff(ctx, service.db, actorID, input.OrganizationID)
		if err != nil {
			return
		}

		if staff.Role != organizations.StaffRoleAdministrator {
			err = kernel.ErrPermissionDenied
			return
		}
	}

	onlySelfServePlans := true
	if actorIsAdmin {
		onlySelfServePlans = false
	}
	err = validatePlan(input.Plan, onlySelfServePlans)
	if err != nil {
		return
	}

	err = validateExtraSlots(input.ExtraSlots, input.Plan)
	if err != nil {
		return
	}

	now := time.Now().UTC()

	tx, err := service.db.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("organizations.UpdateSubscription: Starting DB transaction: %w", err)
		return
	}
	defer tx.Rollback()

	organization, err := service.repo.FindOrganizationByID(ctx, tx, input.OrganizationID, true)
	if err != nil {
		return
	}

	// Do nothing
	if input.Plan == organization.Plan && input.ExtraSlots == organization.ExtraSlots {
		logger.Debug("organizations.UpdateSubscription: subscription has not changed. Do nothing.")
		return
	}

	err = service.checkCanUpdateSubscription(ctx, organization, input.Plan, input.ExtraSlots)
	if err != nil {
		return
	}

	organization.UpdatedAt = now

	// if plan = free then we cancel subscription
	if input.Plan == kernel.PlanFree.ID {
		if organization.PaymentDueSince != nil {
			err = errs.InvalidArgument("Please pay all your due invoices before cancelling your subscription")
			return
		}

		// StripeSubscriptionID can be null. If it was given as a gift for example
		if organization.StripeSubscriptionID != nil && organization.StripeCustomerID != nil {
			idempotencyKey := fmt.Sprintf("cancel_subscription-%s", *organization.StripeSubscriptionID)
			err = service.invoiceForUsageData(ctx, tx, &organization, idempotencyKey, true)
			if err != nil {
				err = errs.InvalidArgument("There was a problem canceling your subscription. Please contact support if the problem persist")
				return
			}

			params := &stripe.SubscriptionCancelParams{
				// InvoiceNow: stripe.Bool(true),
			}
			_, err = stripesubscription.Cancel(*organization.StripeSubscriptionID, params)
			if err != nil {
				err = fmt.Errorf("organizations.UpdateSubscription: error cancelling stripe subscription for organization [%s]: %w", organization.ID, err)
				return
			}
		}

		// reset all the fields related to subscription
		organization.StripeSubscriptionID = nil
		organization.SubscriptionStartedAt = nil
		organization.Plan = kernel.PlanFree.ID
		organization.ExtraSlots = 0
		organization.UsageLastInvoicedAt = nil
		organization.PaymentDueSince = nil

		err = service.repo.UpdateOrganization(ctx, tx, organization)
		if err != nil {
			return
		}

		err = tx.Commit()
		if err != nil {
			err = fmt.Errorf("organizations.UpdateSubscription: Comitting DB transaction (cancel): %w", err)
			return
		}

		return
	}

	organization.ExtraSlots = input.ExtraSlots

	// if the organization has no stripe customer, then we create one
	if organization.StripeCustomerID == nil {
		var stripeCustomer *stripe.Customer
		createStripeCustomerParams := service.generateStripeCustomerParams(organization)
		createStripeCustomerParams.AddExpand("invoice_settings.default_payment_method")
		stripeCustomer, err = stripecustomer.New(createStripeCustomerParams)
		if err != nil {
			err = fmt.Errorf("organizations.UpdateSubscription: creating Stripe customer: %w", err)
			return
		}

		organization.StripeCustomerID = &stripeCustomer.ID
	}
	// else {
	// 	getStripeCustomerParams := &stripe.CustomerParams{}
	// 	getStripeCustomerParams.AddExpand("invoice_settings.default_payment_method")
	// 	stripeCustomer, err = stripecustomer.Get(*organization.StripeCustomerID, getStripeCustomerParams)
	// 	if err != nil {
	// 		err = fmt.Errorf("organizations.UpdateSubscription: fetching Stripe customer: %w", err)
	// 		return
	// 	}
	// }

	// TODO: if the customer already has a valid (non-expired) payment method attached. Do we really want to
	// create a stripe checkout session?
	// instead we could directly create the subscription and charge the payment method.

	// if there is no active subscription, we create one
	if organization.StripeSubscriptionID == nil {
		paymentMethodToUse, _ := service.getDefaultPaymentMethodForStripeCustomer(ctx, *organization.StripeCustomerID)
		if paymentMethodToUse != nil {
			// if a payment method is available, try to charge directly
			var newSubscriptionNewLineItems []*stripe.SubscriptionItemsParams
			newSubscriptionNewLineItems, err = service.getStripeSubscriptionLineItemsForPlan(input.Plan, organization.ExtraSlots)
			if err != nil {
				err = fmt.Errorf("organizations.UpdateSubscription: %w", err)
				return
			}

			// var newSubscription *stripe.Subscription
			createSubscriptionParams := &stripe.SubscriptionParams{
				Customer:             stripe.String(*organization.StripeCustomerID),
				Items:                newSubscriptionNewLineItems,
				DefaultPaymentMethod: stripe.String(paymentMethodToUse.ID),
				Metadata: map[string]string{
					"markdown_ninja_organization_id": organization.ID.String(),
					"markdown_ninja_plan":            string(input.Plan),
				},
			}
			_, err = stripesubscription.New(createSubscriptionParams)
			if err != nil {
				return
			}
			// the subscription id will be updated when receiving the webhooks / syncing Stripe
			// organization.StripeSubscriptionID = &newSubscription.ID
		} else {
			// otherwise redirecto to stripe Checkout
			var stripeCheckoutSession *stripe.CheckoutSession
			var stripeCheckoutSessionLineItems []*stripe.CheckoutSessionLineItemParams
			stripeCheckoutSessionLineItems, err = service.getStripeCheckoutSessionLineItemsForPlan(input.Plan, organization.ExtraSlots)
			if err != nil {
				err = fmt.Errorf("organizations.UpdateSubscription: %w", err)
				return
			}

			// billingAnchor := timeutil.GetFirstDayOfNextMonth(time.Now().UTC()).UTC()
			checkoutParams := service.generateStripeCheckoutSessionParams(organization, input.Plan, stripeCheckoutSessionLineItems)
			stripeCheckoutSession, err = session.New(checkoutParams)
			if err != nil {
				err = fmt.Errorf("organizations.UpdateSubscription: error creating stripe checkout session: %w", err)
				return
			}

			ret.StripeCheckoutSessionUrl = &stripeCheckoutSession.URL
		}

	} else {
		// otherwise, we update the subscription
		var stripeSubscription *stripe.Subscription
		getStripeSubscriptionParams := &stripe.SubscriptionParams{}
		stripeSubscription, err = stripesubscription.Get(*organization.StripeSubscriptionID, getStripeSubscriptionParams)
		if err != nil {
			err = fmt.Errorf("organizations.UpdateSubscription: fetching stripe subscription for organization [%s]: %w", organization.ID, err)
			return
		}

		// we first delete the existing subscription items
		updateStripeSubscriptionParams := &stripe.SubscriptionParams{
			ProrationBehavior: stripe.String(string(stripe.SubscriptionSchedulePhaseProrationBehaviorAlwaysInvoice)),
			Metadata: map[string]string{
				"markdown_ninja_organization_id": organization.ID.String(),
				"markdown_ninja_plan":            string(input.Plan),
			},
		}

		// existing subscription items are removed
		// https://docs.stripe.com/billing/subscriptions/upgrade-downgrad
		for _, existingSubscriptionItem := range stripeSubscription.Items.Data {
			item := &stripe.SubscriptionItemsParams{
				ID:      stripe.String(existingSubscriptionItem.ID),
				Deleted: stripe.Bool(true),
			}
			updateStripeSubscriptionParams.Items = append(updateStripeSubscriptionParams.Items, item)
		}

		var subscriptionNewLineItems []*stripe.SubscriptionItemsParams
		subscriptionNewLineItems, err = service.getStripeSubscriptionLineItemsForPlan(input.Plan, organization.ExtraSlots)
		if err != nil {
			err = fmt.Errorf("organizations.UpdateSubscription: %w", err)
			return
		}

		updateStripeSubscriptionParams.Items = append(updateStripeSubscriptionParams.Items, subscriptionNewLineItems...)

		_, err = stripesubscription.Update(*organization.StripeSubscriptionID, updateStripeSubscriptionParams)
		if err != nil {
			err = fmt.Errorf("organizations.UpdateSubscription: error updating stripe subscription: %w", err)
			return
		}
	}

	// The plan will be updated when receiving Stripe webhook / syncing the organization with Stripe
	// organization.Plan = input.Plan
	err = service.repo.UpdateOrganization(ctx, tx, organization)
	if err != nil {
		return
	}

	err = tx.Commit()
	if err != nil {
		err = fmt.Errorf("organizations.UpdateSubscription: Comitting DB transaction: %w", err)
		return
	}

	return
}

func (service *OrganizationsService) checkCanUpdateSubscription(ctx context.Context, organization organizations.Organization, newPlanID kernel.PlanID,
	newExtraSlots int64) (err error) {
	if service.blockSubscriptions && organization.Plan == kernel.PlanFree.ID && newPlanID != kernel.PlanFree.ID {
		return errs.InvalidArgument("New subscriptions are currently disabled. Please contact support for further information.")
	}

	usageData, err := service.getOrganizationBillingUsage(ctx, service.db, organization)
	if err != nil {
		return
	}

	newPlan := kernel.AllPlans[newPlanID]

	if newExtraSlots > newPlan.MaxExtraSlots {
		return errs.InvalidArgument(fmt.Sprintf("Too many extra slots for plan %s. Please upgrade to a higher plan to get more extra slots.", newPlanID))
	}

	// websites
	if usageData.UsedWebsites > (1 + newExtraSlots) {
		return errs.InvalidArgument("You have too many websites. Please delete some websites to stay under the allowed quota of your new plan.")
	}

	// storage
	if usageData.UsedStorage > (newPlan.AllowedStorage + (newExtraSlots * kernel.StoragePerSlot)) {
		return errs.InvalidArgument("Too much storage used. Please delete some assets to stay under the allowed quota of your new plan.")
	}

	// staffs
	if usageData.UsedStaffs > (1 + newExtraSlots) {
		return errs.InvalidArgument("You have too many staffs. Please remove some staffs from your organization to stay under the allowed quota of your new plan.")
	}

	return
}
