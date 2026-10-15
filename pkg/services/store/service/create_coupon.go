package service

import (
	"context"
	"math"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

// TODO: check if coupon with code already exists
func (service *StoreService) CreateCoupon(ctx context.Context, input store.CreateCouponInput) (coupon store.Coupon, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	// for now only admins can create coupons
	if !httpCtx.AccessToken.IsAdmin {
		err = kernel.ErrPermissionDenied
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	code := strings.TrimSpace(input.Code)
	discount := input.Discount
	description := strings.TrimSpace(input.Description)
	var expiresAt *time.Time

	if input.ExpiresAt != nil {
		expiresAtUtc := input.ExpiresAt.UTC()
		err = service.validateCouponExpiryDate(expiresAtUtc)
		if err != nil {
			return
		}

		expiresAt = &expiresAtUtc
	}

	err = service.validateCouponCode(code)
	if err != nil {
		return
	}

	var existingCoupon store.Coupon
	// check if code is not already in use
	existingCoupon, err = service.repo.FindCouponByCode(ctx, service.db, code)
	if err == nil && !existingCoupon.ID.Equal(coupon.ID) {
		err = store.ErrCouponCodeAlreadyExists(code)
	} else if err != nil {
		if errs.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		return
	}

	err = service.validateCouponDescription(description)
	if err != nil {
		return
	}

	err = service.validateCouponDiscount(discount)
	if err != nil {
		return
	}

	if input.Products == nil {
		err = store.ErrProductNotFound
		return
	}

	websiteProducts, err := service.repo.FindProductsByWebsiteID(ctx, service.db, input.WebsiteID, math.MaxInt64)
	if err != nil {
		return
	}

	productsDiff, err := service.diffProducts([]store.Product{}, websiteProducts, input.Products)
	if err != nil {
		return
	}

	coupon = store.Coupon{
		ID:          guid.NewTimeBased(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Code:        code,
		ExpiresAt:   expiresAt,
		Discount:    discount,
		UsesLimit:   0,
		ArchivedAt:  expiresAt,
		Description: description,
		WebsiteID:   input.WebsiteID,
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreateCoupon(ctx, tx, coupon)
		if txErr != nil {
			return txErr
		}

		txErr = service.associateProductsToCoupon(ctx, tx, coupon, productsDiff)
		if txErr != nil {
			return txErr
		}

		txErr = service.hydrateCoupon(ctx, tx, &coupon)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
