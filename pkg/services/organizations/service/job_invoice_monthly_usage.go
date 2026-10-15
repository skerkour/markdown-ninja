package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/invoice"
	"github.com/stripe/stripe-go/v81/invoiceitem"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/timeutil"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/retry"
)

func (service *OrganizationsService) JobInvoiceMonthlyUsage(ctx context.Context, input organizations.JobInvoiceMonthlyUsage) error {
	logger := slogx.FromCtx(ctx)

	err := service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		organization, txErr := service.repo.FindOrganizationByID(ctx, service.db, input.OrganizationID, true)
		if txErr != nil {
			if errs.IsNotFound(txErr) {
				logger.Warn("organizations.JobSendOrganizationUsageData: organization not found",
					slog.String("organization.id", input.OrganizationID.String()))
				return nil
			}
			return txErr
		}

		txErr = service.invoiceForUsageData(ctx, tx, &organization, input.IdempotencyKey, false)
		if txErr != nil {
			return txErr
		}

		now := time.Now().UTC()
		organization.UpdatedAt = now
		return service.repo.UpdateOrganization(ctx, service.db, organization)
	})
	if err != nil {
		return err
	}

	return nil
}

func (service *OrganizationsService) invoiceForUsageData(ctx context.Context, db db.Queryer, organization *organizations.Organization, idempotencyKey string, cancelingSubscription bool) (err error) {
	logger := slogx.FromCtx(ctx)

	if organization.StripeCustomerID == nil || organization.StripeSubscriptionID == nil {
		return nil
	}

	now := time.Now().UTC()

	invoicePeriodStart := timeutil.GetFirstDayOfLastMonth(now)
	if organization.UsageLastInvoicedAt != nil {
		// if this customer has already been invoiced for usage data, we continue where we left
		invoicePeriodStart = *organization.UsageLastInvoicedAt
	} else if organization.SubscriptionStartedAt != nil && organization.SubscriptionStartedAt.After(invoicePeriodStart) {
		// else if this customer has subscribed recently, we use the subscribtion date
		invoicePeriodStart = *organization.SubscriptionStartedAt
	}

	invoicePeriodEnd := timeutil.GetLastHourOfLastMonth(now)
	// if the user is canceling their subscription, we invoice all the usage since the last invoice
	if cancelingSubscription {
		invoicePeriodEnd = now
	}

	organization.UsageLastInvoicedAt = &invoicePeriodEnd

	emailsSent, err := service.eventsService.GetEmailsSentCountForOrganization(ctx, db, organization.ID, invoicePeriodStart, invoicePeriodEnd)
	if err != nil {
		return fmt.Errorf("organizations.invoiceForUsageData: %w", err)
	}

	// if no email was sent during the invoice period there is no need to create an invoice
	if emailsSent <= 0 {
		return nil
	}

	// 1 € / 1000 emails = 1 cents / 10 emails
	usageBasedAmountToPayInCents := emailsSent / 10
	// don't invoice customers if they need to pay less than 1 €
	// this is not an official policy so it can change any time
	if usageBasedAmountToPayInCents < 100 {
		return nil
	}

	// Create an invoice item for the usage
	invoiceItemParams := &stripe.InvoiceItemParams{
		Customer: stripe.String(*organization.StripeCustomerID),
		Amount:   stripe.Int64(usageBasedAmountToPayInCents),
		// Quantity:    stripe.Int64(billedEmailsSent),
		Currency:    stripe.String(string(stripe.CurrencyEUR)),
		Description: stripe.String(fmt.Sprintf("Emails: %d", emailsSent)),
		Period: &stripe.InvoiceItemPeriodParams{
			Start: stripe.Int64(invoicePeriodStart.Unix()),
			End:   stripe.Int64(invoicePeriodEnd.Unix()),
		},
		Metadata: map[string]string{
			"markdown_ninja_organization_id": organization.ID.String(),
			"markdown_ninja_emails_sent":     strconv.Itoa(int(emailsSent)),
		},
	}
	invoiceItemParams.SetIdempotencyKey("create_invoice_item-" + idempotencyKey)
	err = retry.Do(func() error {
		_, retryErr := invoiceitem.New(invoiceItemParams)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		err = fmt.Errorf("organizations.invoiceForUsageData: error creating monthly usage invoice item for organization [%s]: %w", organization.ID.String(), err)
		logger.Error(err.Error(), slog.String("organization_id", organization.ID.String()))
		return err
	}

	// Create and finalize the invoice
	var newStripeInvoice *stripe.Invoice
	newInvoiceParams := &stripe.InvoiceParams{
		Customer:                    stripe.String(*organization.StripeCustomerID),
		AutoAdvance:                 stripe.Bool(true),
		PendingInvoiceItemsBehavior: stripe.String("include"),
		// PendingInvoiceItemsBehavior: stripe.String("include"),
		Metadata: map[string]string{
			"markdown_ninja_organization_id": organization.ID.String(),
			"markdown_ninja_emails_sent":     strconv.Itoa(int(emailsSent)),
		},
	}
	newInvoiceParams.SetIdempotencyKey("create_invoice-" + idempotencyKey)
	err = retry.Do(func() (retryErr error) {
		newStripeInvoice, retryErr = invoice.New(newInvoiceParams)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		err = fmt.Errorf("organizations.invoiceForUsageData: error creating monthly usage invoice for organization [%s]: %w", organization.ID.String(), err)
		logger.Error(err.Error(), slog.String("organization_id", organization.ID.String()))
		return err
	}

	finalizeInvoiceParams := &stripe.InvoiceFinalizeInvoiceParams{}
	finalizeInvoiceParams.SetIdempotencyKey("finalize_invoice-" + idempotencyKey)
	err = retry.Do(func() error {
		_, retryErr := invoice.FinalizeInvoice(newStripeInvoice.ID, finalizeInvoiceParams)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		err = fmt.Errorf("organizations.invoiceForUsageData: error finalizing the invoice [%s]: %w", organization.ID.String(), err)
		logger.Error(err.Error(), slog.String("organization_id", organization.ID.String()))
		return err
	}

	if cancelingSubscription {
		var paymentMethodToCharge *stripe.PaymentMethod
		paymentMethodToCharge, err = service.getDefaultPaymentMethodForStripeCustomer(ctx, *organization.StripeCustomerID)
		if err != nil || paymentMethodToCharge == nil {
			err = errs.InvalidArgument("Please make sure that a valid payment method is attached to your account before canceling your subscription")
			return
		}

		// if the user is canceling their subscription, then we immediately pay the invoice
		payInvoiceParams := &stripe.InvoicePayParams{
			PaymentMethod: stripe.String(paymentMethodToCharge.ID),
		}
		payInvoiceParams.SetIdempotencyKey("pay_invoice-" + idempotencyKey)
		err = retry.Do(func() error {
			_, retryErr := invoice.Pay(newStripeInvoice.ID, payInvoiceParams)
			return retryErr
		}, retry.Context(ctx), retry.Attempts(15), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
		if err != nil {
			err = fmt.Errorf("organizations.invoiceForUsageData: error paying the invoice [%s]: %w", organization.ID.String(), err)
			logger.Error(err.Error(), slog.String("organization_id", organization.ID.String()))
			return err
		}
	}

	return nil
}
