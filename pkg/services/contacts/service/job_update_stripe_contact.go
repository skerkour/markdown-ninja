package service

import (
	"context"
	"log/slog"

	"github.com/stripe/stripe-go/v81"
	stripecustomer "github.com/stripe/stripe-go/v81/customer"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/log/slogx"
)

func (service *ContactsService) JobUpdateStripeContact(ctx context.Context, data contacts.JobUpdateStripeContact) (err error) {
	logger := slogx.FromCtx(ctx)
	contact, err := service.repo.FindContactByID(ctx, service.db, data.ContactID)
	if err != nil {
		if errs.IsNotFound(err) {
			logger.Warn("contacts.JobUpdateStripeContact: contact not found", slog.String("contact.id", data.ContactID.String()))
		}
		return
	}

	if contact.StripeCustomerID != nil {
		params := &stripe.CustomerParams{
			Name:  stripe.String(contact.Name),
			Email: stripe.String(contact.Email),
			Address: &stripe.AddressParams{
				Country: stripe.String(contact.Country),
			},
		}
		_, err = stripecustomer.Update(
			*contact.StripeCustomerID,
			params,
		)
		if err != nil {
			errMessage := "contacts.JobUpdateStripeContact: Error updating stripe customer"
			logger.Error(errMessage, slogx.Err(err))
		}
	}

	return
}
