package middlewares

import (
	"context"
	"net/http"
	"strings"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/pkg/server/auth"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/set"
)

type authMiddleware struct {
	webappDomain         string
	kernelService        kernel.PrivateService
	organizationsService organizations.Service
	contactsService      contacts.Service
	pingooClient         *pingoo.Client
}

var allowPathsWithoutAuth = set.NewFromSlice([]string{
	"/api/init",
	"/api/signup",
	"/api/complete_signup",
	"/api/login",
	"/api/complete_2fa_challenge",
})

// Auth is an HTTP middleware that checks authentication and return an error code if
// some credentials are present but not valid.
// for the webapp domain it checks the users auth cookie and the `Authorization` header (for API keys)
// for the websites domaians it checks the contacts auth cookie
// if the authentication is successful then the autenticated entity is injected into the request's context
// no rate-limiting is performed by the Auth middleware. Rate-limiting should be performed by dowstream
// services.
func Auth(webappDomain string, kernelService kernel.PrivateService, organizationsService organizations.Service,
	contactsService contacts.Service, pingooClient *pingoo.Client) func(next http.Handler) http.Handler {
	authMiddleware := &authMiddleware{
		webappDomain:         webappDomain,
		kernelService:        kernelService,
		organizationsService: organizationsService,
		contactsService:      contactsService,
		pingooClient:         pingooClient,
	}
	return authMiddleware.Middleware
}

func (middleware *authMiddleware) Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		httpCtx := httpctx.FromCtx(ctx)
		hostname := httpCtx.Hostname
		var err error

		if hostname == middleware.webappDomain {
			err = middleware.handleWebappAuth(ctx, w, req)
		} else {
			err = middleware.handleWebsiteAuth(ctx, w, req)
		}
		if err != nil {
			return
		}

		next.ServeHTTP(w, req.WithContext(ctx))
	}

	return http.HandlerFunc(fn)
}

func (middleware *authMiddleware) handleWebappAuth(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	// as of now, we only need to autenticate api requests as there is no server-side rendering
	// for the webapp that would require authentication
	// also, checking auth with a SPA may cause race conditions where an inflight requests for theme's assets
	// hits the auth middleware just before the sessions token has been refreshed on app load
	if !strings.HasPrefix(req.URL.Path, "/api") {
		return nil
	}

	httpCtx := httpctx.FromCtx(ctx)

	// if Authorization header is set
	authHeader := req.Header.Get(kernel.AuthHttpHeader)
	if authHeader != "" {
		tokenType, token, err := decodeAuthorizationHeader(authHeader)
		if err != nil {
			apiutil.SendError(ctx, w, err)
			return err
		}

		switch tokenType {
		case "apikey":
			apiKey, err := middleware.organizationsService.VerifyApiKey(ctx, token)
			if err != nil {
				apiutil.SendError(ctx, w, err)
				return err
			}

			httpCtx.ApiKey = &apiKey
			return nil
		case "bearer":
			accessToken, err := pingoo.VerifyJWT[auth.AccessToken](ctx, middleware.pingooClient, token)
			if err != nil {
				apiutil.SendError(ctx, w, err)
				return err
			}
			httpCtx.AccessToken = &accessToken
		default:
			err = errs.InvalidArgument("Authorization header is not valid")
			apiutil.SendError(ctx, w, err)
			return err
		}

		return nil
	}

	if allowPathsWithoutAuth.Contains(req.URL.Path) || strings.HasPrefix(req.URL.Path, "/api/webhooks/") {
		return nil
	}

	middleware.kernelService.SleepAuth()
	apiutil.SendError(ctx, w, kernel.ErrApiRequiresAuthentication)
	return kernel.ErrApiRequiresAuthentication
}

func (middleware *authMiddleware) handleWebsiteAuth(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	// we don't need to authenticate requests to static theme assets
	if strings.HasPrefix(req.URL.Path, "/theme/") {
		return nil
	}

	// we can ignore error as req.Cookie will returns an error only if the cookie is missing
	authCookie, _ := req.Cookie(contacts.AuthCookie)
	if authCookie != nil {
		httpCtx := httpctx.FromCtx(ctx)

		contactAndSession, err := middleware.contactsService.VerifySessionToken(ctx, strings.TrimSpace(authCookie.Value))
		if err != nil {
			if errs.IsInternal(err) {
				// TODO: if not a __markdown_ninja route: send page internal error instead of api response
				apiutil.SendError(ctx, w, err)
				return err
			} else {
				// handle invalid/expired sessions
				logoutCoookie := middleware.contactsService.GenerateLogoutCookie()
				http.SetCookie(w, &logoutCoookie)
				return nil
			}
		}

		httpCtx.Contact = &contactAndSession.Contact
		httpCtx.ContactSession = &contactAndSession.Session
		return nil
	}

	return nil
}

func decodeAuthorizationHeader(header string) (tokenType, token string, err error) {
	header = strings.TrimSpace(header)
	parts := strings.Split(header, " ")
	if len(parts) != 2 {
		err = kernel.ErrApiKeyIsNotValid
		return
	}
	tokenType = strings.ToLower(parts[0])
	token = parts[1]
	return
}
