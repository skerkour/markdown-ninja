package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/countries"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContactsService) UpdateContact(ctx context.Context, input contacts.UpdateContactInput) (contact contacts.Contact, err error) {
	currentUserID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	contact, err = service.repo.FindContactByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, currentUserID, contact.WebsiteID)
	if err != nil {
		return
	}

	err = service.UpdateContactInternal(ctx, service.db, &contact, input)
	if err != nil {
		return
	}

	return
}

func (service *ContactsService) UpdateContactInternal(ctx context.Context, db db.Queryer, contact *contacts.Contact, input contacts.UpdateContactInput) (err error) {
	logger := slogx.FromCtx(ctx)

	if contact == nil {
		errs.InvalidArgument("Contact is null")
		logger.Error("contacts.UpdateContactInternal: contact is null")
		return
	}

	updateStripeContact := false
	now := time.Now().UTC()

	if input.SubscribedToNewsletter != nil {
		if *input.SubscribedToNewsletter == false && contact.SubscribedToNewsletterAt != nil {
			contact.SubscribedToNewsletterAt = nil
		} else if *input.SubscribedToNewsletter && contact.SubscribedToNewsletterAt == nil {
			contact.SubscribedToNewsletterAt = &now
		}
	}

	if input.Verified != nil {
		contact.Verified = *input.Verified
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name != contact.Name {
			err = service.ValidateContactName(name)
			if err != nil {
				return err
			}
			contact.Name = name
			updateStripeContact = true
		}
	}

	if input.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*input.Email))
		if email != contact.Email {
			err = service.kernel.ValidateEmail(ctx, email, false)
			if err != nil {
				return err
			}
			contact.Email = email
			updateStripeContact = true
		}
	}

	if input.Country != nil {
		countryCode := strings.TrimSpace(*input.Country)
		if contact.Country != countryCode {
			_, err = countries.Name(countryCode)
			if err != nil {
				return countries.ErrCountryNotFound
			}
			contact.Country = countryCode
		}
	}

	if input.SignupCodeHash != nil {
		contact.SignupCodeHash = *input.SignupCodeHash
	}

	if input.FailedSignupAttempts != nil {
		contact.FailedSignupAttempts = *input.FailedSignupAttempts
	}

	if input.StripeCustomerID != nil {
		contact.StripeCustomerID = input.StripeCustomerID
	}

	contact.UpdatedAt = now
	err = service.repo.UpdateContact(ctx, db, *contact)
	if err != nil {
		return err
	}

	if updateStripeContact && contact.StripeCustomerID != nil {
		job := queue.NewJobInput{
			Data: contacts.JobUpdateStripeContact{
				ContactID: contact.ID,
			},
		}
		pushJobErr := service.queue.Push(ctx, nil, job)
		if pushJobErr != nil {
			logger.Error("contacts.UpdateContactInternal: error pushing job to queue",
				slogx.Err(pushJobErr))
		}
	}

	return nil
}
