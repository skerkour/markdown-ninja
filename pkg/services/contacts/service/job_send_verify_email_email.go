package service

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/mail"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/contacts/templates"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/email"
	"markdown.ninja/stdx/log/slogx"
)

func (service *ContactsService) JobSendVerifyEmailEmail(ctx context.Context, input contacts.JobSendVerifyEmailEmail) (err error) {
	logger := slogx.FromCtx(ctx)
	var htmlContent bytes.Buffer

	emailConfig, err := service.emailsService.FindWebsiteConfiguration(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}

	var from mail.Address
	if emailConfig.DomainVerified {
		from = mail.Address{
			Name:    emailConfig.FromName,
			Address: emailConfig.FromAddress,
		}
	} else {
		website, err := service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
		if err != nil {
			return err
		}
		from = service.emailsService.GetDefaultFromAddressForWebsite(website)
	}

	to := mail.Address{
		Name:    input.Name,
		Address: input.Email,
	}
	subject := "Confirm your email address"

	textContent := fmt.Sprintf("Use the following link to confirm your new email address: %s", input.VerifyEmailLink)

	emailData := templates.VerifyEmailEmailData{
		Link: template.URL(input.VerifyEmailLink),
	}
	err = service.verifyEmailEmailTemplate.Execute(&htmlContent, emailData)
	if err != nil {
		errMessage := "contacts.JobSendVerifyEmailEmail: Executing email template"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	message := email.Email{
		From:    from,
		To:      []mail.Address{to},
		Subject: subject,
		HTML:    htmlContent.Bytes(),
		Text:    []byte(textContent),
	}
	err = service.mailer.SendTransactionnal(ctx, message)
	if err != nil {
		errMessage := "contacts.JobSendVerifyEmailEmail: Sending email"
		logger.Error(errMessage, slogx.Err(err), slog.String("email", to.String()))
		err = errs.Internal(errMessage, err)
	}

	trackEventInput := events.TrackEmailSentInput{
		FromAddress: from.Address,
		ToAddress:   to.Address,
		WebsiteID:   input.WebsiteID,
	}
	service.eventsService.TrackEmailSent(ctx, trackEventInput)

	return
}
