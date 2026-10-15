package website

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/httpx/cors"
)

func Routes(ctx context.Context, siteService site.Service, contactsService contacts.Service, storeService store.Service) (router chi.Router) {
	router = chi.NewRouter()
	// server := websitesServer{
	// 	siteService,
	// }

	cors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodOptions,
		},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Origin", "Set-Cookie", "Cookie"},
		ExposedHeaders:   []string{},
		AllowCredentials: false,
		MaxAge:           3600,
	})
	router.Use(cors.Handler)

	router.Route(websites.MarkdownNinjaPathPrefix, func(mdninjaRouter chi.Router) {
		// mdninjaRouter.Get("/videos/{asset_id}/iframe", siteService.ServeVideoIframe)
		mdninjaRouter.Get("/preview/{page_id}", siteService.ServePreview)

		mdninjaRouter.Route("/api", func(apiRouter chi.Router) {
			apiRouter.Use(middleware.NoCache)

			apiRouter.Get("/", apiutil.IndexHandler)
			// CMS
			apiRouter.Get("/website", apiutil.GetEndpoint(siteService.GetWebsite))
			apiRouter.Get("/page", apiutil.GetEndpoint(siteService.GetPage))
			apiRouter.Get("/tags", apiutil.GetEndpoint(siteService.ListTags))
			apiRouter.Get("/pages", apiutil.GetEndpoint(siteService.ListPages))

			// Contacts
			apiRouter.Get("/me", apiutil.GetEndpoint(siteService.GetMe))
			apiRouter.Post("/login", apiutil.JsonEndpoint(siteService.Login))
			apiRouter.Post("/complete_login", apiutil.JsonEndpoint(siteService.CompleteLogin))
			apiRouter.Post("/logout", apiutil.JsonEndpointOk(siteService.Logout))
			apiRouter.Post("/subscribe", apiutil.JsonEndpoint(siteService.Subscribe))
			apiRouter.Post("/complete_subscription", apiutil.JsonEndpoint(siteService.CompleteSubscription))
			apiRouter.Post("/unsubscribe", apiutil.JsonEndpointOk(siteService.Unsubscribe))
			apiRouter.Post("/update_my_account", apiutil.JsonEndpoint(siteService.UpdateMyAccount))
			apiRouter.Post("/verify_email", apiutil.JsonEndpointOk(contactsService.VerifyEmail))
			apiRouter.Post("/delete_my_account", apiutil.JsonEndpointOk(siteService.DeleteMyAccount))

			// store
			apiRouter.Post("/place_order", apiutil.JsonEndpoint(storeService.PlaceOrder))
			apiRouter.Post("/complete_order", apiutil.JsonEndpointOk(storeService.CompleteOrder))
			apiRouter.Post("/cancel_order", apiutil.JsonEndpointOk(storeService.CancelOrder))
			apiRouter.Get("/my_orders", apiutil.GetEndpoint(siteService.ListMyOrders))
			apiRouter.Get("/my_products", apiutil.GetEndpoint(siteService.ListMyProducts))
			// TODO: productID as URL param instead of query param?
			apiRouter.Get("/product", apiutil.GetEndpoint(siteService.GetProduct))

			// events
			apiRouter.Post("/events/page_view", apiutil.JsonEndpointOk(siteService.TrackEventPageView))
		})

		mdninjaRouter.NotFound(apiutil.NotFoundHandler)
	})

	router.NotFound(siteService.ServeContent)

	return
}
