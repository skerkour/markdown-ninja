package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/paymentintent"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/retry"
)

func (service *StoreService) HandleStripeEvent(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	logger.Debug("store: Stripe event received", slog.String("event.type", string(stripeEvent.Type)))

	switch stripeEvent.Type {
	case "checkout.session.completed", "checkout.session.expired":
		return service.handleStripeEventCheckoutSession(ctx, stripeEvent)
	case "invoice.paid":
		return service.handleStripeEventInvoice(ctx, stripeEvent)
	}

	return nil
}

func (service *StoreService) handleStripeEventCheckoutSession(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripeCheckoutSession := &stripe.CheckoutSession{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripeCheckoutSession)
	if err != nil {
		return fmt.Errorf("store.stripeCheckoutSession: error parsing event JSON: %w", err)
	}

	logger = logger.With(
		slog.Group("stripe.event",
			slog.String("id", stripeEvent.ID),
			slog.String("type", string(stripeEvent.Type)),
		),
		slog.Group("stripe.checkout_session",
			slog.String("id", stripeCheckoutSession.ID),
			slog.String("status", string(stripeCheckoutSession.Status)),
		),
	)

	ctx = slogx.ToCtx(ctx, logger)

	if _, isOrganizationEvent := stripeCheckoutSession.Metadata["markdown_ninja_organization_id"]; isOrganizationEvent {
		// for now only log it to avoid infinite calls betewen the two services
		logger.Error("store.handleStripeEventCheckoutSession: received an organization event")
		return nil
	}

	websiteIDStr := stripeCheckoutSession.Metadata["markdown_ninja_website_id"]
	if websiteIDStr == "" {
		logger.Error("store.handleStripeEventCheckoutSession: checkout_session.metadata.markdown_ninja_website_id is empty")
		return nil
	}
	websiteID, err := guid.Parse(websiteIDStr)
	if err != nil {
		return fmt.Errorf("store.handleStripeEventCheckoutSession: error parsing checkout_session.metadata.markdown_ninja_website_id[%s]: %w", websiteIDStr, err)
	}

	orderIDStr := stripeCheckoutSession.Metadata["markdown_ninja_order_id"]
	if orderIDStr == "" {
		logger.Error("store.handleStripeEventCheckoutSession: checkout_session.metadata.markdown_ninja_order_id is empty")
		return nil
	}

	orderID, err := guid.Parse(orderIDStr)
	if err != nil {
		return fmt.Errorf("store.handleStripeEventCheckoutSession: error parsing checkout_session.metadata.markdown_ninja_order_id[%s]: %w", orderIDStr, err)
	}

	return service.completeOrder(ctx, orderID, websiteID)
}

func (service *StoreService) handleStripeEventInvoice(ctx context.Context, stripeEvent stripe.Event) error {
	logger := slogx.FromCtx(ctx)

	stripeInvoice := &stripe.Invoice{}
	err := json.Unmarshal(stripeEvent.Data.Raw, stripeInvoice)
	if err != nil {
		return fmt.Errorf("store.handleStripeInvoiceEventInvoice: error parsing event JSON: %w", err)
	}

	logger = logger.With(
		slog.Group("stripe.event",
			slog.String("id", stripeEvent.ID),
			slog.String("type", string(stripeEvent.Type)),
		),
		slog.Group("stripe.invoice",
			slog.String("id", stripeInvoice.ID),
			slog.String("status", string(stripeInvoice.Status)),
		),
	)

	ctx = slogx.ToCtx(ctx, logger)

	if _, isOrganizationEvent := stripeInvoice.Metadata["markdown_ninja_organization_id"]; isOrganizationEvent {
		// for now only log it to avoid infinite calls betewen the two services
		logger.Error("store.handleStripeEventInvoice: received an organization event")
		return nil
	}

	if stripeInvoice.PaymentIntent == nil {
		logger.Error("store.handleStripeEventInvoice: invoice.payment_intent is null")
		return nil
	}

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
		return fmt.Errorf("store.handleStripeEventInvoice: error getting stripe payment intent for invoice [%s]: %w", stripeInvoice.ID, err)
	}

	orderIdStr := stripePaymentIntent.Metadata["markdown_ninja_order_id"]
	if orderIdStr == "" {
		return nil
	}

	orderID, err := guid.Parse(orderIdStr)
	if err != nil {
		return fmt.Errorf("store.handleStripeEventInvoice: error parsing payment_intent.metadata[markdown_ninja_order_id] (%s) for invoice [%s]: %w", orderIdStr, stripeInvoice.ID, err)
	}

	websiteIDStr := stripePaymentIntent.Metadata["markdown_ninja_website_id"]
	if websiteIDStr == "" {
		return nil
	}

	websiteID, err := guid.Parse(websiteIDStr)
	if err != nil {
		return fmt.Errorf("store.handleStripeEventInvoice: error parsing payment_intent.metadata[markdown_ninja_website_id] (%s) for invoice [%s]: %w", orderIdStr, stripeInvoice.ID, err)
	}

	return service.completeOrder(ctx, orderID, websiteID)
}
