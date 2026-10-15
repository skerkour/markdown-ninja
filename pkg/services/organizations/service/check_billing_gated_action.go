package service

import (
	"context"
	"fmt"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *OrganizationsService) CheckBillingGatedAction(ctx context.Context, db db.Queryer, organizationID guid.GUID, action organizations.BillingGatedAction) (err error) {
	// Actions don't need to be checked if self-hosted
	if service.isSelfHosted {
		return nil
	}

	organization, err := service.repo.FindOrganizationByID(ctx, db, organizationID, false)
	if err != nil {
		return err
	}

	plan := kernel.AllPlans[organization.Plan]

	switch actionData := action.(type) {
	case organizations.BillingGatedActionCreateWebsite:
		var websitesCount int64
		websitesCount, err = service.websitesService.GetWebsitesCountForOrganization(ctx, db, organization.ID)
		if err != nil {
			return err
		}

		websitesLimit := 1 + organization.ExtraSlots
		if websitesCount >= websitesLimit {
			return errs.InvalidArgument(fmt.Sprintf("Websites limit reached. Please upgrade your plan to create more websites. Current limit: %d", websitesLimit))
		}

	case organizations.BillingGatedActionUpdateWebsite:
		if actionData.PoweredBy == false && plan.ID == kernel.PlanFree.ID {
			return errs.InvalidArgument("Please upgrade to a paid plan to remove \"powered by Markdown Ninja\"")
		}

	case organizations.BillingGatedActionInviteStaffs:
		var staffsCount int64
		var existingInvitationsCount int64
		staffsCount, err = service.repo.GetStaffsCountForOrganization(ctx, db, organizationID)
		if err != nil {
			return err
		}

		existingInvitationsCount, err = service.repo.GetStaffInvitationsCountForOrganization(ctx, db, organizationID)
		if err != nil {
			return err
		}

		staffsLimit := 1 + organization.ExtraSlots
		if actionData.NewStaffs+staffsCount+existingInvitationsCount > staffsLimit {
			return errs.InvalidArgument(fmt.Sprintf("Staffs limit reached. Please upgrade your plan to invite more staffs. Current limit: %d", staffsLimit))
		}

	case organizations.BillingGatedActionSendNewsletter:
		if plan.ID == kernel.PlanFree.ID {
			err = errs.InvalidArgument("Newsletters can't be sent on the free plan to prevent abuse. Please upgrade your plan to send your newsletter.")
			return
		}

	case organizations.BillingGatedActionAddWebsiteCustomDomain:
		if plan.ID == kernel.PlanFree.ID {
			return errs.InvalidArgument("Custom domains can't be added on the free plan to prevent abuse. Please upgrade your plan to add a custom domain.")
		}

		var domainsCount int64
		domainsCount, err = service.websitesService.GetDomainsCountForWebsite(ctx, db, actionData.WebsiteID)
		if err != nil {
			return err
		}

		if domainsCount >= plan.CustomDomainsPerWebsite {
			return errs.InvalidArgument(fmt.Sprintf("Custom domains limit reached. Please upgrade your plan to add more custom domains to your websites. Current limit: %d", plan.CustomDomainsPerWebsite))
		}

	case organizations.BillingGatedActionSetupCustomEmailDomain:
		if plan.ID == kernel.PlanFree.ID {
			return errs.InvalidArgument("Custom domains can't be added on the free plan to prevent abuse. Please upgrade your plan to add a custom domain.")
		}

	case organizations.BillingGatedActionUploadAsset:
		var usedStorageBytes int64
		var assetsCount int64

		if plan.ID == kernel.PlanFree.ID {
			if actionData.AssetType == content.AssetTypeVideo {
				return errs.InvalidArgument("To prevent abuse, videos can't be uploaded on the free plan.")
			}
			return errs.InvalidArgument("To prevent abuse, a paid plan is required to upload assets.")
		}

		if actionData.NewAssetSize > plan.MaxAssetSize {
			return errs.InvalidArgument(fmt.Sprintf("Asset is too large. Please upgrade your plan or contact support to uplaod larger assets. Current limit: %d bytes", plan.MaxAssetSize))
		}

		assetsCount, err = service.contentService.GetAssetsCountForWebsite(ctx, db, actionData.WebsiteID)
		if err != nil {
			return err
		}
		if assetsCount >= plan.AllowedAssets {
			return errs.InvalidArgument("Assets limit reached to prevent abuse. Please upgrade your plan or contact support if you need more.")
		}

		usedStorageBytes, err = service.contentService.GetUsedStorageForOrganization(ctx, db, organizationID)
		if err != nil {
			return err
		}

		storageLimit := plan.AllowedStorage + (organization.ExtraSlots * kernel.StoragePerSlot)
		if (usedStorageBytes + actionData.NewAssetSize) > storageLimit {
			return errs.InvalidArgument(fmt.Sprintf("Storage limit reached. Please upgrade your plan to upload more assets. Current limit: %d GB", storageLimit/1_000_000_000))
		}

	case organizations.BillingGatedActionCreatePage:
		var pagesCount int64
		pagesCount, err = service.contentService.GetPagesCountForWebsite(ctx, db, actionData.WebsiteID)
		if err != nil {
			return err
		}

		if pagesCount >= plan.AllowedPages {
			return errs.InvalidArgument(fmt.Sprintf("Pages limit reached. Please upgrade your plan or contact support if you need more. Current limit: %d", plan.AllowedPages))
		}

	default:
		return fmt.Errorf("organizations.CheckBillingGatedAction: Unknown type: %T", action)
	}

	return nil
}
