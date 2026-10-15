package httpctx

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"

	"markdown.ninja/pkg/server/auth"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/uuid"
)

// Context is used to carry data during the lifecycle of a request
type Context struct {
	AccessToken *auth.AccessToken
	// ApiKey is present only when authenticating with an API Key
	ApiKey *organizations.ApiKey

	// Contacts is present only when visiting a website and a contact is authenticated with cookies
	Contact        *contacts.Contact
	ContactSession *contacts.Session

	RequestID uuid.UUID
	Hostname  string
	Headers   http.Header
	// Url is always present but a pointer because http.Request.URL is a pointer
	Url    *url.URL
	Client ClientData

	Response Response
	Request  Request
}

type ClientData struct {
	IPStr       string
	IP          netip.Addr
	ASN         int64
	CountryCode string
	UserAgent   string
}

type Response struct {
	Headers  http.Header
	Cookies  []http.Cookie
	CacheHit *CacheHit
}

type Request struct {
	IfNoneMatch *string
}

type CacheHit struct {
	CacheControl string
	ETag         string
}

// httpCtxKey type to use when setting the Context
type httpCtxKey struct{}

// CtxKey is the key that holds the unique Context in a request context.
var CtxKey httpCtxKey = httpCtxKey{}

func FromCtx(ctx context.Context) *Context {
	httpCtx, ok := ctx.Value(CtxKey).(*Context)
	if !ok {
		logger := slogx.FromCtx(ctx)
		logger.Error("httpctx.FromCtx: error getting httpCtx from context")
		return nil
	}
	return httpCtx
}
