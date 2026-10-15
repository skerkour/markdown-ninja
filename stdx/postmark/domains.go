package postmark

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type Domain struct {
	// Unique ID of the Domain.
	ID int64
	// Domain name
	Name string
	// DEPRECATED: See our [blog post](https://postmarkapp.com/blog/why-we-no-longer-ask-for-spf-records) to learn why this field was deprecated.
	SPFVerified bool
	// DKIM DNS text record has been setup correctly at your domain registrar or DNS host.
	DKIMVerified bool
	// DKIM is using a strength weaker than 1024 bit. If so, it’s possible to request a new DKIM using the RequestNewDKIM function below.
	WeakDKIM bool
	// The verification state of the Return-Path domain. Tells you if the Return-Path is actively being used or still needs further action to be used.
	ReturnPathDomainVerified bool
}

type DomainsList struct {
	TotalCount int
	Domains    []Domain
}

type DetailedDomain struct {
	// Unique ID of the Domain.
	ID int64
	// Domain name
	Name string
	// DEPRECATED: See our [blog post](https://postmarkapp.com/blog/why-we-no-longer-ask-for-spf-records) to learn why this field was deprecated.
	SPFVerified bool
	// Host name used for the SPF configuration.
	SPFHost string
	// Value that must be setup at your domain registrar or DNS host in order for SPF to function correctly.
	SPFTextValue string
	// DKIM DNS text record has been setup correctly at your domain registrar or DNS host.
	DKIMVerified bool
	// DKIM is using a strength weaker than 1024 bit. If so, it’s possible to request a new DKIM using the RequestNewDKIM function below.
	WeakDKIM bool
	// DNS TXT host being used to validate messages sent in.
	DKIMHost string
	// DNS TXT value being used to validate messages sent in.
	DKIMTextValue string
	// If a DKIM rotation has been intiated or this DKIM is from a new Domain, this field will show the pending DKIM DNS TXT host which has yet to be setup and confirmed at your registrar or DNS host.
	DKIMPendingHost string
	// Similar to the DKIMPendingHost field, this will show the DNS TXT value waiting to be confirmed at your registrar or DNS host.
	DKIMPendingTextValue string
	// Once a new DKIM has been confirmed at your registrar or DNS host, Postmark will revoke the old DKIM host in preparation for removing it permantly from the system.
	DKIMRevokedHost string
	// Similar to DKIMRevokedHost, this field will show the DNS TXT value that will soon be removed from the Postmark system.
	DKIMRevokedTextValue string
	// Indicates whether you may safely delete the old DKIM DNS TXT records at your registrar or DNS host. The new DKIM is now safely in use.
	SafeToRemoveRevokedKeyFromDNS bool
	// While DKIM renewal or new DKIM operations are being conducted or setup, this field will indicate Pending. After all DNS TXT records are up to date and any pending renewal operations are finished, it will indicate Verified.
	DKIMUpdateStatus string
	// The custom Return-Path for this domain, please read our support page.
	ReturnPathDomain string
	// The verification state of the Return-Path domain. Tells you if the Return-Path is actively being used or still needs further action to be used.
	ReturnPathDomainVerified bool
	// The CNAME DNS record that Postmark expects to find at the ReturnPathDomain value.
	ReturnPathDomainCNAMEValue string
}

type CreateDomainInput struct {
	// Domain name
	Name string
	// A custom value for the Return-Path domain. It is an optional field, but it must be a subdomain of your From Email domain and must have a CNAME record that points to pm.mtasv.net. For more information about this field, please read our support page.
	ReturnPathDomain string `json:",omitempty"`
}

type UpdateDomainInput struct {
	// A custom value for the Return-Path domain. It is an optional field, but it must be a subdomain of your From Email domain and must have a CNAME record that points to pm.mtasv.net. For more information about this field, please read our support page.
	ReturnPathDomain string `json:",omitempty"`
}

func (client *Client) GetDomains(ctx context.Context, count, offset int64) (DomainsList, error) {
	res := DomainsList{}

	values := url.Values{}
	values.Add("count", fmt.Sprintf("%d", count))
	values.Add("offset", fmt.Sprintf("%d", offset))

	err := client.request(ctx, requestParams{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("/domains?%s", values.Encode()),
	}, &res)

	return res, err
}

func (client *Client) GetDomain(ctx context.Context, domainID string) (DetailedDomain, error) {
	res := DetailedDomain{}
	err := client.request(ctx, requestParams{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("/domains/%s", domainID),
	}, &res)

	return res, err
}

func (client *Client) CreateDomain(ctx context.Context, input CreateDomainInput) (DetailedDomain, error) {
	res := DetailedDomain{}
	err := client.request(ctx, requestParams{
		Method:  http.MethodPost,
		URL:     "/domains",
		Payload: input,
	}, &res)

	return res, err
}

func (client *Client) UpdateDomain(ctx context.Context, domainID string, input UpdateDomainInput) (DetailedDomain, error) {
	res := DetailedDomain{}
	err := client.request(ctx, requestParams{
		Method:  http.MethodPut,
		URL:     fmt.Sprintf("/domains/%s", domainID),
		Payload: input,
	}, &res)

	return res, err
}

func (client *Client) DeleteDomain(ctx context.Context, domainID string) error {
	err := client.request(ctx, requestParams{
		Method: http.MethodDelete,
		URL:    fmt.Sprintf("/domains/%s", domainID),
	}, nil)

	return err
}

func (client *Client) VerifyDKIMStatus(ctx context.Context, domainID string) (DetailedDomain, error) {
	res := DetailedDomain{}
	err := client.request(ctx, requestParams{
		Method: http.MethodPut,
		URL:    fmt.Sprintf("/domains/%s/verifyDkim", domainID),
	}, &res)

	return res, err
}

func (client *Client) VerifyReturnPathStatus(ctx context.Context, domainID string) (DetailedDomain, error) {
	res := DetailedDomain{}
	err := client.request(ctx, requestParams{
		Method: http.MethodPut,
		URL:    fmt.Sprintf("/domains/%s/verifyReturnPath", domainID),
	}, &res)

	return res, err
}
