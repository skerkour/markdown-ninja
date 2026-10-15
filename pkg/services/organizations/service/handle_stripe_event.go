package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/stripe/stripe-go/v81"
	stripecustomer "github.com/stripe/stripe-go/v81/customer"
	"github.com/stripe/stripe-go/v81/paymentintent"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/retry"
)

func (service *OrganizationsService) HandleStripeEvent(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	logger.Debug("organizations: Stripe event received", slog.String("event.type", string(stripeEvent.Type)))
	// fmt.Println(string(stripeEvent.Data.Raw))

	// List of all Stripe events:
	// https://docs.stripe.com/api/events/types

	switch stripeEvent.Type {
	case "charge.failed":
		return service.handleStripeEventCharge(ctx, stripeEvent)

	case "checkout.session.completed", "checkout.session.expired":
		return service.handleStripeEventCheckoutSession(ctx, stripeEvent)

	case "customer.updated":
		return service.handleStripeEventCustomer(ctx, stripeEvent)

	case "customer.subscription.created",
		"customer.subscription.updated",
		"customer.subscription.deleted":
		return service.handleStripeEventSubscription(ctx, stripeEvent)

	case "invoice.paid":
		return service.handleStripeEventInvoice(ctx, stripeEvent)

	case "payment_method.attached":
		return service.handleStripeEventPaymentMethodAttached(ctx, stripeEvent)
	}

	return nil
}

// handleStripeEventPaymentMethodAttached is used to set newly attached payment methods to Stripe customers
// as their default payment method
func (service *OrganizationsService) handleStripeEventPaymentMethodAttached(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripePaymentMethod := &stripe.PaymentMethod{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripePaymentMethod)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventCheckoutSessionCompleted: error parsing event JSON: %w", err)
	}

	if _, isWebsiteEvent := stripePaymentMethod.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	if stripePaymentMethod.Customer == nil || stripePaymentMethod.Customer.ID == "" {
		logger.Error("organizations.handleStripeEventCheckoutSessionCompleted: customer is null",
			slog.String("stripe.payment_method.id", stripePaymentMethod.ID))
		return nil
	}

	getCustomerParams := &stripe.CustomerParams{}
	getCustomerParams.AddExpand("sources")
	getCustomerParams.AddExpand("invoice_settings.default_payment_method")
	getCustomerParams.AddExpand("default_source")
	getCustomerParams.AddExpand("subscriptions")
	stripeCustomer, err := stripecustomer.Get(stripePaymentMethod.Customer.ID, getCustomerParams)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventCheckoutSessionCompleted: error fetching stripe customer: %w", err)
	}

	updateStripeCustomerParams := &stripe.CustomerParams{
		InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(stripePaymentMethod.ID),
		},
	}
	_, err = stripecustomer.Update(stripeCustomer.ID, updateStripeCustomerParams)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventCheckoutSessionCompleted: error updating stripe customer: %w", err)
	}

	return nil
}

func (service *OrganizationsService) handleStripeEventCustomer(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripeCustomer := &stripe.Customer{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripeCustomer)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventCustomer: error parsing event JSON: %w", err)
	}

	if _, isWebsiteEvent := stripeCustomer.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	organization, err := service.repo.FindOrganizationByStripeCustomerID(ctx, service.db, stripeCustomer.ID, false)
	if err != nil {
		if errs.IsNotFound(err) {
			logger.Error("organizations.handleStripeEventCustomer: organization not found for stripe customer",
				slog.String("stripe.customer.id", stripeCustomer.ID))
			return nil
		}
		return err
	}

	_, err = service.syncOrganizationWithStripeCustomer(ctx, organization.ID)
	return err
}

func (service *OrganizationsService) handleStripeEventSubscription(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripeSubscription := &stripe.Subscription{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripeSubscription)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventSubscription: error parsing event JSON: %w", err)
	}

	if _, isWebsiteEvent := stripeSubscription.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	if stripeSubscription.Customer == nil || stripeSubscription.Customer.ID == "" {
		logger.Error("organizations.handleStripeEventSubscription: customer is null",
			slog.String("stripe.subscription.id", stripeSubscription.ID))
		return nil
	}

	organization, err := service.repo.FindOrganizationByStripeCustomerID(ctx, service.db, stripeSubscription.Customer.ID, false)
	if err != nil {
		if errs.IsNotFound(err) {
			logger.Error("organizations.handleStripeEventSubscription: organization not found for stripe customer",
				slog.String("stripe.subscription.id", stripeSubscription.ID), slog.String("stripe.customer.id", stripeSubscription.Customer.ID))
			return nil
		}
		return err
	}

	_, err = service.syncOrganizationWithStripeCustomer(ctx, organization.ID)
	return err
}

func (service *OrganizationsService) handleStripeEventInvoice(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripeInvoice := &stripe.Invoice{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripeInvoice)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeInvoiceEventInvoice: error parsing event JSON: %w", err)
	}

	if _, isWebsiteEvent := stripeInvoice.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	if stripeInvoice.PaymentIntent == nil {
		logger.Error("organizations.handleStripeEventInvoice: invoice.payment_intent is null",
			slog.String("invoice.id", stripeInvoice.ID),
			slog.Group("event", slog.String("id", stripeEvent.ID), slog.String("type", string(stripeEvent.Type))),
		)
		return nil
	}

	// it's not possible to set metdata to invoices with stripe checkout sessions, so we need to fetch the payment intent
	// to get the metadata
	var stripePaymentIntent *stripe.PaymentIntent
	getStripePaymentIntentParams := stripe.PaymentIntentParams{}
	err = retry.Do(func() (retryErr error) {
		stripePaymentIntent, retryErr = paymentintent.Get(
			stripeInvoice.PaymentIntent.ID,
			&getStripePaymentIntentParams,
		)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(3), retry.Delay(20*time.Millisecond))
	if err != nil {
		return fmt.Errorf("store.handleStripeEventInvoice: error getting stripe payment intent for invoice[%s]: %w", stripeInvoice.ID, err)
	}

	if _, isWebsiteEvent := stripePaymentIntent.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	if stripeInvoice.Customer == nil || stripeInvoice.Customer.ID == "" {
		logger.Error("organizations.handleStripeInvoiceEventInvoice: customer is null",
			slog.String("stripe.invoice.id", stripeInvoice.ID))
		return nil
	}

	organization, err := service.repo.FindOrganizationByStripeCustomerID(ctx, service.db, stripeInvoice.Customer.ID, false)
	if err != nil {
		if errs.IsNotFound(err) {
			logger.Error("organizations.handleStripeInvoiceEventInvoice: organization not found for stripe customer",
				slog.String("stripe.invoice.id", stripeInvoice.ID), slog.String("stripe.customer.id", stripeInvoice.Customer.ID))
			return nil
		}
		return err
	}

	_, err = service.syncOrganizationWithStripeCustomer(ctx, organization.ID)
	return err
}

func (service *OrganizationsService) handleStripeEventCharge(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripeCharge := &stripe.Charge{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripeCharge)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventCharge: error parsing event JSON: %w", err)
	}

	if _, isWebsiteEvent := stripeCharge.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	if stripeCharge.Customer == nil || stripeCharge.Customer.ID == "" {
		logger.Warn("organizations.handleStripeEventCharge: customer is null for charge",
			slog.String("stripe.charge.id", stripeCharge.ID))
		return nil
	}

	organization, err := service.repo.FindOrganizationByStripeCustomerID(ctx, service.db, stripeCharge.Customer.ID, false)
	if err != nil {
		if errs.IsNotFound(err) {
			logger.Error("organizations.handleStripeEventCharge: organization not found for stripe customer",
				slog.String("stripe.charge.id", stripeCharge.ID), slog.String("stripe.customer.id", stripeCharge.Customer.ID))
			return nil
		}
		return err
	}

	_, err = service.syncOrganizationWithStripeCustomer(ctx, organization.ID)
	return err
}

func (service *OrganizationsService) handleStripeEventCheckoutSession(ctx context.Context, stripeEvent stripe.Event) error {
	stripeCheckoutSession := stripe.CheckoutSession{}
	err := json.Unmarshal(stripeEvent.Data.Raw, &stripeCheckoutSession)
	if err != nil {
		return fmt.Errorf("organizations.handleStripeEventCheckoutSession: error parsing event JSON: %w", err)
	}

	if _, isWebsiteEvent := stripeCheckoutSession.Metadata["markdown_ninja_website_id"]; isWebsiteEvent {
		return service.storeService.HandleStripeEvent(ctx, stripeEvent)
	}

	return nil
}

// func (service *OrganizationsService) handleStripeEventSubscriptionCreated(ctx context.Context, stripeEvent stripe.Event) (err error) {
// 	logger := slogx.FromCtx(ctx)

// 	stripeSubscription := &stripe.Subscription{}
// 	err = json.Unmarshal(stripeEvent.Data.Raw, stripeSubscription)
// 	if err != nil {
// 		return fmt.Errorf("organizations.handleStripeEventSubscriptionCreated: error parsing event JSON: %w", err)
// 	}

// 	now := time.Now().UTC()
// 	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
// 		var organization organizations.Organization
// 		organization, txErr = service.repo.FindOrganizationByStripeCustomerID(ctx, tx, stripeSubscription.Customer.ID, true)
// 		if txErr != nil {
// 			if errs.IsNotFound(txErr) {
// 				logger.Error("organizations.handleStripeEventSubscriptionCreated: organization not found for stripe customer",
// 					slog.String("stripe.customer.id", stripeSubscription.Customer.ID))
// 				return nil
// 			}
// 			return txErr
// 		}

// 		organization.StripeSubscriptionID = &stripeSubscription.ID
// 		subscriptionStartedAt := time.Unix(stripeSubscription.StartDate, 0).UTC()
// 		organization.SubscriptionStartedAt = &subscriptionStartedAt
// 		organization.UpdatedAt = now
// 		txErr = service.repo.UpdateOrganization(ctx, tx, organization)
// 		return txErr
// 	})
// 	if err != nil {
// 		return
// 	}

// 	return
// }

// func (service *OrganizationsService) handleStripeEventSubscriptionDeleted(ctx context.Context, stripeEvent stripe.Event) (err error) {
// 	logger := slogx.FromCtx(ctx)

// 	stripeSubscription := &stripe.Subscription{}
// 	err = json.Unmarshal(stripeEvent.Data.Raw, stripeSubscription)
// 	if err != nil {
// 		return fmt.Errorf("organizations.handleStripeEventSubscriptionDeleted: error parsing event JSON: %w", err)
// 	}

// 	now := time.Now().UTC()
// 	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
// 		var organization organizations.Organization
// 		organization, txErr = service.repo.FindOrganizationByStripeCustomerID(ctx, tx, stripeSubscription.Customer.ID, true)
// 		if txErr != nil {
// 			if errs.IsNotFound(txErr) {
// 				logger.Error("organizations.handleStripeEventSubscriptionDeleted: organization not found for stripe customer",
// 					slog.String("stripe.customer.id", stripeSubscription.Customer.ID))
// 				return nil
// 			}
// 			return txErr
// 		}

// 		organization.StripeSubscriptionID = nil
// 		organization.SubscriptionStartedAt = nil
// 		organization.Plan = organizations.PlanFree
// 		organization.ExtraStaffs = 0
// 		organization.ExtraWebsites = 0
// 		organization.ExtraStorage = 0

// 		organization.UpdatedAt = now
// 		txErr = service.repo.UpdateOrganization(ctx, tx, organization)
// 		return txErr
// 	})
// 	if err != nil {
// 		return
// 	}

// 	return nil
// }
