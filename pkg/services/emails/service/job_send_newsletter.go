package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/mail"
	"slices"
	"strings"
	"time"

	"log/slog"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/markdown"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/emails/templates"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

type newsletterRecipient struct {
	Name            string
	Email           string
	ContactID       *guid.GUID
	UnsubscribeLink string
}

func (service *EmailsService) JobSendNewsletter(ctx context.Context, input emails.JobSendNewsletter) error {
	logger := slogx.FromCtx(ctx).With(slog.String("newsletter.id", input.NewsletterID.String()))
	newsletter, err := service.repo.FindNewsletterByID(ctx, service.db, input.NewsletterID)
	if err != nil {
		if !errs.IsNotFound(err) {
			return err
		}

		// newsletter has been deleted...
		logger.Warn("emails.JobSendNewsletter: Newsletter not found")
		return nil
	}

	if !input.Test && newsletter.SentAt == nil {
		// edge case where the newsletter has been successfully pushed to queue
		// but not updated due to a database failure
		now := time.Now().UTC()
		newsletter.SentAt = &input.SentAt
		// newsletter.ScheduledFor = nil // TODO: or &now?
		newsletter.UpdatedAt = now
		err = service.repo.UpdateNewsletter(ctx, service.db, newsletter)
		if err != nil {
			return err
		}
	}

	emailConfig, err := service.repo.FindWebsiteConfiguration(ctx, service.db, newsletter.WebsiteID)
	if err != nil {
		return err
	}

	website, err := service.websitesService.FindWebsiteByID(ctx, service.db, newsletter.WebsiteID)
	if err != nil {
		return err
	}

	// TODO: improve error
	if !emailConfig.DomainVerified {
		return errors.New("emails.JobSendNewsletter: No custom domain configured")
	}

	var recipients []newsletterRecipient
	from := mail.Address{
		Name:    emailConfig.FromName,
		Address: emailConfig.FromAddress,
	}

	if input.Test {
		recipients = make([]newsletterRecipient, len(input.TestEmails))
		for i, testEmail := range input.TestEmails {
			recipients[i] = newsletterRecipient{
				Name:            testEmail,
				Email:           testEmail,
				ContactID:       nil,
				UnsubscribeLink: "https://placeholder_unsubscribe_link", // TODO: dummy link?
			}
		}
	} else {
		var recipientsContacts []contacts.Contact
		recipientsContacts, err = service.contactsService.FindVerifiedAndSubscribedToNewsletterContacts(ctx, service.db, website.ID)
		if err != nil {
			return err
		}

		recipients = make([]newsletterRecipient, len(recipientsContacts))
		for i, contact := range recipientsContacts {
			unsubscribeLink, unsubscribeLinkErr := service.contactsService.GenerateUnsubscribeLink(website.PrimaryDomain, contact.ID)
			if unsubscribeLinkErr != nil {
				logger.Error("email.JobSendNewsletter: error generating unsubscribeLink", slogx.Err(err),
					slog.String("contact.id", contact.ID.String()))
				continue
			}
			recipients[i] = newsletterRecipient{
				Name:            contact.Name,
				Email:           contact.Email,
				ContactID:       &contact.ID,
				UnsubscribeLink: unsubscribeLink,
			}
		}
	}

	contentMarkdown := newsletter.BodyMarkdown
	contentHtml, err := markdown.ToHtmlEmail(
		service.httpConfig.WebsitesBaseUrl.Scheme+"://"+website.PrimaryDomain+service.httpConfig.WebsitesPort,
		contentMarkdown,
	)
	if err != nil {
		return fmt.Errorf("emails.JobSendNewsletter: error converting markdown to HTML: %w", err)
	}

	// TODO: do we really want to render all the snippets?
	if strings.Contains(contentHtml, "{{<") {
		var snippets []content.Snippet
		snippets, err = service.contentService.FindSnippets(ctx, service.db, newsletter.WebsiteID)
		if err != nil {
			return fmt.Errorf("emails.JobSendNewsletter: Finding snippets: %w", err)
		}
		if len(snippets) != 0 {
			contentHtml = service.contentService.RenderSnippets(contentHtml, snippets, true)
			// contentMarkdown = service.contentService.RenderSnippets(contentMarkdown, snippets, true)
		}
	}

	tx, err := service.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("emails.JobSendNewsletter: Starting DB transaction: %w", err)
	}
	defer tx.Rollback()

	scheduledFor := time.Now().UTC()

	// emails are pushed to the queue by batches of 256
	// we do that to limit the amount of RAM used both by the app servers, and by the database
	for recipientsChunk := range slices.Chunk(recipients, 256) {
		jobs := make([]queue.NewJobInput, 0, len(recipientsChunk))

		// for each recipient we generate an email
		for i, recipient := range recipientsChunk {
			emailBodyBuffer := bytes.NewBuffer(make([]byte, 0, len(contentHtml)))

			subject := newsletter.Subject
			if input.Test {
				subject = "[Test] " + subject
			}

			emailData := templates.NewsletterEmailData{
				Subject:         newsletter.Subject,
				Content:         template.HTML(contentHtml),
				UnsubscribeLink: template.URL(recipient.UnsubscribeLink),
			}
			err = service.newsletterEmailTemplate.Execute(emailBodyBuffer, emailData)
			if err != nil {
				logger.Error("emails.JobSendNewsletter: error executing email template", slogx.Err(err))
				err = nil
				continue
			}

			// bodyText := contentMarkdown + "\n\n\n" + "Unsubscribe: " + recipient.UnsubscribeLink

			// don't send all the emails at the same time
			if i != 0 && (i%emails.NewsletterRateLimit == 0) {
				scheduledFor = scheduledFor.Add(time.Second)
			}

			sendEmailJob := queue.NewJobInput{
				ScheduledFor: &scheduledFor,
				Data: emails.JobSendEmail{
					Type:        emails.EmailTypeBroadcast,
					FromAddress: from.Address,
					FromName:    from.Name,
					ToAddress:   recipient.Email,
					ToName:      recipient.Name,
					Subject:     subject,
					BodyHtml:    emailBodyBuffer.String(),
					// BodyText:    &bodyText,
					Headers: map[string][]string{
						// See here for more information: https://mailtrap.io/blog/list-unsubscribe-header
						// https://sendgrid.com/blog/list-unsubscribe
						// https://www.gmass.co/blog/list-unsubscribe-header/
						// TODO: mailto:
						"List-Unsubscribe": {"<" + recipient.UnsubscribeLink + ">"},
					},
					WebsiteID:      &website.ID,
					ContactID:      recipient.ContactID,
					NewsletterID:   &newsletter.ID,
					OrganizationID: nil,
				},
			}
			jobs = append(jobs, sendEmailJob)
		}

		err = service.queue.PushMany(ctx, tx, jobs)
		if err != nil {
			return fmt.Errorf("emails.JobSendNewsletter: pushing JobSendEmail jobs to queue: %w", err)
		}
	}

	// we report data usage 1 minute after all the emails have been sent
	sendUsageDataJob := queue.NewJobInput{
		ScheduledFor: new(scheduledFor.Add(time.Minute)),
		Data: organizations.JobSendUsageData{
			OrganizationID: website.OrganizationID,
		},
	}
	err = service.queue.Push(ctx, tx, sendUsageDataJob)
	if err != nil {
		return fmt.Errorf("emails.JobSendNewsletter: pushing JobSendUsageData to queue: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("emails.JobSendNewsletter: Comitting DB transaction: %w", err)
	}

	return nil
}
