package service

import (
	"context"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/markdown"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) CreateProductPage(ctx context.Context, input store.CreateProductPageInput) (page store.ProductPage, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	// for now only admins can create product pages
	if !httpCtx.AccessToken.IsAdmin {
		err = kernel.ErrPermissionDenied
		return
	}

	product, err := service.repo.FindProductByID(ctx, service.db, input.ProductID)
	if err != nil {
		return
	}

	website, err := service.websitesService.FindWebsiteByID(ctx, service.db, product.WebsiteID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, product.WebsiteID)
	if err != nil {
		return
	}

	// if product.Type != store.ProductTypeCourse {
	// 	err = store.ErrProductIsNotACourse
	// 	return
	// }

	now := time.Now().UTC()
	title := strings.TrimSpace(input.Title)

	bodyMarkdown := input.BodyMarkdown
	err = service.validateProductPageContent(bodyMarkdown)
	if err != nil {
		return
	}

	size := int64(len(input.BodyMarkdown))
	bodyHash := blake3.Sum256([]byte(bodyMarkdown))
	_, err = markdown.ToHtmlPage(
		bodyMarkdown,
		service.httpConfig.WebsitesBaseUrl.Scheme+"://"+website.PrimaryDomain+service.httpConfig.WebsitesPort,
	)
	if err != nil {
		return
	}

	err = service.validateProductPageTitle(title)
	if err != nil {
		return
	}

	position, err := service.repo.GetProductPagesCountForProduct(ctx, service.db, product.ID)
	if err != nil {
		return
	}

	page = store.ProductPage{
		ID:           guid.NewTimeBased(),
		CreatedAt:    now,
		UpdatedAt:    now,
		Position:     position,
		Title:        title,
		Size:         size,
		Hash:         bodyHash[:],
		BodyMarkdown: bodyMarkdown,
		ProductID:    product.ID,
	}
	err = service.repo.CreateProductPage(ctx, service.db, page)
	if err != nil {
		return
	}

	return
}
