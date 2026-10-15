package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/httpx"
	"markdown.ninja/stdx/log/slogx"
)

type webappIndexHtmlTemplateData struct {
	InitData template.JS
}

func (server *server) serveWebapp(res http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	if path == "/favicon.ico" {
		http.Redirect(res, req, "/webapp/markdown_ninja_logo_64.png", http.StatusMovedPermanently)
		return
	} else if path != "/index.html" {
		file, err := server.webappFs.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			file.Close()
			server.webappHandler(res, req)
			return
		}
	}

	ctx := req.Context()
	httpCtx := httpctx.FromCtx(ctx)
	logger := slogx.FromCtx(ctx)
	initData := kernel.InitData{
		StripePublicKey: server.stripePublicKey,
		Country:         httpCtx.Client.CountryCode,
		ContactEamil:    server.emailsConfig.ContactAddress.Address,
		Pricing: []kernel.Plan{
			kernel.PlanFree,
			kernel.PlanPro,
			kernel.PlanEnterprise,
		},
		Pingoo: kernel.InitDataPingoo{
			AppID:    server.pingooConfig.AppID,
			Endpoint: server.pingooConfig.Endpoint,
		},
		WebsitesBaseUrl: server.httpConfig.WebsitesBaseUrl.String(),
	}

	initDataJson, err := json.Marshal(initData)
	if err != nil {
		logger.Error(fmt.Sprintf("server.serveWebapp: error marhsalling init data to JSON: %s", err))
		http.Error(res, "Internal Error.", http.StatusInternalServerError)
		return
	}

	var rawEtag [32]byte
	etagHasher := blake3.New()
	etagHasher.Write(initDataJson)
	etagHasher.Write(server.webappIndexHtmlHash)
	etagHasher.Sum(rawEtag[:0])
	etag := base64.RawURLEncoding.EncodeToString(rawEtag[:])

	res.Header().Set(httpx.HeaderContentType, httpx.MediaTypeHtmlUtf8)
	res.Header().Set(httpx.HeaderCacheControl, httpx.CacheControlDynamic)
	res.Header().Set(httpx.HeaderETag, strconv.Quote(etag))

	requestEtag := httpx.CleanIfNoneMatchHeader(req.Header.Get(httpx.HeaderIfNoneMatch))
	if requestEtag == etag {
		res.WriteHeader(http.StatusNotModified)
		return
	}

	templateBuffer := bytes.NewBuffer(make([]byte, 0, 2000))
	err = server.webappIndexHtmlTemplate.Execute(templateBuffer, webappIndexHtmlTemplateData{
		InitData: template.JS(initDataJson),
	})
	if err != nil {
		logger.Error(fmt.Sprintf("server.serveWebapp: error executing webapp/index.html template: %s", err))
		http.Error(res, "Internal Error.", http.StatusInternalServerError)
		return
	}

	// https://developer.mozilla.org/en-US/docs/Web/Performance/Guides/dns-prefetch
	// https://www.keycdn.com/blog/resource-hints
	// https://web.dev/learn/performance/resource-hints#preconnect
	//
	// 	res.Header().Add("Link", "<https://account.markdown.ninja>; rel=preconnect")
	// 	res.Header().Add("Link", "<https://cms.markdown.ninja>; rel=preconnect")

	res.Header().Set(httpx.HeaderContentLength, strconv.Itoa(templateBuffer.Len()))
	res.WriteHeader(http.StatusOK)
	res.Write(templateBuffer.Bytes())
}
