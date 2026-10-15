package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

func (repo *OrganizationsRepository) CreateStaff(ctx context.Context, db db.Queryer, staff organizations.Staff) (err error) {
	const query = `INSERT INTO staffs
	(created_at, updated_at, role, user_id, organization_id)
	VALUES ($1, $2, $3, $4, $5)`

	_, err = db.Exec(ctx, query, staff.CreatedAt, staff.UpdatedAt,
		staff.Role, staff.UserID, staff.OrganizationID)
	if err != nil {
		err = fmt.Errorf("organizations.CreateStaff: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) DeleteStaff(ctx context.Context, db db.Queryer, userID uuid.UUID, organizationID guid.GUID) (err error) {
	const query = `DELETE FROM staffs WHERE user_id = $1 AND organization_id = $2`

	_, err = db.Exec(ctx, query, userID, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.DeleteStaff: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) GetStaffsCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (count int64, err error) {
	const query = "SELECT COUNT(*) FROM staffs WHERE organization_id = $1"

	err = db.Get(ctx, &count, query, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.GetStaffsCountForOrganization: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindStaff(ctx context.Context, db db.Queryer, userID uuid.UUID, organizationID guid.GUID) (staff organizations.Staff, err error) {
	const query = "SELECT * FROM staffs WHERE user_id = $1 AND organization_id = $2"

	err = db.Get(ctx, &staff, query, userID, organizationID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = organizations.ErrStaffNotFound
		} else {
			err = fmt.Errorf("organizations.FindStaff: %w", err)
		}
		return
	}

	return
}

func (repo *OrganizationsRepository) FindStaffsForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (ret []organizations.Staff, err error) {
	ret = make([]organizations.Staff, 0, 10)
	const query = `SELECT * FROM staffs
			WHERE staffs.organization_id = $1`

	err = db.Select(ctx, &ret, query, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.FindStaffsForOrganization: %w", err)
		return
	}

	return
}
