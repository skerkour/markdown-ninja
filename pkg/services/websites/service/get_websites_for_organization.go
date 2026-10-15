package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) GetWebsitesForOrganization(ctx context.Context, input websites.GetWebsitesForOrganizationInput) (sites []websites.Website, err error) {
	sites = make([]websites.Website, 0)
	httpCtx := httpctx.FromCtx(ctx)
	var organizationID guid.GUID

	if httpCtx.ApiKey != nil {
		organizationID = httpCtx.ApiKey.OrganizationID
	} else {
		actorID, err := service.kernel.CurrentUserID(ctx)
		if err != nil {
			return sites, err
		}

		if input.OrganizationID == nil {
			return sites, errs.InvalidArgument("organization_id is missing")
		}
		organizationID = *input.OrganizationID

		if !httpCtx.AccessToken.IsAdmin {
			_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, organizationID)
			if err != nil {
				return sites, err
			}
		}
	}

	sites, err = service.repo.FindWebsitesForOrganization(ctx, service.db, organizationID)
	return
}
