package service

import (
	"context"

	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/uuid"
)

func (service *OrganizationsService) ListOrganizationsForUser(ctx context.Context, db db.Queryer, userID uuid.UUID) (orgs []organizations.Organization, err error) {
	return service.repo.FindOrganizationsForUser(ctx, db, userID)
}
