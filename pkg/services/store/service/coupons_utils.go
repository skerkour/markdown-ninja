package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type productsDiff struct {
	couponProductRelationsToCreate []store.Product
	couponProductRelationsToRemove []store.Product
}

// hydrateCoupon hydrates the coupons with products
func (service *StoreService) hydrateCoupon(ctx context.Context, db db.Queryer, coupon *store.Coupon) (err error) {
	products, err := service.repo.FindProductsForCoupon(ctx, db, coupon.ID)
	if err != nil {
		return
	}

	productsIDs := make([]guid.GUID, len(products))
	for i := range products {
		productsIDs[i] = products[i].ID
	}

	coupon.Products = productsIDs

	return
}

func (service *StoreService) diffProducts(currentProducts []store.Product, websiteProducts []store.Product, newProducts []guid.GUID) (diff productsDiff, err error) {
	diff = productsDiff{
		couponProductRelationsToCreate: []store.Product{},
		couponProductRelationsToRemove: []store.Product{},
	}

	currentProductsMap := make(map[guid.GUID]store.Product)
	for _, product := range currentProducts {
		currentProductsMap[product.ID] = product
	}

	websiteProductsMap := make(map[guid.GUID]store.Product)
	for _, product := range websiteProducts {
		websiteProductsMap[product.ID] = product
	}

	newProductsSet := make(map[guid.GUID]bool)
	for _, productID := range newProducts {
		newProductsSet[productID] = true
	}

	for _, currentProduct := range currentProducts {
		if isInNewProducts := newProductsSet[currentProduct.ID]; !isInNewProducts {
			diff.couponProductRelationsToRemove = append(diff.couponProductRelationsToRemove, currentProduct)
		}
	}

	for productID := range newProductsSet {
		if _, alreadyAssociated := currentProductsMap[productID]; !alreadyAssociated {
			existingProduct, ok := websiteProductsMap[productID]
			if ok {
				diff.couponProductRelationsToCreate = append(diff.couponProductRelationsToCreate, existingProduct)
			} else {
				err = store.ErrProductNotFound
				return
			}
		}
	}

	return
}

func (service *StoreService) associateProductsToCoupon(ctx context.Context, db db.Queryer, coupon store.Coupon, diff productsDiff) (err error) {
	for _, relationToCreate := range diff.couponProductRelationsToCreate {
		relation := store.CouponProductRelation{
			ProductID: relationToCreate.ID,
			CouponID:  coupon.ID,
		}
		err = service.repo.CreateCouponProductRelation(ctx, db, relation)
		if err != nil {
			return
		}
	}

	for _, relationToDelete := range diff.couponProductRelationsToRemove {
		relation := store.CouponProductRelation{
			ProductID: relationToDelete.ID,
			CouponID:  coupon.ID,
		}
		err = service.repo.DeleteCouponProductRelation(ctx, db, relation)
		if err != nil {
			return
		}
	}

	return
}
