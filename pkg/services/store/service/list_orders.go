package service

import (
	"context"
	"strings"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) ListOrders(ctx context.Context, input store.ListOrdersInput) (ret kernel.PaginatedResult[store.OrderMetadata], err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return ret, err
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return ret, err
	}

	limit := input.Limit
	if limit < 0 {
		return ret, errs.InvalidArgument("limit is not valid")
	} else if limit > 1000 {
		return ret, errs.InvalidArgument("limit is too high. max: 1000")
	} else if limit == 0 {
		limit = 100 // default value
	}

	searchQuery := strings.TrimSpace(input.Query)
	if searchQuery != "" {
		var order store.Order
		orderID, _ := guid.Parse(searchQuery)
		order, err = service.repo.FindOrderByID(ctx, service.db, orderID, false)
		if err != nil {
			return ret, err
		}
		ret.Data = []store.OrderMetadata{convertOrderToMetadata(order)}
	} else {
		ret.Data, err = service.repo.FindOrdersMetadataForWebsite(ctx, service.db, input.WebsiteID, limit, input.After)
		if err != nil {
			return ret, err
		}
	}

	return ret, nil
}
