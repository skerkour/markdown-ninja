package service

import (
	"bytes"
	"context"
	"fmt"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/organizations/templates"
	"markdown.ninja/stdx/queue"
)

func (service *OrganizationsService) JobSendStaffInvitations(ctx context.Context, input organizations.JobSendStaffInvitations) error {
	invitations, err := service.repo.FindInviteeInvitationsByIDs(ctx, service.db, input.InvitationIDs)
	if err != nil {
		return err
	}

	subject := "You have been invited to join an organization on markdown.ninja"
	jobs := make([]queue.NewJobInput, 0, len(invitations))

	for _, invitation := range invitations {
		inviter, err := service.pingoo.GetUser(ctx, pingoo.GetUserInput{ID: invitation.Invitation.InviterID})
		if err != nil {
			return err
		}

		var htmlContent bytes.Buffer
		templateData := templates.StaffInvitationEmailData{
			InviterEmail:     inviter.Email,
			OrganizationName: invitation.OrganizationName,
		}

		err = service.staffInvitationEmailTemplate.Execute(&htmlContent, templateData)
		if err != nil {
			return fmt.Errorf("executing email template: %w", err)
		}

		sendEmailJob := queue.NewJobInput{
			Data: emails.JobSendEmail{
				Type: emails.EmailTypeTransactional,
				// left empty because it's a transactional email
				FromAddress:    "",
				FromName:       "",
				ToAddress:      invitation.Invitation.InviteeEmail,
				ToName:         "",
				Subject:        subject,
				BodyHtml:       htmlContent.String(),
				BodyText:       nil,
				Headers:        nil,
				WebsiteID:      nil,
				ContactID:      nil,
				NewsletterID:   nil,
				OrganizationID: &invitation.OrganizationID,
			},
		}
		jobs = append(jobs, sendEmailJob)
	}

	err = service.queue.PushMany(ctx, nil, jobs)
	if err != nil {
		return fmt.Errorf("pushing jobs to queue: %w", err)
	}

	return nil
}
