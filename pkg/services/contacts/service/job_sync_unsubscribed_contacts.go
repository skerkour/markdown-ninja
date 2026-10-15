package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
)

// JobSyncUnsubscribedContacts fetches contacts that have been "suppressed" (bounces, spam complaints...) by
// the email provider
// if an email address has been "suppressed", it will unsubscribe the contact for ALL the websites
func (service *ContactsService) JobSyncUnsubscribedContacts(ctx context.Context, data contacts.JobSyncUnsubscribedContacts) (err error) {
	logger := slogx.FromCtx(ctx)

	suppressions, err := service.mailer.GetSuppressions(ctx)
	if err != nil {
		return err
	}

	for _, suppression := range suppressions {
		err = service.handleEmailSuppression(ctx, suppression.Email)
		if err != nil {
			logger.Error("contacts.JobSyncUnsubscribedContacts: error handling email suppression", slogx.Err(err),
				slog.String("email", suppression.Email),
			)
			// we don't return an error because the suppressions that we have failed to process will be
			// handled during next batch
			continue
		}
	}

	return nil
}

func (service *ContactsService) handleEmailSuppression(ctx context.Context, email string) (err error) {
	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		contacts, txErr := service.repo.FindContactsByEmail(ctx, tx, email, true)
		if txErr != nil {
			return txErr
		}

		for _, contact := range contacts {
			if contact.SubscribedToNewsletterAt != nil {
				contact.SubscribedToNewsletterAt = nil
				contact.UpdatedAt = now
				txErr = service.repo.UpdateContact(ctx, tx, contact)
				if txErr != nil {
					return fmt.Errorf("error updating contact: %w", txErr)
				}
			}
		}

		return nil
	})
	if err != nil {
		return
	}

	err = service.mailer.DeleteSuppression(ctx, email)
	if err != nil {
		return fmt.Errorf("error deleting suppression: %w", err)
	}

	return nil
}
