package service

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/retry"
)

func (service *StoreService) CompleteOrder(ctx context.Context, input store.CompleteOrderInput) (err error) {
	// authenticatedContact := service.contactsService.CurrentContact(ctx)
	// if authenticatedContact == nil {
	// 	err = kernel.ErrAuthenticationRequired
	// 	return
	// }

	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		return
	}

	return service.completeOrder(ctx, input.OrderID, website.ID)
}

func (service *StoreService) completeOrder(ctx context.Context, orderID guid.GUID, websiteID guid.GUID) error {
	now := time.Now().UTC()
	logger := slogx.FromCtx(ctx).With(slog.String("order.id", orderID.String()), slog.String("website.id", websiteID.String()))

	tx, err := service.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("kernel.completeOrder: Starting DB transaction: %w", err)
	}
	defer tx.Rollback()

	order, err := service.repo.FindOrderByID(ctx, tx, orderID, true)
	if err != nil {
		return err
	}

	country := order.Country

	if !websiteID.Equal(order.WebsiteID) {
		return store.ErrOrderNotFound(orderID)
	}

	contact, err := service.contactsService.FindContact(ctx, tx, order.ContactID)
	if err != nil {
		return err
	}

	contactSubscribedToNewsletter := false
	if !contact.Verified && contact.SubscribedToNewsletterAt != nil {
		contactSubscribedToNewsletter = true
	}

	var stripeCheckoutSession *stripe.CheckoutSession
	getStripeCheckoutSessionParam := stripe.CheckoutSessionParams{}
	getStripeCheckoutSessionParam.AddExpand("payment_intent")
	getStripeCheckoutSessionParam.AddExpand("payment_intent.invoice")
	getStripeCheckoutSessionParam.AddExpand("customer")
	getStripeCheckoutSessionParam.AddExpand("line_items")
	getStripeCheckoutSessionParam.AddExpand("line_items.data.price.product")

	err = retry.Do(func() (retryErr error) {
		stripeCheckoutSession, retryErr = session.Get(
			order.StripeCheckoutSessionID,
			&getStripeCheckoutSessionParam,
		)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(3), retry.Delay(20*time.Millisecond))
	if err != nil {
		return fmt.Errorf("store.completeOrder: error getting stripe checkout session: %w", err)
	}

	if order.Status == store.OrderStatusCompleted {
		// if order is already completed but stripe invoice is not saved yet
		if order.StripeInvoiceID == nil || order.StripeInvoiceUrl == nil ||
			(order.StripeInvoiceUrl != nil && *order.StripeInvoiceUrl == "") {
			if stripeCheckoutSession.PaymentIntent != nil && stripeCheckoutSession.PaymentIntent.Invoice != nil {
				order.UpdatedAt = now
				order.StripeInvoiceID = &stripeCheckoutSession.PaymentIntent.Invoice.ID
				order.StripeInvoiceUrl = &stripeCheckoutSession.PaymentIntent.Invoice.HostedInvoiceURL
				err = service.repo.UpdateOrder(ctx, tx, order)
				if err != nil {
					return err
				}
				err = tx.Commit()
				if err != nil {
					return fmt.Errorf("store.completeOrder: Comitting DB transaction (StripeInvoice) for order [%s]: %w", orderID.String(), err)
				}
			}
		}

		// otherwise, do nothing if order is already completed
		return nil
	}

	if stripeCheckoutSession.Status == stripe.CheckoutSessionStatusExpired ||
		(stripeCheckoutSession.PaymentIntent != nil && stripeCheckoutSession.PaymentIntent.Status == stripe.PaymentIntentStatusCanceled) {

		if order.Status == store.OrderStatusCanceled {
			// do nothing
			return nil
		}

		order.UpdatedAt = now
		// order.BillingAddress = billingAddress
		order.Status = store.OrderStatusCanceled
		if order.CanceledAt == nil {
			order.CanceledAt = &now
		}
		if stripeCheckoutSession.PaymentIntent != nil {
			order.StripPaymentItentID = &stripeCheckoutSession.PaymentIntent.ID
			order.TotalAmount = stripeCheckoutSession.PaymentIntent.Amount
		}

		err = service.repo.UpdateOrder(ctx, tx, order)
		if err != nil {
			return fmt.Errorf("store.completeOrder: updating order (CheckoutSessionStatusExpired): %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("store.completeOrder: Comitting DB transaction (CheckoutSessionStatusExpired) for order [%s]: %w", orderID.String(), err)
		}

		service.eventsService.TrackOrderCanceled(ctx, events.TrackOrderCanceledInput{
			OrderID:   order.ID,
			WebsiteID: order.WebsiteID,
			Country:   country,
		})

		return nil
	}

	var stripeCustomerID *string
	if stripeCheckoutSession.Customer != nil {
		stripeCustomerID = &stripeCheckoutSession.Customer.ID
	}

	if stripeCheckoutSession.CustomerDetails != nil && stripeCheckoutSession.CustomerDetails.Address != nil &&
		stripeCheckoutSession.CustomerDetails.Address.Country != "" {
		country = stripeCheckoutSession.CustomerDetails.Address.Country
	} else {
		if stripeCheckoutSession.CustomerDetails == nil {
			logger.Warn("store.completeOrder: stripeCheckoutSession.CustomerDetails is null")
		} else if stripeCheckoutSession.CustomerDetails != nil && stripeCheckoutSession.CustomerDetails.Address == nil {
			logger.Warn("store.completeOrder: stripeCheckoutSession.CustomerDetails.Address is null")
		}
	}

	checkoutSessionLogArgs := []any{}
	if stripeCheckoutSession.PaymentIntent != nil {
		checkoutSessionLogArgs = append(checkoutSessionLogArgs, slog.Group("payment_intent",
			slog.String("id", stripeCheckoutSession.PaymentIntent.ID),
			slog.String("status", string(stripeCheckoutSession.PaymentIntent.Status)),
			slog.Bool("invoice_is_present", stripeCheckoutSession.PaymentIntent.Invoice != nil),
			slog.Bool("customer_is_present", stripeCheckoutSession.Customer != nil),
		))
	} else {
		checkoutSessionLogArgs = append(checkoutSessionLogArgs, slog.Any("payment_intent", nil))
	}

	logger.Debug("store.completeOrder: successfully fetched stripe.CheckoutSession", checkoutSessionLogArgs...)

	if stripeCheckoutSession.PaymentIntent == nil {
		return nil
	}

	if stripeCheckoutSession.PaymentIntent.Status != stripe.PaymentIntentStatusSucceeded {
		// return store.ErrOrderIsNotCompleted
		logger.Error(fmt.Sprintf("store.completeOrder: invalid Stripe payment intent status: %s. expected: succeeded", stripeCheckoutSession.PaymentIntent.Status))
		return nil
	}

	order.UpdatedAt = now
	order.StripPaymentItentID = &stripeCheckoutSession.PaymentIntent.ID
	order.TotalAmount = stripeCheckoutSession.PaymentIntent.Amount / 100
	order.Country = country
	order.CompletedAt = &now
	order.Status = store.OrderStatusCompleted
	if stripeCheckoutSession.PaymentIntent.Invoice != nil {
		order.StripeInvoiceID = &stripeCheckoutSession.PaymentIntent.Invoice.ID
		order.StripeInvoiceUrl = &stripeCheckoutSession.PaymentIntent.Invoice.HostedInvoiceURL
	}

	products, err := service.repo.FindProductsForOrder(ctx, tx, order.ID)
	if err != nil {
		return err
	}

	productsByID := make(map[string]store.Product, len(products))
	for _, product := range products {
		productsByID[product.ID.String()] = product
	}

	orderedProductsQuantity := make(map[string]int64, len(products))
	for _, lineItem := range stripeCheckoutSession.LineItems.Data {
		orderedProductsQuantity[lineItem.Price.Product.Metadata["markdown_ninja_product_id"]] = lineItem.Quantity
	}

	orderLineItems, err := service.repo.FindOrderLineItems(ctx, tx, order.ID)
	if err != nil {
		return err
	}

	for _, lineItem := range orderLineItems {
		purchasedQuantity := orderedProductsQuantity[lineItem.ProductID.String()]
		// if purchasedQuantity  == 0 it means that it was not found in orderedProductsQuantity
		if purchasedQuantity != 0 && purchasedQuantity != lineItem.Quantity {
			lineItem.Quantity = purchasedQuantity
			err = service.repo.UpdateOrderLineItem(ctx, tx, lineItem)
			if err != nil {
				return err
			}
		}
	}

	// TODO: do we really mark the contact as verified?
	// but otherwise we can't see them in the dashbaord...
	updateContactInput := contacts.UpdateContactInput{
		ID:               contact.ID,
		Verified:         new(true),
		Country:          &country,
		StripeCustomerID: stripeCustomerID,
	}
	err = service.contactsService.UpdateContactInternal(ctx, tx, &contact, updateContactInput)
	if err != nil {
		return err
	}

	err = service.repo.UpdateOrder(ctx, tx, order)
	if err != nil {
		return err
	}

	// give the customer access to the purchased products
	for _, product := range products {
		_, err = service.repo.FindContactProductAccess(ctx, tx, order.ContactID, product.ID)
		if err == nil {
			// if contact already has access to product we don't need to create product-access relation
			continue
		} else {
			if !errs.IsNotFound(err) {
				return err
			}
			err = nil
		}

		contactProductAccess := store.ContactProductAccess{
			CreatedAt: now,
			ContactID: order.ContactID,
			ProductID: product.ID,
		}
		err = service.repo.CreateContactProductAccess(ctx, tx, contactProductAccess)
		if err != nil {
			if db.IsErrAlreadyExists(err) {
				// nothing has to be done if contact already has access to product
				err = nil
			} else {
				return err
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("store.completeOrder: Comitting DB transaction for order [%s]: %w", orderID.String(), err)
	}

	go func() {
		job := queue.NewJobInput{
			Data: store.JobSendOrderConfirmationEmail{
				OrderID: order.ID,
			},
			RetryDelay: new(int64(120)),
			RetryMax:   new(int64(10)),
		}
		errQueue := service.queue.Push(context.Background(), nil, job)
		if errQueue != nil {
			logger.Error("store.completeOrder: error pushing job JobSendOrderConfirmationEmail to queue", slogx.Err(errQueue))
			return
		}
	}()

	service.eventsService.TrackOrderCompleted(ctx, events.TrackOrderCompletedInput{
		OrderID:     order.ID,
		WebsiteID:   order.WebsiteID,
		TotalAmount: order.TotalAmount,
	})

	if contactSubscribedToNewsletter {
		service.eventsService.TrackSubscribedToNewsletter(ctx, events.TrackSubscribedToNewsletterInput{
			WebsiteID: order.WebsiteID,
		})
	}

	return nil
}
