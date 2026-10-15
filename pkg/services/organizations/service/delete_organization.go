package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
)

func (service *OrganizationsService) DeleteOrganization(ctx context.Context, input organizations.DeleteOrganizationInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	staff, err := service.repo.FindStaff(ctx, service.db, actorID, input.ID)
	if err != nil {
		return
	}

	if staff.Role != organizations.StaffRoleAdministrator {
		err = kernel.ErrPermissionDenied
		return
	}

	organization, err := service.repo.FindOrganizationByID(ctx, service.db, input.ID, false)
	if err != nil {
		return
	}

	websites, err := service.websitesService.FindWebsitesForOrganization(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	if len(websites) != 0 {
		err = organizations.ErrDeleteWebsitesToDeleteOrganization
		return
	}

	if organization.Plan != kernel.PlanFree.ID {
		return errs.InvalidArgument("Please cancel your subscription before deleting your organization")
	}

	if organization.PaymentDueSince != nil {
		return errs.InvalidArgument("Please pay all your due invoices before deleting your organization")
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.eventsService.ScheduleDeletionOfOrganizationData(ctx, tx, input.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.repo.DeleteOrganization(ctx, service.db, input.ID)
		if txErr != nil {
			return txErr
		}

		return nil
	})

	return err
}
