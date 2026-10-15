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
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/site/templates"
	"markdown.ninja/stdx/email"
	"markdown.ninja/stdx/log/slogx"
)

func (service *SiteService) JobSendSubscribeEmail(ctx context.Context, input site.JobSendSubscribeEmail) (err error) {
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
	subject := fmt.Sprintf("Your confirmation code: %s", input.Code)
	subscribeLink := service.generateSubscribeLink(input.WebsiteDomain, input.ContactID, input.Code)
	textContent := fmt.Sprintf(`
%s

or use the following link: %s`, input.Code, subscribeLink)

	emailData := templates.SubscribeEmailData{
		Code: template.HTML(input.Code),
		Link: template.URL(subscribeLink),
	}
	err = service.subscribeEmailTemplate.Execute(&htmlContent, emailData)
	if err != nil {
		errMessage := "site.JobSendSubscribeEmail: Executing email template"
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
		errMessage := "site.JobSendSubscribeEmail: Sending email"
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
