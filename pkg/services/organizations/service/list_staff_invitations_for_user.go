package service

import (
	"context"
	"slices"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/iterx"
	"markdown.ninja/stdx/set"
	"markdown.ninja/stdx/uuid"
)

func (service *OrganizationsService) ListUserInvitations(ctx context.Context, _ kernel.EmptyInput) (ret kernel.PaginatedResult[organizations.UserInvitation], err error) {
	httpCtx := httpctx.FromCtx(ctx)

	_, err = service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	invitations, err := service.repo.FindInvitationsForInviteeEmail(ctx, service.db, httpCtx.AccessToken.Email)
	if err != nil {
		return
	}

	uniqueInvitersIds := set.NewFromIter(iterx.Map(slices.Values(invitations), func(invit organizations.StaffInvitationWithOrganizationDetails) uuid.UUID {
		return invit.Invitation.InviterID
	}))
	inviters, err := service.pingoo.ListUsers(ctx, pingoo.ListUsersInput{IDs: uniqueInvitersIds.ToSlice()})
	if err != nil {
		return
	}

	invitersByID := make(map[uuid.UUID]pingoo.User, len(inviters.Data))
	for _, inviter := range inviters.Data {
		invitersByID[inviter.ID] = inviter
	}

	ret.Data = make([]organizations.UserInvitation, 0, len(invitations))
	for _, invitation := range invitations {
		if inviter, inviterOk := invitersByID[invitation.Invitation.InviterID]; inviterOk {
			userInvit := organizations.UserInvitation{
				Invitation:   invitation,
				InviterName:  inviter.Name,
				InviterEmail: inviter.Email,
			}
			ret.Data = append(ret.Data, userInvit)
		}
	}
	return
}
