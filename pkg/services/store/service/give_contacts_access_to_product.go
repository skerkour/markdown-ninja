package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/countries"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/iterx"
	"markdown.ninja/stdx/slicesx"
)

func (service *StoreService) GiveContactsAccessToProduct(ctx context.Context, input store.GiveContactsAccessToProductInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	product, err := service.repo.FindProductByID(ctx, service.db, input.ProductID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, product.WebsiteID)
	if err != nil {
		return
	}

	emails := slices.Collect(iterx.Map(slices.Values(slicesx.Unique(input.Emails)), func(email string) string {
		return strings.TrimSpace(email)
	}))

	validateEmailsRes, err := service.pingoo.LookupEmails(ctx, pingoo.LookupEmailsInput{
		Emails: emails,
	})
	if err != nil {
		err = fmt.Errorf("store.GiveContactsAccessToProduct: error looking up emails: %w", err)
		return
	}

	for _, emailInfo := range validateEmailsRes {
		if !emailInfo.Valid {
			err = errs.InvalidArgument(fmt.Sprintf("%s is not a valid email address", emailInfo.Email))
			return
		}
	}

	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		for _, email := range emails {
			email = strings.ToLower(strings.TrimSpace(email))
			if email == "" {
				continue
			}

			txErr = service.kernel.ValidateEmail(ctx, email, false)
			if txErr != nil {
				return txErr
			}

			var contact contacts.Contact
			contact, txErr = service.contactsService.FindContactByEmail(ctx, tx, product.WebsiteID, email)
			if txErr != nil {
				if !errs.IsNotFound(txErr) {
					return txErr
				}

				// if contact is not found we create it
				createContactInput := contacts.CreateContactInternalInput{
					Email:     email,
					Name:      "",
					Verified:  true,
					Country:   countries.CodeUnknown,
					WebsiteID: product.WebsiteID,
				}
				contact, txErr = service.contactsService.CreateContactInternal(ctx, tx, createContactInput)
				if txErr != nil {
					return txErr
				}
			} else {
				// if contact is found but not verified, mark as verified
				if !contact.Verified {
					updateContactInput := contacts.UpdateContactInput{
						ID:       contact.ID,
						Verified: new(true),
					}
					txErr = service.contactsService.UpdateContactInternal(ctx, tx, &contact, updateContactInput)
					if txErr != nil {
						return txErr
					}
				}
			}

			_, txErr = service.repo.FindContactProductAccess(ctx, tx, contact.ID, product.ID)
			if txErr == nil {
				// if contact already has access to product we do nothing
				continue
			} else {
				if !errs.IsNotFound(txErr) {
					return txErr
				}
				txErr = nil
			}

			contactProductAccess := store.ContactProductAccess{
				CreatedAt: now,
				ContactID: contact.ID,
				ProductID: product.ID,
			}
			txErr = service.repo.CreateContactProductAccess(ctx, tx, contactProductAccess)
			if txErr != nil {
				return txErr
			}
		}

		return nil
	})
	if err != nil {
		return
	}

	return
}
