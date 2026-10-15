package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/guid"
)

func (service *OrganizationsService) GetOrganization(ctx context.Context, input organizations.GetOrganizationInput) (org organizations.Organization, err error) {
	httpCtx := httpctx.FromCtx(ctx)
	var organizationID guid.GUID

	if httpCtx.ApiKey != nil {
		organizationID = httpCtx.ApiKey.OrganizationID
	} else {
		actorID, err := service.kernel.CurrentUserID(ctx)
		if err != nil {
			return org, err
		}

		if input.ID == nil {
			return org, errs.InvalidArgument("id is missing")
		}
		organizationID = *input.ID

		if !httpCtx.AccessToken.IsAdmin {
			_, err = service.CheckUserIsStaff(ctx, service.db, actorID, organizationID)
			if err != nil {
				return org, err
			}
		}
	}

	org, err = service.repo.FindOrganizationByID(ctx, service.db, organizationID, false)
	if err != nil {
		return
	}

	if input.ApiKeys {
		org.ApiKeys, err = service.repo.FindApiKeysForOrganization(ctx, service.db, org.ID)
		if err != nil {
			return
		}
	}

	if input.Staffs {
		org.Staffs, err = service.getStaffsWithDetails(ctx, service.db, org.ID)
		if err != nil {
			return
		}
	}

	if httpCtx.AccessToken.IsAdmin {
		org.OrganizationAdminFields = &organizations.OrganizationAdminFields{
			StripeCustomerID:     org.StripeCustomerID,
			StripeSubscriptionID: org.StripeSubscriptionID,
		}
	}

	return org, nil
}
