package service

import (
	"context"
	"math"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
)

func (service *StoreService) UpdateCoupon(ctx context.Context, input store.UpdateCouponInput) (coupon store.Coupon, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	coupon, err = service.repo.FindCouponByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, coupon.WebsiteID)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	var productsDiff productsDiff

	if input.Code != nil {
		code := strings.TrimSpace(*input.Code)
		err = service.validateCouponCode(code)
		if err != nil {
			return
		}

		var existingCoupon store.Coupon
		// check if code is not already in use
		existingCoupon, err = service.repo.FindCouponByCode(ctx, service.db, code)
		if err == nil && !existingCoupon.ID.Equal(coupon.ID) {
			err = store.ErrCouponCodeAlreadyExists(code)
			return
		} else if err != nil {
			if !errs.IsNotFound(err) {
				return
			}
			err = nil
		}

		coupon.Code = code
	}

	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		err = service.validateProductDescription(description)
		if err != nil {
			return
		}
		coupon.Description = description
	}

	if input.Discount != nil {
		discount := *input.Discount
		err = service.validateCouponDiscount(discount)
		if err != nil {
			return
		}
		coupon.Discount = discount
	}

	if input.ExpiresAt != nil {
		expiresAt := *input.ExpiresAt
		err = service.validateCouponExpiryDate(expiresAt)
		if err != nil {
			return
		}
		coupon.ExpiresAt = &expiresAt
	}

	if input.Archived != nil {
		if *input.Archived == false {
			coupon.ArchivedAt = nil
		} else if *input.Archived && coupon.ArchivedAt == nil {
			coupon.ArchivedAt = &now
		}
	}

	couponProducts, err := service.repo.FindProductsForCoupon(ctx, service.db, coupon.ID)
	if err != nil {
		return
	}

	websiteProducts, err := service.repo.FindProductsByWebsiteID(ctx, service.db, coupon.WebsiteID, math.MaxInt64)
	if err != nil {
		return
	}

	if input.Products != nil {
		productsDiff, err = service.diffProducts(couponProducts, websiteProducts, input.Products)
		if err != nil {
			return
		}
	}

	coupon.UpdatedAt = now

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.UpdateCoupon(ctx, tx, coupon)
		if txErr != nil {
			return txErr
		}

		if input.Products != nil {
			txErr = service.associateProductsToCoupon(ctx, tx, coupon, productsDiff)
			if txErr != nil {
				return txErr
			}
		}

		txErr = service.hydrateCoupon(ctx, tx, &coupon)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
