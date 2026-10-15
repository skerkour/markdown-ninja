package service

import (
	"context"
	"encoding/json"
	"fmt"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/stdx/uuid"
)

type pingooWebhookDataDeleteUser struct {
	ID uuid.UUID `json:"id"`
}

func (service *KernelService) HandlePingooWebhook(ctx context.Context, event pingoo.Event) (err error) {
	switch event.Type {
	case "auth.delete_user":
		return service.handlePingooEventDeleteUser(ctx, event)
	}

	return nil
}

func (service *KernelService) handlePingooEventDeleteUser(ctx context.Context, event pingoo.Event) error {
	var eventData pingooWebhookDataDeleteUser

	err := json.Unmarshal(event.Data, &eventData)
	if err != nil {
		return fmt.Errorf("kernel.handlePingooEventDeleteuser: error unmarshalling event data: %w", err)
	}

	orgs, err := service.organizationsService.ListOrganizationsForUser(ctx, service.db, eventData.ID)
	if err != nil {
		return err
	}

	if len(orgs) != 0 {
		return errs.InvalidArgument("Please leave or delete all your organizations before deleting your account.")
	}

	return nil
}
