package service

import (
	"context"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/crypto"
)

func (service *OrganizationsService) DeleteStaffInvitation(ctx context.Context, input organizations.DeleteStaffInvitationInput) (err error) {
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
		// if user is not the invitee then it must be administrator of the organization to delete the
		// invitation
		var actorStaff organizations.Staff
		actorStaff, err = service.CheckUserIsStaff(ctx, service.db, actorID, invitation.OrganizationID)
		if err != nil {
			return
		}

		if actorStaff.Role != organizations.StaffRoleAdministrator {
			return kernel.ErrPermissionDenied
		}
	}

	err = service.repo.DeleteStaffInvitation(ctx, service.db, invitation.ID)
	if err != nil {
		return
	}

	return nil
}
