package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

func (service *OrganizationsService) CheckUserIsStaff(ctx context.Context, db db.Queryer, userID uuid.UUID, organizationID guid.GUID) (staff organizations.Staff, err error) {
	staff, err = service.repo.FindStaff(ctx, db, userID, organizationID)
	if err != nil {
		if errs.IsNotFound(err) {
			err = kernel.ErrPermissionDenied
		}
		return
	}

	return
}
