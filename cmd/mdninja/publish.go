package main

import (
	"os"

	"markdown.ninja/cmd/mdninja/client"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/stdx/cobra"
	"markdown.ninja/stdx/log/slogx"
)

var flagPublishSync bool
var flagPublishConfig string
var flagPublishSite string

func init() {
	publishCmd.Flags().StringVar(&flagPublishConfig, "config", "markdown_ninja.yml", "Configuration file")
	publishCmd.Flags().StringVarP(&flagPublishSite, "site", "s", "", "Website's slug")
}

var publishCmd = &cobra.Command{
	Use:           "publish",
	Short:         "Publish your website or a new version of your book",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		ctx := cmd.Context()
		logger := slogx.FromCtx(ctx)

		markdowNinjaApiKey := os.Getenv("MARKDOWN_NINJA_API_KEY")
		if markdowNinjaApiKey == "" {
			err = errs.InvalidArgument("MARKDOWN_NINJA_API_KEY env var not found")
			return
		}

		markdowNinjaUrl := os.Getenv("MARKDOWN_NINJA_URL")
		if markdowNinjaUrl == "" {
			markdowNinjaUrl = "https://markdown.ninja"
		}

		markdowNinjaClient, err := client.New(markdowNinjaUrl, markdowNinjaApiKey, logger)
		if err != nil {
			return
		}

		var websiteSlug *string
		if flagPublishSite != "" {
			websiteSlug = &flagPublishSite
		}

		opt := client.PublishInput{
			ConfigPath: flagPublishConfig,
			Site:       websiteSlug,
		}
		err = markdowNinjaClient.Publish(ctx, opt)
		return err
	},
}
