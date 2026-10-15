package store

import "markdown.ninja/stdx/guid"

type JobSendOrderConfirmationEmail struct {
	OrderID guid.GUID `json:"order_id"`
}

func (JobSendOrderConfirmationEmail) JobType() string {
	return "store.send_order_confirmation_email"
}

type JobCreateStripeRefund struct {
	RefundID guid.GUID `json:"refund_id"`
}

func (JobCreateStripeRefund) JobType() string {
	return "store.create_stripe_refund"
}

type JobSyncRefundWithStripe struct {
	RefundID guid.GUID `json:"refund_id"`
}

func (JobSyncRefundWithStripe) JobType() string {
	return "store.sync_refund_with_stripe"
}
