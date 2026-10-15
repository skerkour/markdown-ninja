package client

import (
	"context"
	"errors"

	"markdown.ninja/stdx/log/slogx"
)

type PublishInput struct {
	ConfigPath string
	Site       *string
}

func (client *Client) Publish(ctx context.Context, input PublishInput) (err error) {
	logger := slogx.FromCtx(ctx)

	config, err := client.loadConfig(ctx, input.ConfigPath)
	if err != nil {
		return
	}
	logger.Debug("publish: config successfully loaded")

	if input.Site == nil && config.Site == nil {

	}
	var websiteDomain string
	if input.Site != nil {
		websiteDomain = *input.Site
	} else if config.Site != nil {
		websiteDomain = *config.Site
	} else {
		err = errors.New("a site must be provided either in the configuration file or by the CLI")
		return
	}

	logger.Debug("Publishing website")
	err = client.publishWebsite(ctx, websiteDomain, input, config)
	if err != nil {
		return
	}
	logger.Debug("publish: website susccessfully updated")

	return
}
