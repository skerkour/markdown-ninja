package postmark

// import (
// 	"fmt"
// 	"net/url"
// )

// type SenderSignature struct {
// 	Domain              string
// 	EmailAddress        string
// 	ReplyToEmailAddress string
// 	Name                string
// 	Confirmed           bool
// 	ID                  int64
// }

// type SenderSignaturesList struct {
// 	TotalCount       int
// 	SenderSignatures []SenderSignature
// }

// type DetailedSenderSignature struct {
// 	// Domain associated with sender signature.
// 	Domain string
// 	// string of objects that each represent a sender signature.
// 	EmailAddress string
// 	// Reply-To email associated with sender signature.
// 	ReplyToEmailAddress string
// 	// From name of sender signature.
// 	Name string
// 	// Indicates whether or not this sender signature has been confirmed.
// 	Confirmed bool
// 	// Unique ID of sender signature.
// 	ID int64
// 	// DEPRECATED: See our [blog post](https://postmarkapp.com/blog/why-we-no-longer-ask-for-spf-records) to learn why this field was deprecated.
// 	SPFVerified bool
// 	// Host name used for the SPF configuration.
// 	SPFHost string
// 	// Value that must be setup at your domain registrar or DNS host in order for SPF to function correctly.
// 	SPFTextValue string
// 	// DKIM DNS text record has been setup correctly at your domain registrar or DNS host.
// 	DKIMVerified bool
// 	// DKIM is using a strength weaker than 1024 bit. If so, it’s possible to request a new DKIM using the RequestNewDKIM function below.
// 	WeakDKIM bool
// 	// DNS TXT host being used to validate messages sent in.
// 	DKIMHost string
// 	// DNS TXT value being used to validate messages sent in.
// 	DKIMTextValue string
// 	// If a DKIM renewal has been intiated or this DKIM is from a new Sender Signature, this field will show the pending DKIM DNS TXT host which has yet to be setup and confirmed at your registrar or DNS host.
// 	DKIMPendingHost string
// 	// Similar to the DKIMPendingHost field, this will show the DNS TXT value waiting to be confirmed at your registrar or DNS host.
// 	DKIMPendingTextValue string
// 	// Once a new DKIM has been confirmed at your registrar or DNS host, Postmark will revoke the old DKIM host in preparation for removing it permantly from the system.
// 	DKIMRevokedHost string
// 	// Similar to DKIMRevokedHost, this field will show the DNS TXT value that will soon be removed from the Postmark system.
// 	DKIMRevokedTextValue string
// 	// Indicates whether you may safely delete the old DKIM DNS TXT records at your registrar or DNS host. The new DKIM is now safely in use.
// 	SafeToRemoveRevokedKeyFromDNS bool
// 	// While DKIM renewal or new DKIM operations are being conducted or setup, this field will indicate Pending. After all DNS TXT records are up to date and any pending renewal operations are finished, it will indicate Verified.
// 	DKIMUpdateStatus string
// 	// The custom Return-Path domain for this signature. For more information about this field, please read [our support page](http://support.postmarkapp.com/article/910-adding-a-custom-return-path-domain).
// 	ReturnPathDomain string
// 	// The verification state of the Return-Path domain. Tells you if the Return-Path is actively being used or still needs further action to be used.
// 	ReturnPathDomainVerified bool
// 	// The CNAME DNS record that Postmark expects to find at the ReturnPathDomain value.
// 	ReturnPathDomainCNAMEValue string
// }

// func (client *Client) GetSenderSignatures(count, offset int64) (SenderSignaturesList, error) {
// 	res := SenderSignaturesList{}

// 	values := &url.Values{}
// 	values.Add("count", fmt.Sprintf("%d", count))
// 	values.Add("offset", fmt.Sprintf("%d", offset))

// 	err := client.request(requestParams{
// 		Method: "http.MethodGet",
// 		URL:    fmt.Sprintf("/senders?%s", values.Encode()),
// 	}, &res)
// 	return res, err
// }
