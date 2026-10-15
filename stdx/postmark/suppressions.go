package postmark

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type SuppressionReason string

const (
	SuppressionReasonHardBounce        SuppressionReason = "HardBounce"
	SuppressionReasonSpamComplaint     SuppressionReason = "SpamComplaint"
	SuppressionReasonManualSuppression SuppressionReason = "ManualSuppression"
)

type SuppressionOrigin string

const (
	SuppressionOriginRecipient SuppressionOrigin = "Recipient"
	SuppressionOriginCustomer  SuppressionOrigin = "Customer"
	SuppressionOriginAdmin     SuppressionOrigin = "Admin"
)

type SuppressionDeleteStatus string

const (
	SuppressionDeleteStatusFailed  SuppressionDeleteStatus = "Failed"
	SuppressionDeleteStatusDeleted SuppressionDeleteStatus = "Deleted"
)

type GetSuppressionsOptions struct {
	SuppressionReason *SuppressionReason
	FromDate          *time.Time
	ToDate            *time.Time
	Origin            *SuppressionOrigin
	EmailAddress      *string
}

type SuppressionsList struct {
	Suppressions []Suppression `json:"Suppressions"`
}

type Suppression struct {
	EmailAddress      string            `json:"EmailAddress"`
	SuppressionReason SuppressionReason `json:"SuppressionReason"`
	Origin            SuppressionOrigin `json:"Origin"`
	CreatedAt         time.Time         `json:"CreatedAt"`
}

type DeleteSuppressionsRequest struct {
	Suppressions []DeleteSuppressionInput `json:"Suppressions"`
}

type DeleteSuppressionInput struct {
	EmailAddress string `json:"EmailAddress"`
}

type DeleteSuppressionsResponse struct {
	Suppressions []DeleteSuppressionResponse `json:"Suppressions"`
}

type DeleteSuppressionResponse struct {
	EmailAddress string                  `json:"EmailAddress"`
	Status       SuppressionDeleteStatus `json:"Status"`
	Message      *string                 `json:"Message"`
}

// GetSuppressions fetches the Suppressions for a given message stream.
// See https://postmarkapp.com/developer/api/suppressions-api
func (client *Client) GetSuppressions(ctx context.Context, serverToken, messageStreamID string, options *GetSuppressionsOptions) (SuppressionsList, error) {
	res := SuppressionsList{}
	var queryStringParameters string

	if options != nil {
		values := url.Values{}

		if options.SuppressionReason != nil {
			values.Add("SuppressionReason", string(*options.SuppressionReason))
		}
		if options.Origin != nil {
			values.Add("Origin", string(*options.Origin))
		}
		if options.ToDate != nil {
			values.Add("todate", options.ToDate.Format(time.DateOnly))
		}
		if options.FromDate != nil {
			values.Add("fromdate", options.FromDate.Format(time.DateOnly))
		}
		if options.EmailAddress != nil {
			values.Add("EmailAddress", *options.EmailAddress)
		}

		queryStringParameters = "?" + values.Encode()
	}

	err := client.request(ctx, requestParams{
		Method:      http.MethodGet,
		URL:         fmt.Sprintf("/message-streams/%s/suppressions/dump%s", messageStreamID, queryStringParameters),
		ServerToken: &serverToken,
	}, &res)

	return res, err
}

// DeleteSuppressions deletes many Suppressions for the given message stream.
// See https://postmarkapp.com/developer/api/suppressions-api#delete-a-suppression
func (client *Client) DeleteSuppressions(ctx context.Context, serverToken, messageStreamID string, input DeleteSuppressionsRequest) (res DeleteSuppressionsResponse, err error) {
	err = client.request(ctx, requestParams{
		Method:      http.MethodPost,
		URL:         fmt.Sprintf("/message-streams/%s/suppressions/delete", messageStreamID),
		ServerToken: &serverToken,
		Payload:     input,
	}, &res)

	return res, err
}
