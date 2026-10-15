package service

import (
	"context"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

func (service *OrganizationsService) FindStaffsForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (staffs []organizations.StaffWithDetails, err error) {
	return service.getStaffsWithDetails(ctx, db, organizationID)
}

func (service *OrganizationsService) getStaffsWithDetails(ctx context.Context, db db.Queryer, orgID guid.GUID) (staffsWithDetails []organizations.StaffWithDetails, err error) {
	staffs, err := service.repo.FindStaffsForOrganization(ctx, service.db, orgID)
	if err != nil {
		return staffsWithDetails, err
	}

	userIDs := make([]uuid.UUID, 0, len(staffs))
	for _, staff := range staffs {
		userIDs = append(userIDs, uuid.UUID(staff.UserID))
	}

	staffUsers, err := service.pingoo.ListUsers(ctx, pingoo.ListUsersInput{IDs: userIDs})
	if err != nil {
		return staffsWithDetails, err
	}
	staffUsersById := make(map[uuid.UUID]pingoo.User, len(staffUsers.Data))
	for _, user := range staffUsers.Data {
		staffUsersById[user.ID] = user
	}

	staffsWithDetails = make([]organizations.StaffWithDetails, 0, len(staffs))
	for _, staff := range staffs {
		if user, userOk := staffUsersById[uuid.UUID(staff.UserID)]; userOk {
			staffWithDetails := organizations.StaffWithDetails{
				Staff: staff,
				Name:  user.Name,
				Email: user.Email,
			}
			staffsWithDetails = append(staffsWithDetails, staffWithDetails)
		}
	}

	return staffsWithDetails, nil
}
