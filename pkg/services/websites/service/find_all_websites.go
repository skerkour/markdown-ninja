package service

import (
	"context"
	"math"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
)

func (service *WebsitesService) FindAllWebsites(ctx context.Context, db db.Queryer) (websites []websites.Website, err error) {
	return service.repo.FindWebsites(ctx, service.db, math.MaxInt64)
}
