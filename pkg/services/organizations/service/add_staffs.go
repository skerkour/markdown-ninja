package service

import (
	"context"
	"fmt"
	"time"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/set"
	"markdown.ninja/stdx/uuid"
)

func (service *OrganizationsService) AddStaffs(ctx context.Context, input organizations.AddStaffsInput) (ret []organizations.StaffWithDetails, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	_, err = service.kernel.CurrentUserID(ctx)
	if err != nil {
		return ret, err
	}

	accessToken := httpCtx.AccessToken
	if !accessToken.IsAdmin {
		return ret, kernel.ErrPermissionDenied
	}

	// unique IDs
	uniqueUserIDs := set.NewFromSlice(input.UserIDs)

	// find users and make sure they exist
	users, err := service.pingoo.ListUsers(ctx, pingoo.ListUsersInput{IDs: uniqueUserIDs.ToSlice()})
	if err != nil {
		return ret, err
	}

	if len(users.Data) != len(uniqueUserIDs) {
		for _, user := range users.Data {
			if !uniqueUserIDs.Contains(user.ID) {
				return ret, errs.InvalidArgument(fmt.Sprintf("user not found %s", user.ID.String()))
			}
		}
		return ret, errs.InvalidArgument("user not found")
	}

	usersByID := make(map[uuid.UUID]pingoo.User, len(users.Data))
	for _, user := range users.Data {
		usersByID[user.ID] = user
	}

	ret = make([]organizations.StaffWithDetails, 0, len(uniqueUserIDs))

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		// make sure some user is not alrady staff
		existingStaffs, txErr := service.repo.FindStaffsForOrganization(ctx, tx, input.OrganizationID)
		if txErr != nil {
			return txErr
		}

		for _, existingStaff := range existingStaffs {
			if uniqueUserIDs.Contains(existingStaff.UserID) {
				return errs.InvalidArgument(fmt.Sprintf("User %s is already staff", existingStaff.UserID))
			}
		}

		// create staffs
		now := time.Now().UTC()
		for userID := range uniqueUserIDs.Iter() {
			newStaff := organizations.Staff{
				CreatedAt:      now,
				UpdatedAt:      now,
				Role:           organizations.StaffRoleAdministrator,
				UserID:         userID,
				OrganizationID: input.OrganizationID,
			}
			txErr = service.repo.CreateStaff(ctx, tx, newStaff)
			if txErr != nil {
				return txErr
			}

			user := usersByID[userID]
			staffWithDetails := organizations.StaffWithDetails{
				Staff: newStaff,
				Name:  user.Name,
				Email: user.Email,
			}
			ret = append(ret, staffWithDetails)
		}

		return nil
	})
	if err != nil {
		return ret, err
	}

	return ret, nil
}
