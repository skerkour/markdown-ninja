package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	stripecustomer "github.com/stripe/stripe-go/v81/customer"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/slicesx"
)

func (service *StoreService) PlaceOrder(ctx context.Context, input store.PlaceOrderInput) (output store.PlaceOrderOutput, err error) {
	logger := slogx.FromCtx(ctx)
	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname
	contactIsAuthenticated := false

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		return
	}

	if !service.rateLimiter.IsAllowed("StoreService.PlaceOrder", website.ID.Bytes(), httpCtx.Client.IP.AsSlice(), time.Hour, 30) {
		err = errs.TooManyRequests()
		return
	}

	customer := service.contactsService.CurrentContact(ctx)
	if customer == nil && input.Email == nil {
		err = errs.InvalidArgument("email is required")
		return
	} else if customer != nil && input.Email != nil {
		err = errs.InvalidArgument("email should not be provided when authenticated")
		return
	}

	if customer != nil {
		contactIsAuthenticated = true
	} else {
		var contact contacts.Contact
		email := strings.TrimSpace(strings.ToLower(*input.Email))
		err = service.kernel.ValidateEmail(ctx, email, false)
		if err != nil {
			return
		}
		contact, err = service.contactsService.FindOrCreateContact(ctx, service.db, website.ID, email, input.SubscribeToNewsletter)
		if err != nil {
			return
		}
		customer = &contact
	}

	productIDs := slicesx.Unique(input.Products)

	if len(productIDs) == 0 {
		err = store.ErrAtLeastOneProductIsRequiredForCheckout
		return
	}

	orderedProducts, err := service.repo.FindWebsiteProductsIn(ctx, service.db, website.ID, productIDs)
	if err != nil {
		return
	}

	if len(orderedProducts) != len(productIDs) {
		err = store.ErrProductNotFound
		return
	}

	var totalAmount int64
	for _, product := range orderedProducts {
		if product.Status != store.ProductStatusActive {
			err = store.ErrProductNotFound
			return
		}

		totalAmount += product.Price
	}

	orderID := guid.NewTimeBased()

	currency := website.Currency
	if httpCtx.Client.CountryCode == "US" {
		currency = websites.CurrencyUSD
	}

	stripeItems := make([]*stripe.CheckoutSessionLineItemParams, len(orderedProducts))
	for i, product := range orderedProducts {
		if product.Status != store.ProductStatusActive {
			err = store.ErrProductIsNotAvailable(product.Name)
			return
		}
		stripeItems[i] = &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				// Stripe uses lowercase codes for currencies. See stripe.CurrencyUSD for example.
				Currency: stripe.String(strings.ToLower(string(currency))),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(product.Name),
					Metadata: map[string]string{
						"markdown_ninja_product_id": product.ID.String(),
						"markdown_ninja_website_id": website.ID.String(),
						"markdown_ninja_order_id":   orderID.String(),
						"markdown_ninja_contact_id": customer.ID.String(),
					},
				},
				UnitAmount: stripe.Int64(product.Price * 100),
			},
			Quantity: stripe.Int64(1),
			AdjustableQuantity: &stripe.CheckoutSessionLineItemAdjustableQuantityParams{
				Enabled: stripe.Bool(true),
				Minimum: stripe.Int64(1),
			},
		}
	}

	var customerEmail *string
	var customerCreation *string
	var updateStripeCustomer *stripe.CheckoutSessionCustomerUpdateParams
	var stripeCustomerID *string
	var invoiceData *stripe.CheckoutSessionInvoiceCreationInvoiceDataParams
	var additionalInvoiceInformation string

	if input.AdditionalInvoiceInformation != nil {
		additionalInvoiceInformation = strings.TrimSpace(*input.AdditionalInvoiceInformation)
		if additionalInvoiceInformation != "" {
			err = validateAdditionalInvoiceInformation(additionalInvoiceInformation)
			if err != nil {
				return
			}
			invoiceData = &stripe.CheckoutSessionInvoiceCreationInvoiceDataParams{
				Description: stripe.String(additionalInvoiceInformation),
			}
		}
	}

	// metdata for the different stripe entities to help customer support solving issues
	stripeMetadata := map[string]string{
		"markdown_ninja_website_id": website.ID.String(),
		"markdown_ninja_order_id":   orderID.String(),
		"markdown_ninja_contact_id": customer.ID.String(),
		"country":                   httpCtx.Client.CountryCode,
	}

	// TODO: if an account already exists for this email, make the user authenticate before redirecting
	// to the payment page
	if contactIsAuthenticated {
		updateStripeCustomer = &stripe.CheckoutSessionCustomerUpdateParams{
			Name:    stripe.String("auto"),
			Address: stripe.String(string(stripe.CheckoutSessionBillingAddressCollectionAuto)),
		}

		if customer.StripeCustomerID != nil {
			stripeCustomerID = customer.StripeCustomerID
		} else {

			// create customer
			var stripeCustomer *stripe.Customer
			newStripeCustomerParams := &stripe.CustomerParams{
				Email:    &customer.Email,
				Metadata: stripeMetadata,
			}
			stripeCustomer, err = stripecustomer.New(newStripeCustomerParams)
			if err != nil {
				err = fmt.Errorf("store.PlaceOrder: error creating Stripe customer: %w", err)
				return
			}
			stripeCustomerID = &stripeCustomer.ID
		}
	} else {
		customerEmail = &customer.Email
		customerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationIfRequired))
	}

	createStripeCheckoutSessionParams := &stripe.CheckoutSessionParams{
		CustomerEmail:    customerEmail,
		Customer:         stripeCustomerID,
		CustomerCreation: customerCreation,
		CustomerUpdate:   updateStripeCustomer,
		// BillingAddressCollection: stripe.String(string(stripe.CheckoutSessionBillingAddressCollection)),
		// PaymentMethodTypes: []*string{
		// 	stripe.String(string(stripe.PaymentMethodTypeCard)),
		// 	// stripe.String(string(stripe.PaymentMethodTypePaypal)),
		// 	// stripe.String(string(stripe.PaymentMethodCardWalletTypeApplePay)),
		// 	// stripe.String(string(stripe.PaymentMethodCardWalletTypeGooglePay)),

		// 	// iDeal only workd for EUR payments
		// 	// stripe.String(string(stripe.PaymentMethodTypeIDEAL)),
		// 	// stripe.String(string(stripe.PaymentMethodTypeLink)),
		// },

		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: stripeMetadata,
		},
		LineItems:  stripeItems,
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(service.generateCompleteOrderUrl(website.PrimaryDomain, orderID)),
		CancelURL:  stripe.String(service.generateCancelOrderUrl(website.PrimaryDomain, orderID)),
		AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{
			Enabled: stripe.Bool(false),
		},
		InvoiceCreation: &stripe.CheckoutSessionInvoiceCreationParams{
			Enabled:     stripe.Bool(true),
			InvoiceData: invoiceData,
		},
		// CustomerUpdate: &stripe.CheckoutSessionCustomerUpdateParams{
		// 	Address: stripe.String("auto"),
		// },
		Metadata: stripeMetadata,
	}
	createStripeCheckoutSessionParams.AddExpand("payment_intent")
	stripeCheckoutSession, err := session.New(createStripeCheckoutSessionParams)
	if err != nil {
		errMessage := "store.CreateStripeCheckoutSession: creating stripe checkout session"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	now := time.Now().UTC()
	order := store.Order{
		ID:                           orderID,
		CreatedAt:                    now,
		UpdatedAt:                    now,
		TotalAmount:                  totalAmount,
		Currency:                     currency,
		Notes:                        "",
		Status:                       store.OrderStatusPending,
		CompletedAt:                  nil,
		CanceledAt:                   nil,
		Email:                        customer.Email,
		Country:                      httpCtx.Client.CountryCode,
		AdditionalInvoiceInformation: additionalInvoiceInformation,
		StripeCheckoutSessionID:      stripeCheckoutSession.ID,
		StripPaymentItentID:          nil,
		StripeInvoiceID:              nil,
		StripeInvoiceUrl:             nil,
		WebsiteID:                    website.ID,
		ContactID:                    customer.ID,
	}
	err = service.db.Transaction(ctx, func(tx db.Tx) (errTx error) {
		errTx = service.repo.CreateOrder(ctx, tx, order)
		if errTx != nil {
			return errTx
		}

		for _, product := range orderedProducts {
			// TODO: we need a way to adjust quantities before creating the stripe checkout session
			lineItem := store.OrderLineItem{
				ProductName:          product.Name,
				OriginalProductPrice: product.Price,
				Quantity:             1,
				OrderID:              order.ID,
				ProductID:            product.ID,
			}
			errTx = service.repo.CreateOrderLineItem(ctx, tx, lineItem)
			if errTx != nil {
				return errTx
			}
		}

		return nil
	})
	if err != nil {
		return
	}

	service.eventsService.TrackOrderPlaced(ctx, events.TrackOrderPlacedInput{
		OrderID:   order.ID,
		WebsiteID: order.WebsiteID,
		UserAgent: httpCtx.Client.UserAgent,
		Country:   httpCtx.Client.CountryCode,
	})

	output.StripeCheckoutUrl = stripeCheckoutSession.URL

	return
}
