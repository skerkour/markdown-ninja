package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/dbx"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *OrganizationsRepository) CreateStaffInvitations(ctx context.Context, db db.Queryer, invitations []organizations.StaffInvitation) (err error) {
	if len(invitations) == 0 {
		return
	}

	query := `INSERT INTO staff_invitations
            (id, created_at, updated_at, role, invitee_email, organization_id, inviter_id) VALUES`

	args := make([]any, 0, len(invitations)*7)
	for _, invitation := range invitations {
		args = append(args, invitation.ID, invitation.CreatedAt, invitation.UpdatedAt,
			invitation.Role, invitation.InviteeEmail, invitation.OrganizationID, invitation.InviterID)
	}

	query, err = dbx.BuildQuery(query, 7, args)
	if err != nil {
		return fmt.Errorf("organizations.CreateStaffInvitations: %w", err)
	}

	_, err = db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("organizations.CreateStaffInvitations: %w", err)
	}

	return nil
}

func (repo *OrganizationsRepository) DeleteStaffInvitation(ctx context.Context, db db.Queryer, invitationID guid.GUID) (err error) {
	const query = `DELETE FROM staff_invitations WHERE id = $1`

	_, err = db.Exec(ctx, query, invitationID)
	if err != nil {
		err = fmt.Errorf("organizations.DeleteStaffInvitation: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindStaffInvitationByID(ctx context.Context, db db.Queryer, invitationID guid.GUID, forUpdate bool) (invitation organizations.StaffInvitation, err error) {
	query := `SELECT * FROM staff_invitations WHERE id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Get(ctx, &invitation, query, invitationID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = organizations.ErrStaffInvitationNotFound
		} else {
			err = fmt.Errorf("organizations.FindStaffInvitationByID: %w", err)
		}
		return
	}

	return
}

func (repo *OrganizationsRepository) FindStaffInvitationsForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (ret []organizations.StaffInvitation, err error) {
	ret = make([]organizations.StaffInvitation, 1)
	const query = `SELECT * FROM staff_invitations WHERE organization_id = $1`

	err = db.Select(ctx, &ret, query, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.FindStaffInvitationsForOrganization: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) GetStaffInvitationsCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (count int64, err error) {
	const query = `SELECT COUNT(*) FROM staff_invitations WHERE organization_id = $1`

	err = db.Get(ctx, &count, query, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.GetStaffInvitationsCountForOrganization: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindInvitationsForInviteeEmail(ctx context.Context, db db.Queryer, inviteeEmail string) (ret []organizations.StaffInvitationWithOrganizationDetails, err error) {
	ret = make([]organizations.StaffInvitationWithOrganizationDetails, 0, 1)
	const query = `SELECT staff_invitations.*,
			organizations.id AS organization_id, organizations.name AS organization_name
		FROM staff_invitations
		INNER JOIN organizations ON staff_invitations.organization_id = organizations.id
		WHERE staff_invitations.invitee_email = $1`

	err = db.Select(ctx, &ret, query, inviteeEmail)
	if err != nil {
		err = fmt.Errorf("organizations.FindInvitationsForInviteeEmail: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindInviteeInvitationsByIDs(ctx context.Context, db db.Queryer, invitationIDs []guid.GUID) (ret []organizations.StaffInvitationWithOrganizationDetails, err error) {
	ret = make([]organizations.StaffInvitationWithOrganizationDetails, 0, len(invitationIDs))
	const query = `SELECT staff_invitations.*,
			organizations.id AS organization_id, organizations.name AS organization_name
		FROM staff_invitations
		INNER JOIN organizations ON staff_invitations.organization_id = organizations.id
		WHERE staff_invitations.id = ANY($1)`

	if len(invitationIDs) == 0 {
		return
	}

	err = db.Select(ctx, &ret, query, invitationIDs)
	if err != nil {
		err = fmt.Errorf("organizations.FindInviteeInvitationsByIDs: %w", err)
		return
	}

	return
}
