package store

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/set"
)

const (
	ProductNameMaxLength        = 150
	ProductNameMinLength        = 1
	ProductDescriptionMaxLength = 420
	ProductPriceMax             = math.MaxInt32

	CouponDescriptionMaxLength = 512
	CouponCodeMinLength        = 2
	CouponCodeMaxLength        = 42
)

var (
	CouponCodeRegexp = regexp.MustCompile("^[-A-Z0-9]+$")
)

type ProductType int64

const (
	ProductTypeBook ProductType = iota
	ProductTypeCourse
	ProductTypeDigitalDownload
	// ProductTypeBundle
	// ProductTypeSubscription
)

// MarshalText implements encoding.TextMarshaler.
func (productType ProductType) MarshalText() (ret []byte, err error) {
	switch productType {
	case ProductTypeBook:
		ret = []byte("book")
	case ProductTypeCourse:
		ret = []byte("course")
	case ProductTypeDigitalDownload:
		ret = []byte("download")
	default:
		err = fmt.Errorf("Unknown ProductType: %d", productType)
	}
	return
}

func (contentType ProductType) String() string {
	ret, _ := contentType.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (productType *ProductType) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case "book":
		*productType = ProductTypeBook
	case "course":
		*productType = ProductTypeCourse
	case "download":
		*productType = ProductTypeDigitalDownload
	default:
		err = fmt.Errorf("Unknown ProductType: %s", string(data))
	}
	return nil
}

type ProductStatus int64

const (
	ProductStatusDraft ProductStatus = iota
	ProductStatusActive
)

const (
	ProductStatusDraftStr  = "draft"
	ProductStatusActiveStr = "active"
)

// MarshalText implements encoding.TextMarshaler.
func (productStatus ProductStatus) MarshalText() (ret []byte, err error) {
	switch productStatus {
	case ProductStatusDraft:
		ret = []byte(ProductStatusDraftStr)
	case ProductStatusActive:
		ret = []byte(ProductStatusActiveStr)
	default:
		err = fmt.Errorf("Unknown ProductStatus: %d", productStatus)
	}
	return
}

func (productStatus ProductStatus) String() string {
	ret, _ := productStatus.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (productStatus *ProductStatus) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case ProductStatusDraftStr:
		*productStatus = ProductStatusDraft
	case ProductStatusActiveStr:
		*productStatus = ProductStatusActive
	default:
		err = fmt.Errorf("Unknown ProductStatus: %s", string(data))
	}
	return nil
}

type OrderStatus int64

const (
	// Order has been placed and is awaiting payment
	OrderStatusPending OrderStatus = iota
	OrderStatusCompleted
	OrderStatusCanceled
)

const (
	OrderStatusPendingStr   = "pending"
	OrderStatusCompletedStr = "completed"
	OrderStatusCanceledStr  = "canceled"
)

// MarshalText implements encoding.TextMarshaler.
func (orderStatus OrderStatus) MarshalText() (ret []byte, err error) {
	switch orderStatus {
	case OrderStatusPending:
		ret = []byte(OrderStatusPendingStr)
	case OrderStatusCompleted:
		ret = []byte(OrderStatusCompletedStr)
	case OrderStatusCanceled:
		ret = []byte(OrderStatusCanceledStr)
	default:
		err = fmt.Errorf("Unknown OrderStatus: %d", orderStatus)
	}
	return
}

func (orderStatus OrderStatus) String() string {
	ret, _ := orderStatus.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (orderStatus *OrderStatus) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case OrderStatusPendingStr:
		*orderStatus = OrderStatusPending
	case OrderStatusCompletedStr:
		*orderStatus = OrderStatusCompleted
	case OrderStatusCanceledStr:
		*orderStatus = OrderStatusCanceled
	default:
		err = fmt.Errorf("Unknown OrderStatus: %s", string(data))
	}
	return nil
}

type RefundStatus string

const (
	RefundStatusPending        RefundStatus = "pending"
	RefundStatusRequiresAction RefundStatus = "requires_action"
	RefundStatusSucceeded      RefundStatus = "succeeded"
	RefundStatusFailed         RefundStatus = "failed"
	RefundStatusCanceled       RefundStatus = "canceled"
)

type RefundReason string

const (
	RefundReasonDuplicate               RefundReason = "duplicate"
	RefundReasonFraudulent              RefundReason = "fraudulent"
	RefundReasonRequestedByCustomer     RefundReason = "requested_by_customer"
	RefundReasonExpiredUncapturedCharge RefundReason = "expired_uncaptured_charge"
)

var ValidRefundReasons = set.NewFromSlice([]RefundReason{
	RefundReasonDuplicate,
	RefundReasonFraudulent,
	RefundReasonRequestedByCustomer,
	RefundReasonExpiredUncapturedCharge,
})

type RefundFailureReason string

const (
	RefundFailureReasonLostOrStolenCard               RefundFailureReason = "lost_or_stolen_card"
	RefundFailureReasonExpiredOrCanceledCard          RefundFailureReason = "expired_or_canceled_card"
	RefundFailureReasonChargeForPendingRefundDisputed RefundFailureReason = "charge_for_pending_refund_disputed"
	RefundFailureReasonInsufficientFunds              RefundFailureReason = "insufficient_funds"
	RefundFailureReasonDeclined                       RefundFailureReason = "declined"
	RefundFailureReasonMerchantRequest                RefundFailureReason = "merchant_request"
	RefundFailureReasonUnknown                        RefundFailureReason = "unknown"
)

////////////////////////////////////////////////////////////////////////////////////////////////////
// Entities
////////////////////////////////////////////////////////////////////////////////////////////////////

type Product struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Name        string        `db:"name" json:"name"`
	Description string        `db:"description" json:"description"`
	Type        ProductType   `db:"type" json:"type"`
	Price       int64         `db:"price" json:"price"`
	Status      ProductStatus `db:"status" json:"status"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`

	Content []ProductPage   `json:"content"`
	Assets  []content.Asset `json:"assets"`
}

type ProductPage struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Position int64  `db:"position" json:"position"`
	Title    string `db:"title" json:"title"`
	Size     int64  `db:"size" json:"size"`
	// BLAKE3 hash of the bodyMarkdown
	Hash         kernel.BytesHex `db:"hash" json:"hash"`
	BodyMarkdown string          `db:"body_markdown" json:"body_markdown"`

	ProductID guid.GUID `db:"product_id" json:"-"`
}

type ContactProductAccess struct {
	CreatedAt time.Time `db:"created_at"`
	ContactID guid.GUID `db:"contact_id"`
	ProductID guid.GUID `db:"product_id"`
}

type Order struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	TotalAmount int64             `db:"total_amount" json:"total_amount"`
	Currency    websites.Currency `db:"currency" json:"currency"`
	Notes       string            `db:"notes" json:"notes"`
	Status      OrderStatus       `db:"status" json:"status"`
	CompletedAt *time.Time        `db:"completed_at" json:"completed_at"`
	CanceledAt  *time.Time        `db:"canceled_at" json:"canceled_at"`

	Email   string `db:"email" json:"email"`
	Country string `db:"country" json:"country"`
	// customer-provided additional information to show on the invoice e.g. company name, address, tax ID...
	AdditionalInvoiceInformation string `db:"additional_invoice_information" json:"additional_invoice_information"`

	StripeCheckoutSessionID string  `db:"stripe_checkout_session_id" json:"stripe_checkout_session_id"`
	StripPaymentItentID     *string `db:"stripe_payment_intent_id" json:"stripe_payment_intent_id"`
	StripeInvoiceID         *string `db:"stripe_invoice_id" json:"stripe_invoice_id"`
	StripeInvoiceUrl        *string `db:"stripe_invoice_url" json:"stripe_invoice_url"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`
	ContactID guid.GUID `db:"contact_id" json:"contact_id"`

	LineItems []OrderLineItem `db:"-" json:"line_items"`
	Refunds   []Refund        `db:"-" json:"refunds"`
}

type OrderMetadata struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Email       string            `db:"email" json:"email"`
	TotalAmount int64             `db:"total_amount" json:"total_amount"`
	Currency    websites.Currency `db:"currency" json:"currency"`
	Status      OrderStatus       `db:"status" json:"status"`
	CompletedAt *time.Time        `db:"completed_at" json:"completed_at"`
	CanceledAt  *time.Time        `db:"canceled_at" json:"canceled_at"`
	Country     string            `db:"country" json:"country"`

	ContactID guid.GUID `db:"contact_id" json:"contact_id"`
}

type OrderLineItem struct {
	ProductName string `db:"product_name" json:"product_name"`
	// The unit price of the product, not including any discounts.
	OriginalProductPrice int64     `db:"original_product_price" json:"original_product_price"`
	Quantity             int64     `db:"quantity" json:"quantity"`
	OrderID              guid.GUID `db:"order_id" json:"order_id"`
	ProductID            guid.GUID `db:"product_id" json:"product_id"`
}

type Coupon struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Code        string     `db:"code" json:"code"`
	ExpiresAt   *time.Time `db:"expires_at" json:"expires_at"`
	Discount    int64      `db:"discount" json:"discount"`
	UsesLimit   int64      `db:"uses_limit" json:"uses_limit"`
	ArchivedAt  *time.Time `db:"archived_at" json:"-"`
	Description string     `db:"description" json:"description"`

	WebsiteID guid.GUID `db:"website_id" json:"-"`

	Products []guid.GUID `json:"products"`
}

func (coupon Coupon) MarshalJSON() ([]byte, error) {
	// We need a special type otherwise MarshalJSON will trigger infinite recursion
	type couponJson Coupon

	return json.Marshal(struct {
		couponJson
		Archived bool `json:"archived"`
	}{
		couponJson: couponJson(coupon),
		Archived:   coupon.ArchivedAt != nil,
	})
}

type CouponProductRelation struct {
	CouponID  guid.GUID `db:"coupon_id"`
	ProductID guid.GUID `db:"product_id"`
}

type Refund struct {
	ID        guid.GUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	Amount         int64                `db:"amount" json:"amount"`
	Currency       websites.Currency    `db:"currency" json:"currency"`
	Notes          string               `db:"notes" json:"notes"`
	Status         RefundStatus         `db:"status" json:"status"`
	Reason         RefundReason         `db:"reason" json:"reason"`
	FailureReason  *RefundFailureReason `db:"failure_reason" json:"failure_reason"`
	StripeRefundID *string              `db:"stripe_refund_id" json:"stripe_refund_id"`

	WebsiteID guid.GUID `db:"website_id" json:"website_id"`
	OrderID   guid.GUID `db:"order_id" json:"order_id"`
}

// type Transaction struct {
// 	ID   guid.GUID `db:"id"`
// 	Time time.Time `db:"time"`
//
// OriginalPrice
// paid
// CouponDiscount
// CouponID
// ProductName
// ProductID
// CustomerName
// CustomerAddress
// CustomerCountry
// CustomerEmail
//

// 	WebsiteID guid.GUID `db:"website_id"`
// }

// type TransactionProduct struct {
// 	Quantity         int64
// 	PricePaidPerUnit int64

// 	ProductID     guid.GUID
// 	TransactionID guid.GUID
// }

////////////////////////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////////////////////////

type CreateProductInput struct {
	WebsiteID   guid.GUID   `json:"website_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Type        ProductType `json:"type"`
	Price       int64       `json:"price"`
}

type GetProductInput struct {
	ID guid.GUID `json:"id"`
}

type ListProductsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type UpdateProductInput struct {
	ID          guid.GUID      `json:"id"`
	Name        *string        `json:"name"`
	Description *string        `json:"description"`
	Status      *ProductStatus `json:"status"`
	Price       *int64         `json:"price"`
}

type CreateCouponInput struct {
	WebsiteID   guid.GUID   `json:"website_id"`
	Code        string      `json:"code"`
	ExpiresAt   *time.Time  `json:"expires_at"`
	Discount    int64       `json:"discount"`
	Description string      `json:"description"`
	Products    []guid.GUID `json:"products"`
}

type GetCouponInput struct {
	ID guid.GUID `json:"id"`
}

type ListCouponsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type UpdateCouponInput struct {
	ID          guid.GUID   `json:"id"`
	Code        *string     `json:"code"`
	ExpiresAt   *time.Time  `json:"expires_at"`
	Discount    *int64      `json:"discount"`
	Description *string     `json:"description"`
	Archived    *bool       `json:"archived"`
	Products    []guid.GUID `json:"products"`
}

type CreateProductPageInput struct {
	ProductID    guid.GUID `json:"product_id"`
	Title        string    `json:"title"`
	BodyMarkdown string    `json:"body_markdown"`
}

type UpdateProductPageInput struct {
	ID           guid.GUID `json:"id"`
	Title        *string   `json:"title"`
	BodyMarkdown *string   `json:"body_markdown"`
}

type DeleteProductPageInput struct {
	ID guid.GUID `json:"id"`
}

type ReorderProductPagesInput struct {
	ProductID guid.GUID
	Pages     []guid.GUID
}

type GetBookChapterInput struct {
	ID guid.GUID `json:"id"`
}

type GetProductPageInput struct {
	ID guid.GUID `json:"id"`
}

type DeleteBookVersionInput struct {
	ID guid.GUID `json:"id"`
}

type GiveContactsAccessToProductInput struct {
	Emails    []string  `json:"emails"`
	ProductID guid.GUID `json:"product_id"`
}

type RemoveAccessToProductInput struct {
	Emails    []string  `json:"emails"`
	ProductID guid.GUID `json:"product_id"`
}

type ListOrdersInput struct {
	WebsiteID guid.GUID  `json:"website_id"`
	Query     string     `json:"query"`
	Limit     int64      `json:"limit"`
	After     *guid.GUID `json:"after"`
}

type PlaceOrderInput struct {
	Products                     []guid.GUID `json:"products"`
	Email                        *string     `json:"email"`
	SubscribeToNewsletter        bool        `json:"subscribe_to_newsletter"`
	AdditionalInvoiceInformation *string     `json:"additional_invoice_information"`
}

type PlaceOrderOutput struct {
	StripeCheckoutUrl string `json:"stripe_checkout_url"`
}

type CompleteOrderInput struct {
	OrderID guid.GUID `json:"order_id"`
}

type CancelOrderInput struct {
	OrderID guid.GUID `json:"order_id"`
}

type GetOrderInput struct {
	ID guid.GUID `json:"id"`
}

type DeleteProductInput struct {
	ID guid.GUID `json:"id"`
}

type ListRefundsInput struct {
	WebsiteID guid.GUID `json:"website_id"`
}

type CreateRefundInput struct {
	OrderID guid.GUID    `json:"order_id"`
	Reason  RefundReason `json:"reason"`
	Notes   string       `json:"notes"`
	Amount  int64        `json:"amount"`
}
