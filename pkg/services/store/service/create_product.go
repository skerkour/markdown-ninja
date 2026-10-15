package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) CreateProduct(ctx context.Context, input store.CreateProductInput) (product store.Product, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	// for now only admins can create products
	if !httpCtx.AccessToken.IsAdmin {
		err = kernel.ErrPermissionDenied
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	productType := input.Type
	price := input.Price

	err = service.validateProductName(name)
	if err != nil {
		return
	}

	err = service.validateProductDescription(description)
	if err != nil {
		return
	}

	err = service.validateProductType(productType)
	if err != nil {
		return
	}

	err = service.validateProductPrice(price)
	if err != nil {
		return
	}

	product = store.Product{
		ID:          guid.NewTimeBased(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Name:        name,
		Description: description,
		Type:        productType,
		Status:      store.ProductStatusDraft,
		Price:       price,
		WebsiteID:   input.WebsiteID,
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreateProduct(ctx, tx, product)
		if txErr != nil {
			return txErr
		}

		product.Content, txErr = service.initProductContent(ctx, tx, product)
		if txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return
	}

	return
}

func (service *StoreService) initProductContent(ctx context.Context, tx db.Queryer, product store.Product) (pages []store.ProductPage, err error) {
	now := time.Now().UTC()

	bodyMarkdown := fmt.Sprintf("Thank you for purchasing %s!", product.Name)
	size := int64(len(bodyMarkdown))
	bodyHash := blake3.Sum256([]byte(bodyMarkdown))

	firstPage := store.ProductPage{
		ID:           guid.NewTimeBased(),
		CreatedAt:    now,
		UpdatedAt:    now,
		Position:     0,
		Title:        "Thank You!",
		Size:         size,
		Hash:         bodyHash[:],
		BodyMarkdown: bodyMarkdown,
		ProductID:    product.ID,
	}
	err = service.repo.CreateProductPage(ctx, tx, firstPage)
	if err != nil {
		return
	}

	pages = []store.ProductPage{firstPage}

	return
}
