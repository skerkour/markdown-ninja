package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

func (repo *OrganizationsRepository) CreateOrganization(ctx context.Context, db db.Queryer, organization organizations.Organization) (err error) {
	const query = `INSERT INTO organizations
	(id, created_at, updated_at, name, plan, billing_information, stripe_customer_id, stripe_subscription_id,
		payment_due_since, usage_last_invoiced_at, extra_slots)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err = db.Exec(ctx, query, organization.ID, organization.CreatedAt, organization.UpdatedAt,
		organization.Name, organization.Plan, organization.BillingInformation, organization.StripeCustomerID, organization.StripeSubscriptionID,
		organization.PaymentDueSince, organization.UsageLastInvoicedAt, organization.ExtraSlots)
	if err != nil {
		err = fmt.Errorf("organizations.CreateOrganization: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) UpdateOrganization(ctx context.Context, db db.Queryer, organization organizations.Organization) (err error) {
	const query = `UPDATE organizations
		SET updated_at = $1, name = $2, plan = $3, billing_information = $4, stripe_customer_id = $5,
			stripe_subscription_id = $6,
			payment_due_since = $7, usage_last_invoiced_at = $8, extra_slots = $9
		WHERE id = $10`

	_, err = db.Exec(ctx, query, organization.UpdatedAt, organization.Name, organization.Plan,
		organization.BillingInformation, organization.StripeCustomerID, organization.StripeSubscriptionID,
		organization.PaymentDueSince,
		organization.UsageLastInvoicedAt, organization.ExtraSlots,
		organization.ID)
	if err != nil {
		err = fmt.Errorf("organizations.UpdateOrganization: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) DeleteOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (err error) {
	const query = `DELETE FROM organizations WHERE id = $1`

	_, err = db.Exec(ctx, query, organizationID)
	if err != nil {
		err = fmt.Errorf("organizations.DeleteOrganization: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindOrganizationByID(ctx context.Context, db db.Queryer, organizationID guid.GUID, forUpdate bool) (session organizations.Organization, err error) {
	query := `SELECT * FROM organizations WHERE id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Get(ctx, &session, query, organizationID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = organizations.ErrOrganizationNotFound
		} else {
			err = fmt.Errorf("organizations.FindOrganizationByID: %w", err)
		}
		return
	}

	return
}

func (repo *OrganizationsRepository) FindOrganizationByStripeCustomerID(ctx context.Context, db db.Queryer, stripeCustomerID string, forUpdate bool) (session organizations.Organization, err error) {
	query := `SELECT * FROM organizations WHERE stripe_customer_id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Get(ctx, &session, query, stripeCustomerID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = organizations.ErrOrganizationNotFound
		} else {
			err = fmt.Errorf("organizations.FindOrganizationByStripeCustomerID: %w", err)
		}
		return
	}

	return
}

func (repo *OrganizationsRepository) FindOrganizationsForUser(ctx context.Context, db db.Queryer, userID uuid.UUID) (ret []organizations.Organization, err error) {
	ret = make([]organizations.Organization, 0, 4)
	const query = `SELECT * FROM organizations WHERE id = ANY (
		SELECT organization_id FROM staffs WHERE user_id = $1
	)
	ORDER BY name`

	err = db.Select(ctx, &ret, query, userID)
	if err != nil {
		err = fmt.Errorf("organizations.FindOrganizationsForUser: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindOrganizationsByPlan(ctx context.Context, db db.Queryer, planID kernel.PlanID) (ret []organizations.Organization, err error) {
	ret = make([]organizations.Organization, 0, 4)
	const query = `SELECT * FROM organizations WHERE plan = $1`

	err = db.Select(ctx, &ret, query, planID)
	if err != nil {
		err = fmt.Errorf("organizations.FindOrganizationsByPlan: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) FindAllOrganizations(ctx context.Context, db db.Queryer) (ret []organizations.Organization, err error) {
	ret = make([]organizations.Organization, 0, 10)
	const query = "SELECT * FROM organizations ORDER BY id"

	err = db.Select(ctx, &ret, query)
	if err != nil {
		err = fmt.Errorf("organizations.FindAllOrganizations: %w", err)
		return
	}

	return
}

func (repo *OrganizationsRepository) GetOrganizationsCount(ctx context.Context, db db.Queryer) (count int64, err error) {
	const query = "SELECT COUNT(*) FROM organizations"

	err = db.Get(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("organizations.GetOrganizationsCount: %w", err)
	}

	return count, nil
}

func (repo *OrganizationsRepository) GetPayingOrganizationsCount(ctx context.Context, db db.Queryer) (count int64, err error) {
	const query = "SELECT COUNT(*) FROM organizations WHERE plan = $1"

	err = db.Get(ctx, &count, query, kernel.PlanPro.ID)
	if err != nil {
		return 0, fmt.Errorf("organizations.GePayingtOrganizationsCount: %w", err)
	}

	return count, nil
}

func (repo *OrganizationsRepository) GetTotalExtraSlotsCount(ctx context.Context, db db.Queryer) (count int64, err error) {
	const query = "SELECT SUM(extra_slots) FROM organizations"

	err = db.Get(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("organizations.GetTotalExtraSlotsCount: %w", err)
	}

	return count, nil
}
