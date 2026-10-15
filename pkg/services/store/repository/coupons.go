package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) CreateCoupon(ctx context.Context, db db.Queryer, coupon store.Coupon) (err error) {
	const query = `INSERT INTO coupons
			(id, created_at, updated_at, code, expires_at, discount, uses_limit, archived_at, description,
				website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = db.Exec(ctx, query, coupon.ID, coupon.CreatedAt, coupon.UpdatedAt, coupon.Code,
		coupon.ExpiresAt, coupon.Discount, coupon.UsesLimit, coupon.ArchivedAt, coupon.Description,
		coupon.WebsiteID)
	if err != nil {
		err = fmt.Errorf("store.CreateCoupon: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindCouponByID(ctx context.Context, db db.Queryer, couponID guid.GUID) (coupon store.Coupon, err error) {
	const query = "SELECT * FROM coupons WHERE id = $1"

	err = db.Get(ctx, &coupon, query, couponID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrCouponNotFound
		} else {
			err = fmt.Errorf("store.FindCouponByID: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) FindCouponByCode(ctx context.Context, db db.Queryer, code string) (coupon store.Coupon, err error) {
	const query = "SELECT * FROM coupons WHERE code = $1"

	err = db.Get(ctx, &coupon, query, code)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrCouponNotFound
		} else {
			err = fmt.Errorf("store.FindCouponByCode: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) FindCouponsByWebsiteID(ctx context.Context, db db.Queryer, websiteID guid.GUID) (ret []store.Coupon, err error) {
	ret = []store.Coupon{}
	const query = `SELECT * FROM coupons
		WHERE website_id = $1
		ORDER BY code
	`

	err = db.Select(ctx, &ret, query, websiteID)
	if err != nil {
		err = fmt.Errorf("store.FindCouponsByWebsiteID: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) UpdateCoupon(ctx context.Context, db db.Queryer, coupon store.Coupon) (err error) {
	const query = `UPDATE coupons
		SET updated_at = $1, code = $2, description = $3, expires_at = $4, discount = $5, uses_limit = $6,
			archived_at = $7
		WHERE id = $8
`

	_, err = db.Exec(ctx, query, coupon.UpdatedAt, coupon.Code, coupon.Description, coupon.ExpiresAt,
		coupon.Discount, coupon.UsesLimit, coupon.ArchivedAt,
		coupon.ID)
	if err != nil {
		err = fmt.Errorf("store.UpdateCoupon: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) CreateCouponProductRelation(ctx context.Context, db db.Queryer, relation store.CouponProductRelation) (err error) {
	const query = `INSERT INTO coupons_products
				(coupon_id, product_id)
			VALUES ($1, $2)`

	_, err = db.Exec(ctx, query, relation.CouponID, relation.ProductID)
	if err != nil {
		err = fmt.Errorf("store.CreateCouponProductRelation: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) DeleteCouponProductRelation(ctx context.Context, db db.Queryer, relation store.CouponProductRelation) (err error) {
	const query = `DELETE FROM coupons_products WHERE coupon_id = $1 AND product_id = $2`

	_, err = db.Exec(ctx, query, relation.CouponID, relation.ProductID)
	if err != nil {
		err = fmt.Errorf("store.DeleteCouponProductRelation: %w", err)
		return
	}

	return
}
