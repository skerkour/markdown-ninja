package store

import (
	"fmt"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/stdx/guid"
)

var (
	// Products
	ErrProductNotFound                        = errs.NotFound("Product not found.")
	ErrProductNameIsTooShort                  = errs.InvalidArgument(fmt.Sprintf("Name is too short (min: %d characters)", ProductNameMinLength))
	ErrProductNameIsTooLong                   = errs.InvalidArgument(fmt.Sprintf("Name is too long (max: %d characters)", ProductNameMaxLength))
	ErrProductNameIsNotValid                  = errs.InvalidArgument("Name is not valid")
	ErrProductDescriptionIsTooLong            = errs.InvalidArgument(fmt.Sprintf("Description is too long (max: %d characters)", ProductDescriptionMaxLength))
	ErrProductDescriptionIsNotValid           = errs.InvalidArgument("Description is not valid")
	ErrProductTypeIsNotValid                  = errs.InvalidArgument("Product type is not valid")
	ErrProductPriceCantBeNegative             = errs.InvalidArgument("Price can't be negative")
	ErrProductPriceIsTooHigh                  = errs.InvalidArgument(fmt.Sprintf("Price is too high (max: %d)", ProductPriceMax))
	ErrProductStatusIsNotValid                = errs.InvalidArgument("Status is not valid")
	ErrAtLeastOneProductIsRequiredForCheckout = errs.InvalidArgument("At least 1 product is required for checkout")
	ErrProductIsNotAvailable                  = func(productName string) error {
		return errs.InvalidArgument(fmt.Sprintf("%s is currently not available", productName))
	}
	ErrProductAccessNotFound       = errs.NotFound("Product access not found")
	ErrCantDeleteProductWithOrders = errs.InvalidArgument("A product can't be deleted once orders have been placed.")

	// Orders
	ErrOrderNotFound = func(orderID guid.GUID) error {
		return errs.NotFound(fmt.Sprintf("Order %s not found", orderID.String()))
	}
	ErrOrderIsNotCompleted = errs.NotFound("Order is not completed. Please make sure that the payment was successful or contact support if the problem persists.")

	// Refunds
	ErrRefundReasonNotValid                = errs.InvalidArgument(fmt.Sprintf("Refund reason is not valid. Valid values are: %s", ValidRefundReasons.ToSlice()))
	ErrRefundedAmountCantExceedOrderAmount = errs.InvalidArgument("Refunded amount cant exceed order amount")
	ErrRefundNotFound                      = errs.NotFound("Refund not found")
	ErrOrderAlreadyRefunded                = errs.NotFound("Order has already been refunded")
	ErrOrderMustBeCompletedToCreateRefund  = errs.InvalidArgument("Order must be completed to issue refund")

	// Assets
	ErrAssetFilnameAlreadyInUse = func(filname string) error {
		return errs.InvalidArgument(fmt.Sprintf("An asset with the name %s already exists", filname))
	}

	// Pages
	ErrProductIsNotACourse                = errs.InvalidArgument("Product is not a course.")
	ErrAllPagesMusBeProvidedForReordering = errs.InvalidArgument("All pages must be provided for reordering")
	ErrDuplicatePageFound                 = errs.InvalidArgument("Duplicate page.")
	ErrProductPageNotFound                = errs.NotFound("Page not found.")
	ErrProductShouldHaveAtLeastOnePage    = errs.InvalidArgument("Products should have at least 1 page")
	ErrPageContentIsTooLong               = errs.InvalidArgument("Page content is too long")
	ErrProductPageTitleIsNotValid         = errs.InvalidArgument("Page title is not valid")

	// Coupons
	ErrCouponNotFound              = errs.NotFound("Coupon not found.")
	ErrCouponCodeIsTooShort        = errs.InvalidArgument(fmt.Sprintf("Code is too short (min: %d characters)", CouponCodeMinLength))
	ErrCouponCodeIsTooLong         = errs.InvalidArgument(fmt.Sprintf("Code is too long (max: %d characters)", CouponCodeMaxLength))
	ErrCouponCodeIsNotValid        = errs.InvalidArgument("Code is not valid [A-Z-]")
	ErrCouponCodeMustBeUpperCase   = errs.InvalidArgument("Code must be upper case")
	ErrCouponDescriptionIsTooLong  = errs.InvalidArgument(fmt.Sprintf("Description is too long (max: %d characters)", CouponDescriptionMaxLength))
	ErrCouponDescriptionIsNotValid = errs.InvalidArgument("Description is not valid")
	ErrCouponDiscountIsNotValid    = errs.InvalidArgument("Discount is not valid (must be between 1 and 100)")
	ErrCouponCodeAlreadyExists     = func(code string) error {
		return errs.InvalidArgument(fmt.Sprintf("A Coupon with the code \"%s\" already exists", code))
	}
)
