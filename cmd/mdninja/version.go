package main

import (
	"fmt"

	"markdown.ninja/pkg/buildinfo"
	"markdown.ninja/stdx/cobra"
)

var versionOutputFormat string

func init() {
	versionCmd.Flags().StringVarP(&versionOutputFormat, "format", "f", "text", "The ouput format. Valid values are [text, json]")
}

// versionCmd is the server's `version` command. It display various information about the current phaser executable
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display the version and build information",
	Long:  "Display the version and build information",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		switch versionOutputFormat {
		case "text":
			buildinfo.PrintText()
		case "json":
			err = buildinfo.PrintJSON()
		default:
			err = fmt.Errorf("%s is not a valid output format", versionOutputFormat)
		}
		return err
	},
}
