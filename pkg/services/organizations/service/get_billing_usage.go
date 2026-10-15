package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
)

func (service *OrganizationsService) GetBillingUsage(ctx context.Context, input organizations.GetBillingUsageInput) (usage organizations.BillingUsage, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	// if current user is not a Markdown Ninja admin then it needs to be an organization member
	if !httpCtx.AccessToken.IsAdmin {
		_, err = service.repo.FindStaff(ctx, service.db, actorID, input.OrganizationID)
		if err != nil {
			return
		}
	}

	organization, err := service.repo.FindOrganizationByID(ctx, service.db, input.OrganizationID, false)
	if err != nil {
		return
	}

	return service.getOrganizationBillingUsage(ctx, service.db, organization)
}

func (service *OrganizationsService) getOrganizationBillingUsage(ctx context.Context, db db.Queryer, organization organizations.Organization) (usage organizations.BillingUsage, err error) {
	usage.UsedStaffs, err = service.repo.GetStaffsCountForOrganization(ctx, db, organization.ID)
	if err != nil {
		return
	}

	usage.UsedWebsites, err = service.websitesService.GetWebsitesCountForOrganization(ctx, db, organization.ID)
	if err != nil {
		return
	}

	usage.UsedStorage, err = service.contentService.GetUsedStorageForOrganization(ctx, db, organization.ID)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	emailsFrom := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	emailsTo := now
	if organization.SubscriptionStartedAt != nil {
		emailsFrom = *organization.SubscriptionStartedAt
	}
	usage.UsedEmails, err = service.eventsService.GetEmailsSentCountForOrganization(ctx, db, organization.ID, emailsFrom, emailsTo)
	if err != nil {
		return
	}

	switch organization.Plan {
	case kernel.PlanFree.ID:
		usage.AllowedStorage = kernel.PlanFree.AllowedStorage
	case kernel.PlanPro.ID:
		usage.AllowedEmails = kernel.PlanPro.AllowedEmails
		usage.AllowedStorage += kernel.PlanPro.AllowedStorage + (organization.ExtraSlots * kernel.StoragePerSlot)
	case kernel.PlanEnterprise.ID:
		usage.AllowedEmails = kernel.PlanEnterprise.AllowedEmails
		usage.AllowedStorage += kernel.PlanEnterprise.AllowedStorage + (organization.ExtraSlots * kernel.StoragePerSlot)
	}

	usage.AllowedWebsites += 1 + organization.ExtraSlots
	usage.AllowedStaffs += 1 + organization.ExtraSlots

	return
}
