package postmark

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Email struct {
	// From: REQUIRED The sender email address. Must have a registered and confirmed Sender Signature.
	From string
	// To: REQUIRED Recipient email address. Multiple addresses are comma separated. Max 50.
	To string
	// Cc recipient email address. Multiple addresses are comma separated. Max 50.
	Cc string `json:",omitempty"`
	// Bcc recipient email address. Multiple addresses are comma separated. Max 50.
	Bcc string `json:",omitempty"`
	// Subject: Email subject
	Subject string `json:",omitempty"`
	// Tag: Email tag that allows you to categorize outgoing emails and get detailed statistics.
	Tag string `json:",omitempty"`
	// HtmlBody: HTML email message. REQUIRED, If no TextBody specified
	HtmlBody string `json:",omitempty"`
	// TextBody: Plain text email message. REQUIRED, If no HtmlBody specified
	TextBody string `json:",omitempty"`
	// ReplyTo: Reply To override email address. Defaults to the Reply To set in the sender signature.
	ReplyTo string `json:",omitempty"`
	// Headers: List of custom headers to include.
	Headers []Header `json:",omitempty"`
	// TrackOpens: Activate open tracking for this email.
	TrackOpens bool `json:",omitempty"`
	// Attachments: List of attachments
	Attachments []Attachment `json:",omitempty"`
	// Metadata: Custom metadata key/value pairs.
	Metadata map[string]string `json:",omitempty"`
	// Set message stream ID that's used for sending. If not provided, message will default to the "outbound" transactional stream.
	MessageStream string
}

type Header struct {
	// Name: header name
	Name string
	// Value: header value
	Value string
}

type Attachment struct {
	// Name: attachment name
	Name string
	// Content: Base64 encoded attachment data
	Content string
	// ContentType: attachment MIME type
	ContentType string
	// ContentId: populate for inlining images with the images cid
	ContentID string `json:",omitempty"`
}

type EmailResponse struct {
	// To: Recipient email address
	To string
	// SubmittedAt: Timestamp
	SubmittedAt time.Time
	// MessageID: ID of message
	MessageID string
	// ErrorCode: API Error Codes
	ErrorCode int64
	// Message: Response message
	Message string
}

func (client *Client) SendEmail(ctx context.Context, serverToken string, email Email) (EmailResponse, error) {
	res := EmailResponse{}
	err := client.request(ctx, requestParams{
		Method:      http.MethodPost,
		URL:         "/email",
		Payload:     email,
		ServerToken: &serverToken,
	}, &res)

	if res.ErrorCode != 0 {
		return res, fmt.Errorf("%v %s", res.ErrorCode, res.Message)
	}

	return res, err
}

// TODO: handle individual errors in []EmailResponse?
func (client *Client) SendEmailsBatch(ctx context.Context, serverToken string, emails []Email) ([]EmailResponse, error) {
	res := []EmailResponse{}
	err := client.request(ctx, requestParams{
		Method:      http.MethodPost,
		URL:         "/email/batch",
		Payload:     emails,
		ServerToken: &serverToken,
	}, &res)

	return res, err
}
