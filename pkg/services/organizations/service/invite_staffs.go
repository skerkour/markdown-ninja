package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/iterx"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/slicesx"
)

func (service *OrganizationsService) InviteStaffs(ctx context.Context, input organizations.InviteStaffsInput) (invitations []organizations.StaffInvitation, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	logger := slogx.FromCtx(ctx)

	staff, err := service.CheckUserIsStaff(ctx, service.db, actorID, input.OrganizationID)
	if err != nil {
		return
	}

	if staff.Role != organizations.StaffRoleAdministrator {
		err = kernel.ErrPermissionDenied
		return
	}

	emails := slices.Collect(iterx.Map(slices.Values(slicesx.Unique(input.Emails)), func(email string) string {
		return strings.TrimSpace(email)
	}))

	existingStaffs, err := service.getStaffsWithDetails(ctx, service.db, input.OrganizationID)
	if err != nil {
		return
	}
	existingStaffsByEmail := make(map[string]organizations.Staff, len(existingStaffs))
	for _, staff := range existingStaffs {
		existingStaffsByEmail[staff.Email] = staff.Staff
	}

	existingInvitations, err := service.repo.FindStaffInvitationsForOrganization(ctx, service.db, input.OrganizationID)
	if err != nil {
		return
	}
	existingInvitationsByEmail := make(map[string]organizations.StaffInvitation, len(existingInvitations))
	for _, invitation := range existingInvitations {
		existingInvitationsByEmail[invitation.InviteeEmail] = invitation
	}

	err = service.CheckBillingGatedAction(ctx, service.db, input.OrganizationID, organizations.BillingGatedActionInviteStaffs{NewStaffs: int64(len(emails))})
	if err != nil {
		return
	}

	validateEmailsRes, err := service.pingoo.LookupEmails(ctx, pingoo.LookupEmailsInput{
		Emails:    emails,
		MxRecords: new(true),
	})
	if err != nil {
		err = fmt.Errorf("organizations.InviteStaffs: error looking up emails: %w", err)
		return
	}

	for _, emailInfo := range validateEmailsRes {
		if !emailInfo.Valid ||
			emailInfo.Disposable ||
			len(emailInfo.MxRecords) == 0 {
			err = errs.InvalidArgument(fmt.Sprintf("%s is not a valid email address", emailInfo.Email))
			return
		}
	}

	now := time.Now().UTC()

	invitations = make([]organizations.StaffInvitation, 0, len(emails))
	invitationIDs := make([]guid.GUID, 0, len(emails))
	for _, email := range emails {
		// if staff / invitation already exist then we don't create / send new invitations
		_, staffExists := existingStaffsByEmail[email]
		_, invitationExists := existingInvitationsByEmail[email]

		if staffExists || invitationExists {
			continue
		}

		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}

		invitationID := guid.NewTimeBased()
		invitationIDs = append(invitationIDs, invitationID)
		invitation := organizations.StaffInvitation{
			ID:             invitationID,
			CreatedAt:      now,
			UpdatedAt:      now,
			Role:           organizations.StaffRoleAdministrator,
			InviteeEmail:   email,
			OrganizationID: input.OrganizationID,
			InviterID:      actorID,
		}
		invitations = append(invitations, invitation)
	}

	err = service.repo.CreateStaffInvitations(ctx, service.db, invitations)
	if err != nil {
		return
	}

	err = service.queue.Push(ctx, nil, queue.NewJobInput{
		Data: organizations.JobSendStaffInvitations{
			InvitationIDs: invitationIDs,
		},
	})
	if err != nil {
		logger.Error("organizations.InviteStaffs: pushing job to queue", slogx.Err(err))
		err = nil
	}

	return
}
