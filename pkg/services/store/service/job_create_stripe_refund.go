package service

import (
	"context"
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v81"
	striperefund "github.com/stripe/stripe-go/v81/refund"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/retry"
)

func (service *StoreService) JobCreateStripeRefund(ctx context.Context, input store.JobCreateStripeRefund) (err error) {
	refund, err := service.repo.FindRefundByID(ctx, service.db, input.RefundID)
	if err != nil {
		return err
	}

	order, err := service.repo.FindOrderByID(ctx, service.db, refund.OrderID, false)
	if err != nil {
		return
	}

	// TODO: correct conversion
	stripeRefundReason := string(refund.Reason)
	stripeRefundCurrendy := string(refund.Currency)
	stripeRefundParams := &stripe.RefundParams{
		Amount:        &refund.Amount,
		Currency:      &stripeRefundCurrendy,
		Reason:        &stripeRefundReason,
		PaymentIntent: order.StripPaymentItentID,
		Metadata: map[string]string{
			"markdown_ninja_website_id": refund.WebsiteID.String(),
			"markdown_ninja_order_id":   refund.OrderID.String(),
		},
	}
	stripeRefund, err := striperefund.New(stripeRefundParams)
	if err != nil {
		err = fmt.Errorf("store.JobCreateStripeRefund: error creating Stripe refund (%s): %w", refund.ID.String(), err)
		return err
	}

	refund.UpdatedAt = time.Now().UTC()
	refund.StripeRefundID = &stripeRefund.ID
	// we retry to be sure that the refund is created
	err = retry.Do(func() error {
		return service.repo.UpdateRefund(ctx, service.db, refund)
	}, retry.Context(ctx), retry.Attempts(5), retry.Delay(1*time.Second))
	if err != nil {
		return err
	}

	return nil
}
