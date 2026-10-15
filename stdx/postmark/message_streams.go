package postmark

import (
	"context"
	"fmt"
	"net/http"
)

const (
	DefaultBroadcastStreamID     = "broadcast"
	DefaultInboundStreamID       = "inbound"
	DefaultTransactionalStreamID = "outbound"
)

type MessageStreamType string

const (
	MessageStreamTypeInbound       MessageStreamType = "Inbound"
	MessageStreamTypeBroadcasts    MessageStreamType = "Broadcasts"
	MessageStreamTypeTransactional MessageStreamType = "Transactional"
)

type MessageStreamUnsubscribeHandlingType string

const (
	MessageStreamUnsubscribeHandlingPostmark MessageStreamUnsubscribeHandlingType = "Postmark"
	MessageStreamUnsubscribeHandlingCustom   MessageStreamUnsubscribeHandlingType = "Custom"
	MessageStreamUnsubscribeHandlingNone     MessageStreamUnsubscribeHandlingType = "none"
)

type MessageStreamSubscriptionManagementConfiguration struct {
	UnsubscribeHandlingType MessageStreamUnsubscribeHandlingType `json:"UnsubscribeHandlingType"`
}

// See https://postmarkapp.com/developer/api/message-streams-api
type MessageStream struct {
	ID                string            `json:"ID"`
	ServerID          int64             `json:"ServerID"`
	Name              string            `json:"Name"`
	Description       string            `json:"Description"`
	MessageStreamType MessageStreamType `json:"MessageStreamType"`
	// 	"CreatedAt": "2020-07-02T00:00:00-04:00",
	// 	"UpdatedAt": "2020-07-03T00:00:00-04:00",
	// 	"ArchivedAt": null,
	// 	"ExpectedPurgeDate": null,
	SubscriptionManagementConfiguration MessageStreamSubscriptionManagementConfiguration `json:"SubscriptionManagementConfiguration"`
}

type UpdateMessageStreamInput struct {
	Name                                string                                           `json:"Name,omitempty"`
	Description                         string                                           `json:"Description,omitempty"`
	SubscriptionManagementConfiguration MessageStreamSubscriptionManagementConfiguration `json:"SubscriptionManagementConfiguration,omitempty"`
}

func (client *Client) UpdateMessageStream(ctx context.Context, serverToken, messageStreamID string, input UpdateMessageStreamInput) (res MessageStream, err error) {
	err = client.request(ctx, requestParams{
		Method:      http.MethodPatch,
		URL:         fmt.Sprintf("/message-streams/%s", messageStreamID),
		Payload:     input,
		ServerToken: &serverToken,
	}, &res)

	return
}
