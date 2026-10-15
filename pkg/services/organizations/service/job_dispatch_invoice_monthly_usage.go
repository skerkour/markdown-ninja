package service

import (
	"context"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/queue"
)

func (service *OrganizationsService) JobDispatchInvoiceMonthlyUsage(ctx context.Context, _ organizations.JobDispatchInvoiceMonthlyUsage) error {
	now := time.Now().UTC()

	// Only execute this job on the 2 of every month
	if now.Day() != 2 {
		return nil
	}
	// if now.Day() != timeutil.GetFirstMondayOfTheMonth(now).Day() {
	// 	return nil
	// }

	proOrganizations, err := service.repo.FindOrganizationsByPlan(ctx, service.db, kernel.PlanPro.ID)
	if err != nil {
		return err
	}

	jobs := make([]queue.NewJobInput, 0, len(proOrganizations))
	jobScheduledFor := time.Now().UTC()

	for i, organization := range proOrganizations {
		if organization.StripeCustomerID == nil || organization.StripeSubscriptionID == nil {
			continue
		}

		// Limit the number of requests per second
		if i != 0 && i%10 == 0 {
			jobScheduledFor = jobScheduledFor.Add(time.Second)
		}

		job := queue.NewJobInput{
			Data: organizations.JobInvoiceMonthlyUsage{
				OrganizationID: organization.ID,
				IdempotencyKey: guid.NewTimeBased().String(),
			},
			ScheduledFor: &jobScheduledFor,
			Timeout:      new(int64(300)),
			RetryDelay:   new(int64(300)),
		}
		jobs = append(jobs, job)
	}

	err = service.queue.PushMany(ctx, nil, jobs)
	if err != nil {
		return fmt.Errorf("organizations.JobDispatchInvoiceMonthlyUsage: pushing jobs to queue: %w", err)
	}

	return nil
}
