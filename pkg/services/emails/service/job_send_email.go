package service

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/email"
)

func (service *EmailsService) JobSendEmail(ctx context.Context, input emails.JobSendEmail) error {
	var err error

	var bodyText []byte
	if input.BodyText != nil {
		bodyText = []byte(*input.BodyText)
	}

	var from mail.Address
	if input.Type == emails.EmailTypeTransactional {
		from = service.config.Emails.NotifyAddress
	} else {
		from = mail.Address{
			Name:    input.FromName,
			Address: input.FromAddress,
		}
	}

	to := mail.Address{
		Name:    input.ToName,
		Address: input.ToAddress,
	}
	message := email.Email{
		From:    from,
		To:      []mail.Address{to},
		Subject: input.Subject,
		HTML:    []byte(input.BodyHtml),
		Text:    bodyText,
		Headers: input.Headers,
	}

	// we don't use retry here and prefer to instead rely on the retry mechanism of the queue because we don't
	// want to send the message multiple times if there is a problem with retry.
	switch input.Type {
	case emails.EmailTypeBroadcast:
		err = service.mailer.SendBroadcast(ctx, message)
	case emails.EmailTypeTransactional:
		err = service.mailer.SendTransactionnal(ctx, message)
	default:
		err = fmt.Errorf("emails.JobSendEmail: unknown email type: %s", input.Type)
	}
	// if the email provider returns an error that the contact have been "suppressed" (marked as spam, complaint...)
	if err != nil && strings.Contains(err.Error(), "that have been marked as inactive") {
		return nil
	} else if err != nil {
		return fmt.Errorf("emails.JobSendEmail: sending email: %w", err)
	}

	if input.WebsiteID != nil {
		service.eventsService.TrackEmailSent(ctx, events.TrackEmailSentInput{
			FromAddress:  from.Address,
			ToAddress:    to.Address,
			WebsiteID:    *input.WebsiteID,
			NewsletterID: input.NewsletterID,
		})
	}

	return nil
}

// func (service *EmailsService) getWebsiteSenderApiToken(ctx context.Context, db db.Queryer, websiteID guid.GUID) (string, error) {
// 	senderApiTokenCacheKey := getSenderApiTokenCacheKey(websiteID)
// 	senderApiTokenCacheRes := service.sendEmailCache.Get(senderApiTokenCacheKey)
// 	if senderApiTokenCacheRes != nil {
// 		return senderApiTokenCacheRes.Value().(string), nil
// 	}

// 	// when websiteID is not null it may be a newsletter email.
// 	// This is problematic because it's highly inneficient and will bring the database to its knees
// 	// due to the many database requests that will be issued at the same time, the thundering herd problem.
// 	// To mitigate that, we currently limit the number of concurrent databases queries to 1 with
// 	// singleflight and then cache the result.
// 	// TODO: how to improve?

// 	var emailConfig emails.WebsiteConfiguration
// 	emailConfigCacheKey := getWebsiteConfigurationCacheKey(websiteID)
// 	emailConfigCacheRes := service.sendEmailCache.Get(emailConfigCacheKey)
// 	if emailConfigCacheRes != nil {
// 		emailConfig = emailConfigCacheRes.Value().(emails.WebsiteConfiguration)
// 	} else {
// 		// avoid thundering herd problem with singleflightGroup
// 		singleflightRes, err, _ := service.sendEmailSingleflightGroup.Do(fmt.Sprintf("getWebsiteSenderApiToken.FindWebsiteConfiguration-%s", websiteID.String()), func() (interface{}, error) {
// 			emailConfigSingleFlight, err := service.repo.FindWebsiteConfiguration(ctx, db, websiteID)
// 			if err != nil {
// 				return nil, err
// 			}

// 			service.sendEmailCache.Set(emailConfigCacheKey, emailConfigSingleFlight, time.Minute)
// 			return emailConfigSingleFlight, nil
// 		})
// 		if err != nil {
// 			return "", err
// 		}
// 		emailConfig = singleflightRes.(emails.WebsiteConfiguration)
// 	}

// 	// TODO: improve error
// 	if !emailConfig.DomainVerified {
// 		return "", fmt.Errorf("emails.getWebsiteSenderApiToken: No custom domain configured for website %s", websiteID.String())
// 	}

// 	// avoid thundering herd problem with singleflightGroup
// 	singleflightRes, err, _ := service.sendEmailSingleflightGroup.Do(fmt.Sprintf("getWebsiteSenderApiToken.GetSecret-%s", websiteID.String()), func() (interface{}, error) {
// 		var emailServerTokenSecret emails.PostmarkServerTokenSecret
// 		err := service.secretsService.GetSecret(ctx, db, emailConfig.PostmarkServerTokenSecretID,
// 			&emailServerTokenSecret, websiteID.Bytes())
// 		if err != nil {
// 			return "", err
// 		}

// 		service.sendEmailCache.Set(senderApiTokenCacheKey, emailServerTokenSecret.Token, time.Minute)
// 		return emailServerTokenSecret.Token, nil
// 	})
// 	if err != nil {
// 		return "", err
// 	}

// 	senderApiToken := singleflightRes.(string)
// 	return senderApiToken, nil
// }
