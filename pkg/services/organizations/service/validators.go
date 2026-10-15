package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/countries"
	"markdown.ninja/stdx/money/vat"
	"markdown.ninja/stdx/retry"
)

func (service *OrganizationsService) validateOrganizationName(name string) error {
	if len(name) < organizations.OrganizationNameMinLength {
		return organizations.ErrOrganizationNameIsTooShort
	}

	if len(name) > organizations.OrganizationNameMaxLength {
		return organizations.ErrOrganizationNameIsTooLong
	}

	if !utf8.ValidString(name) {
		return organizations.ErrOrganizationNameIsNotValid
	}

	return nil
}

func (service *OrganizationsService) validateApiKeyName(name string) error {
	if len(name) < organizations.ApiKeyNameMinLength {
		return organizations.ErrApiKeyNameIsTooShort
	}

	if len(name) > organizations.ApiKeyNameMaxLength {
		return organizations.ErrApiKeyNameIsTooLong
	}

	if !utf8.ValidString(name) {
		return organizations.ErrApiKeyNameIsNotValid
	}

	return nil
}

func (service *OrganizationsService) cleanAndValidateBillingInformation(ctx context.Context, actorIsAdmin bool, billingInfo *organizations.BillingInformation) error {
	billingInfo.Name = strings.TrimSpace(billingInfo.Name)
	if billingInfo.Name == "" {
		return errs.InvalidArgument("name is empty")
	}
	if len(billingInfo.Name) > 60 {
		return errs.InvalidArgument("name is too long. max: 60")
	}
	if !utf8.ValidString(billingInfo.Name) {
		return errs.InvalidArgument("Billing name is not valid")
	}

	billingInfo.Email = strings.TrimSpace(billingInfo.Email)
	err := service.kernel.ValidateEmail(ctx, billingInfo.Email, true)
	if err != nil {
		return errs.InvalidArgument(fmt.Sprintf("Billing email is not valid: %s", err))
	}

	_, err = countries.Name(billingInfo.CountryCode)
	if err != nil {
		return err
	}

	billingInfo.AddressLine1 = strings.TrimSpace(billingInfo.AddressLine1)
	if billingInfo.AddressLine1 == "" {
		return errs.InvalidArgument("Adress line 1 is empty")
	}
	if len(billingInfo.AddressLine1) > 120 {
		return errs.InvalidArgument("Adress line 1 is too long. max: 120 characters")
	}
	if !utf8.ValidString(billingInfo.AddressLine1) {
		return errs.InvalidArgument("Address line 1 is not valid")
	}

	// can be empty
	billingInfo.AddressLine2 = strings.TrimSpace(billingInfo.AddressLine2)
	if len(billingInfo.AddressLine2) > 120 {
		return errs.InvalidArgument("Adress line 2 is too long. max: 120 characters")
	}
	if !utf8.ValidString(billingInfo.AddressLine2) {
		return errs.InvalidArgument("Address line 2 is not valid")
	}

	billingInfo.PostalCode = strings.TrimSpace(billingInfo.PostalCode)
	if billingInfo.PostalCode == "" {
		return errs.InvalidArgument("postal code is empty")
	}
	if len(billingInfo.PostalCode) > 20 {
		return errs.InvalidArgument("Postal code is too long. max: 20 charatcers")
	}
	if !utf8.ValidString(billingInfo.PostalCode) {
		return errs.InvalidArgument("Postal code is not valid")
	}

	billingInfo.City = strings.TrimSpace(billingInfo.City)
	if billingInfo.City == "" {
		return errs.InvalidArgument("city is empty")
	}
	if len(billingInfo.City) > 50 {
		return errs.InvalidArgument("Postal code is too long. max: 50 charatcers")
	}
	if !utf8.ValidString(billingInfo.City) {
		return errs.InvalidArgument("City is not valid")
	}

	// can be empty
	billingInfo.State = strings.TrimSpace(billingInfo.State)
	if len(billingInfo.State) > 60 {
		return errs.InvalidArgument("State is too long. max: 60 charatcers")
	}
	if !utf8.ValidString(billingInfo.State) {
		return errs.InvalidArgument("State is not valid")
	}

	if billingInfo.TaxID != nil {
		taxID := strings.TrimSpace(*billingInfo.TaxID)
		if taxID == "" {
			billingInfo.TaxID = nil
		} else if taxID != organizations.TestTaxID {
			var vatInfo *vat.VATresponse

			err = retry.Do(func() (retryErr error) {
				vatInfo, retryErr = vat.CheckVAT(ctx, taxID)
				return retryErr
			}, retry.Context(ctx), retry.Attempts(3), retry.Delay(1*time.Second))
			if err != nil {
				return fmt.Errorf("organizations: error checking VAT number: %w", err)
			}

			if !vatInfo.Valid {
				return errs.InvalidArgument("VAT number is not valid")
			}

			if vatInfo.CountryCode != billingInfo.CountryCode {
				return errs.InvalidArgument("VAT ID's country does not match the billing country")
			}

			billingInfo.TaxID = &taxID
		}
	}

	return nil
}

func validatePlan(planID kernel.PlanID, onlySelfServePlans bool) error {
	plan, ok := kernel.AllPlans[planID]
	if !ok {
		return organizations.ErrPlanIsNotValid
	}

	if onlySelfServePlans && !plan.SelfServe {
		return errs.InvalidArgument("This plan is not available for self-service")
	}

	return nil
}

func validateExtraSlots(extraSlots int64, planID kernel.PlanID) error {
	if extraSlots < 0 {
		return errs.InvalidArgument("slots can't be negative")
	}

	plan, planOk := kernel.AllPlans[planID]
	if !planOk {
		return organizations.ErrPlanIsNotValid
	}

	if extraSlots > plan.MaxExtraSlots {
		return errs.InvalidArgument(fmt.Sprintf("Too many extra slots. max: %d", plan.MaxExtraSlots))
	}

	return nil
}
