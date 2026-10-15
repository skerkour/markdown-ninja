package service

import (
	"context"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/uuid"
)

func (service *KernelService) CurrentUserID(ctx context.Context) (userID uuid.UUID, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	if httpCtx == nil || httpCtx.AccessToken == nil {
		err = kernel.ErrAuthenticationRequired
		return
	}

	return httpCtx.AccessToken.UserID, nil
}
