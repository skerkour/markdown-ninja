package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"math"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/log/slogx"
)

func (service *ContactsService) ExportContacts(ctx context.Context, input contacts.ExportContactsInput) (ret contacts.ExportContactsOutput, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}
	logger := slogx.FromCtx(ctx)

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return
	}

	contacts, err := service.repo.FindVerifiedContactsForWebsite(ctx, service.db, input.WebsiteID, "", math.MaxInt64)
	if err != nil {
		return
	}

	csvBuffer := bytes.NewBuffer(make([]byte, 0, len(contacts)*40))
	csvWriter := csv.NewWriter(csvBuffer)

	err = csvWriter.Write([]string{"email", "name", "subscribed_at"})
	if err != nil {
		errMessage := "contacts.ExportContacts: writing header to csvWriter"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	for _, contact := range contacts {
		var subscribedAtStr string
		if contact.SubscribedToNewsletterAt != nil {
			subscribedAtStr = contact.SubscribedToNewsletterAt.UTC().Format(time.RFC3339)
		}
		err = csvWriter.Write([]string{contact.Email, contact.Name, subscribedAtStr})
		if err != nil {
			errMessage := "contacts.ExportContacts: writing data to csvWriter"
			logger.Error(errMessage, slogx.Err(err))
			err = errs.Internal(errMessage, err)
			return
		}
	}

	csvWriter.Flush()
	err = csvWriter.Error()
	if err != nil {
		errMessage := "contacts.ExportContacts: flushing csvWriter"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	ret.Contacts = csvBuffer.String()

	return
}
