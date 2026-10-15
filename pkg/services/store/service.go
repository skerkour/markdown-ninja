package store

import (
	"context"
	"time"

	"github.com/stripe/stripe-go/v81"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type Service interface {
	// Stripe
	HandleStripeEvent(ctx context.Context, stripeEvent stripe.Event) (err error)

	// Products
	CreateProduct(ctx context.Context, input CreateProductInput) (product Product, err error)
	GetProduct(ctx context.Context, input GetProductInput) (product Product, err error)
	ListProducts(ctx context.Context, input ListProductsInput) (ret kernel.PaginatedResult[Product], err error)
	UpdateProduct(ctx context.Context, input UpdateProductInput) (product Product, err error)
	FindProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (product Product, err error)
	CheckProductAccess(ctx context.Context, db db.Queryer, productID guid.GUID) (err error)
	GiveContactsAccessToProduct(ctx context.Context, input GiveContactsAccessToProductInput) (err error)
	FindProductsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (products []Product, err error)
	FindProductsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID, limit int64) (products []Product, err error)
	// TODO
	RemoveAccessToProduct(ctx context.Context, input RemoveAccessToProductInput) (err error)
	FindProductWithContent(ctx context.Context, db db.Queryer, productID guid.GUID) (product Product, err error)
	DeleteProduct(ctx context.Context, input DeleteProductInput) (err error)

	// Orders
	PlaceOrder(ctx context.Context, input PlaceOrderInput) (ret PlaceOrderOutput, err error)
	CompleteOrder(ctx context.Context, input CompleteOrderInput) (err error)
	CancelOrder(ctx context.Context, input CancelOrderInput) (err error)
	ListOrders(ctx context.Context, input ListOrdersInput) (ret kernel.PaginatedResult[OrderMetadata], err error)
	FindOrdersForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (orders []Order, err error)
	FindCompletedOrdersForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (orders []Order, err error)
	GetWebsiteRevenue(ctx context.Context, db db.Queryer, websiteID guid.GUID, from, to time.Time) (revenue int64, err error)
	GetOrder(ctx context.Context, input GetOrderInput) (order Order, err error)

	// Refunds
	ListRefunds(ctx context.Context, input ListRefundsInput) (ret kernel.PaginatedResult[Refund], err error)
	CreateRefund(ctx context.Context, input CreateRefundInput) (refund Refund, err error)

	// Pages
	CreateProductPage(ctx context.Context, input CreateProductPageInput) (page ProductPage, err error)
	UpdateProductPage(ctx context.Context, input UpdateProductPageInput) (page ProductPage, err error)
	DeleteProductPage(ctx context.Context, input DeleteProductPageInput) (err error)
	GetProductPage(ctx context.Context, input GetProductPageInput) (page ProductPage, err error)

	// Coupons
	CreateCoupon(ctx context.Context, input CreateCouponInput) (coupon Coupon, err error)
	GetCoupon(ctx context.Context, input GetCouponInput) (coupon Coupon, err error)
	ListCoupons(ctx context.Context, input ListCouponsInput) (res kernel.PaginatedResult[Coupon], err error)
	UpdateCoupon(ctx context.Context, input UpdateCouponInput) (coupon Coupon, err error)

	// Jobs
	JobSendOrderConfirmationEmail(ctx context.Context, input JobSendOrderConfirmationEmail) (err error)
	JobCreateStripeRefund(ctx context.Context, input JobCreateStripeRefund) (err error)
	JobSyncRefundWithStripe(ctx context.Context, input JobSyncRefundWithStripe) (err error)

	// Tasks
	TaskSyncRefundsWithStripe(ctx context.Context)
}
