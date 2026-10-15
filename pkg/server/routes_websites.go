package server

import (
	"fmt"
	"net/http"
	"strings"

	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/guid"
)

func (server *server) websiteUpdateIcon(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// we use 1.5MB max of memory
	err := req.ParseMultipartForm(1_500_000)
	if err != nil {
		err = fmt.Errorf("websiteUpdateIcon: error parsing multipart form: %w", err)
		apiutil.SendError(ctx, w, err)
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		err = fmt.Errorf("websiteUpdateIcon: error reading form file: %w", err)
		apiutil.SendError(ctx, w, err)
		return
	}
	defer file.Close()

	siteIDStr := strings.TrimSpace(req.FormValue("website_id"))
	siteID, err := guid.Parse(siteIDStr)
	if err != nil {
		err = fmt.Errorf("websiteUpdateIcon: website_id is not valid: %w", err)
		apiutil.SendError(ctx, w, err)
		return
	}

	input := websites.UpdateWebsiteIconInput{
		WebsiteID: siteID,
		Data:      file,
	}
	err = server.websitesService.UpdateWebsiteIcon(ctx, input)
	if err != nil {
		apiutil.SendError(ctx, w, err)
		return
	}

	apiutil.SendOk(ctx, w)
}
