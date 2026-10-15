package service

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/mail"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/store/notifications"
	"markdown.ninja/stdx/email"
	"markdown.ninja/stdx/log/slogx"
)

func (service *StoreService) JobSendOrderConfirmationEmail(ctx context.Context, input store.JobSendOrderConfirmationEmail) (err error) {
	logger := slogx.FromCtx(ctx)
	var htmlContent bytes.Buffer

	order, err := service.repo.FindOrderByID(ctx, service.db, input.OrderID, false)
	if err != nil {
		return
	}

	emailConfig, err := service.emailsService.FindWebsiteConfiguration(ctx, service.db, order.WebsiteID)
	if err != nil {
		return
	}

	website, err := service.websitesService.FindWebsiteByID(ctx, service.db, order.WebsiteID)
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
		from = service.emailsService.GetDefaultFromAddressForWebsite(website)
	}

	contact, err := service.contactsService.FindContact(ctx, service.db, order.ContactID)
	if err != nil {
		return
	}

	to := mail.Address{
		Name:    contact.Name,
		Address: contact.Email,
	}
	subject := fmt.Sprintf("Order #%s confirmed", order.ID.String())
	hostname := website.PrimaryDomain + service.httpConfig.WebsitesPort
	accountUrl := fmt.Sprintf("%s://%s%s/account", service.httpConfig.WebsitesBaseUrl.Scheme, hostname, service.websitesPort)
	emailData := notifications.OrderConfirmationEmailData{
		AccountURL: template.URL(accountUrl),
		OrderID:    order.ID.String(),
	}
	err = service.orderConfirmationEmailTemplate.Execute(&htmlContent, emailData)
	if err != nil {
		errMessage := "store.JobSendOrderConfirmationEmail: Executing email template"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	message := email.Email{
		From:    from,
		To:      []mail.Address{to},
		Subject: subject,
		HTML:    htmlContent.Bytes(),
		Text:    []byte(subject),
	}
	err = service.mailer.SendTransactionnal(ctx, message)
	if err != nil {
		errMessage := "store.JobSendOrderConfirmationEmail: Sending email"
		logger.Error(errMessage, slogx.Err(err), slog.String("email", to.String()))
		err = errs.Internal(errMessage, err)
	}

	trackEventInput := events.TrackEmailSentInput{
		FromAddress: from.Address,
		ToAddress:   to.Address,
		WebsiteID:   website.ID,
	}
	service.eventsService.TrackEmailSent(ctx, trackEventInput)

	return
}
