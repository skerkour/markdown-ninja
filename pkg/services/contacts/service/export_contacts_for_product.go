package service

import (
	"context"
	"strings"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/log/slogx"
)

func (service *ContactsService) ExportContactsForProduct(ctx context.Context, input contacts.ExportContactsForProductInput) (res contacts.ExportContactsForProductOutput, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}
	logger := slogx.FromCtx(ctx)

	product, err := service.storeService.FindProduct(ctx, service.db, input.ProductID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, product.WebsiteID)
	if err != nil {
		return
	}

	contacts, err := service.repo.FindContactsWithAccessToProduct(ctx, service.db, product.ID)
	if err != nil {
		return
	}

	var output strings.Builder
	output.Grow(len(contacts) * 30)

	for _, contact := range contacts {
		_, err = output.WriteString(contact.Email)
		if err != nil {
			errMessage := "contacts.ExportContactsForProduct: writing email to writer"
			logger.Error(errMessage, slogx.Err(err))
			err = errs.Internal(errMessage, err)
			return
		}

		_, err = output.WriteRune('\n')
		if err != nil {
			errMessage := "contacts.ExportContactsForProduct: writing newline to writer"
			logger.Error(errMessage, slogx.Err(err))
			err = errs.Internal(errMessage, err)
			return
		}
	}

	res.Contacts = output.String()

	return
}
