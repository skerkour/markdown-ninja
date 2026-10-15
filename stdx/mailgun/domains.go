package mailgun

import (
	"context"
	"fmt"
	"net/http"
)

type SpamAction string

// Use these to specify a spam action when creating a new domain.
const (
	// Tag the received message with headers providing a measure of its spamness.
	SpamActionTag = SpamAction("tag")
	// Prevents Mailgun from taking any action on what it perceives to be spam.
	SpamActionDisabled = SpamAction("disabled")
	// instructs Mailgun to just block or delete the message all-together.
	SpamActionDelete = SpamAction("delete")

	DKIMKeySize1024 = 1024
	DKIMKeySize2048 = 2048
)

// A Domain structure holds information about a domain used when sending mail.
type Domain struct {
	CreatedAt    RFC2822Time `json:"created_at"`
	SMTPLogin    string      `json:"smtp_login"`
	Name         string      `json:"name"`
	SMTPPassword string      `json:"smtp_password"`
	Wildcard     bool        `json:"wildcard"`
	SpamAction   SpamAction  `json:"spam_action"`
	State        string      `json:"state"`
}

// DNSRecord structures describe intended records to properly configure your domain for use with Mailgun.
// Note that Mailgun does not host DNS records.
type DNSRecord struct {
	Priority   string
	RecordType string `json:"record_type"`
	Valid      string
	Name       string
	Value      string
}

type DomainResponse struct {
	Domain              Domain      `json:"domain"`
	ReceivingDNSRecords []DNSRecord `json:"receiving_dns_records"`
	SendingDNSRecords   []DNSRecord `json:"sending_dns_records"`
}

type domainConnectionResponse struct {
	Connection DomainConnection `json:"connection"`
}

type domainsListResponse struct {
	// is -1 if Next() or First() have not been called
	TotalCount int      `json:"total_count"`
	Items      []Domain `json:"items"`
}

// Specify the domain connection options
type DomainConnection struct {
	RequireTLS       bool `json:"require_tls"`
	SkipVerification bool `json:"skip_verification"`
}

// Specify the domain tracking options
type DomainTracking struct {
	Click       TrackingStatus `json:"click"`
	Open        TrackingStatus `json:"open"`
	Unsubscribe TrackingStatus `json:"unsubscribe"`
}

// The tracking status of a domain
type TrackingStatus struct {
	Active     bool   `json:"active"`
	HTMLFooter string `json:"html_footer"`
	TextFooter string `json:"text_footer"`
}

type domainTrackingResponse struct {
	Tracking DomainTracking `json:"tracking"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Requests
////////////////////////////////////////////////////////////////////////////////////////////////////

// Optional parameters when creating a domain
type CreateDomainInput struct {
	Name               string      `json:"name"`
	SMTPPassword       *string     `json:"smtp_password"`
	SpamAction         *SpamAction `json:"spam_action"`
	Wildcard           *bool       `json:"wildcard"`
	ForceDKIMAuthority *bool       `json:"force_dkim_authority"`
	DKIMKeySize        *int        `json:"dkim_key_size"`
	IPS                []string    `json:"ips"`
	PoolID             *string     `json:"pool_id"`
	WebScheme          *string     `json:"web_scheme"`
}

type UpdateDomainDKIMSelectorInput struct {
	Domain       string `json:"-"`
	DKIMSelector string `json:"dkim_selector"`
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// API Methods
////////////////////////////////////////////////////////////////////////////////////////////////////

func (client *Client) CreateDomain(ctx context.Context, input CreateDomainInput) (res Domain, err error) {
	err = client.request(ctx, requestParams{
		Payload: input,
		Method:  http.MethodPost,
		URL:     "/v4/domains",
	}, &res)

	return
}

func (client *Client) DeleteDomain(ctx context.Context, domain string) (err error) {
	err = client.request(ctx, requestParams{
		Payload: nil,
		Method:  http.MethodDelete,
		URL:     fmt.Sprintf("/v4/domains/%s", domain),
	}, nil)

	return
}

func (client *Client) UpdateDomainDKIMSelector(ctx context.Context, input UpdateDomainDKIMSelectorInput) (err error) {
	err = client.request(ctx, requestParams{
		Payload: input,
		Method:  http.MethodPut,
		URL:     fmt.Sprintf("/v4/domains/%s/dkim_selector", input.Domain),
	}, nil)

	return
}

func (client *Client) GetDomain(ctx context.Context, domain string) (res DomainResponse, err error) {
	err = client.request(ctx, requestParams{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("/v4/domains/%s", domain),
	}, &res)

	return
}
