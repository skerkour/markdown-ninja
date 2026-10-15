package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/db"
)

func (service *OrganizationsService) AcceptStaffInvitation(ctx context.Context, input organizations.AcceptStaffInvitationInput) (err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	invitation, err := service.repo.FindStaffInvitationByID(ctx, service.db, input.ID, true)
	if err != nil {
		return
	}

	if !crypto.ConstantTimeCompare([]byte(invitation.InviteeEmail), []byte(httpCtx.AccessToken.Email)) {
		err = organizations.ErrStaffInvitationNotFound
		return
	}

	// verify that user is not already staff...
	_, err = service.CheckUserIsStaff(ctx, service.db, actorID, invitation.OrganizationID)
	if err == nil {
		// if the user is already staff then we simply delete the invitation
		err = service.repo.DeleteStaffInvitation(ctx, service.db, invitation.ID)
		if err != nil {
			return
		}
		return nil
	}
	err = nil

	now := time.Now().UTC()
	staff := organizations.Staff{
		CreatedAt:      now,
		UpdatedAt:      now,
		Role:           organizations.StaffRoleAdministrator,
		UserID:         actorID,
		OrganizationID: invitation.OrganizationID,
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.DeleteStaffInvitation(ctx, tx, invitation.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.repo.CreateStaff(ctx, tx, staff)
		if txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return
	}

	return
}
