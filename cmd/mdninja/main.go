package main

import (
	"context"
	stdlog "log"
	"os"

	"log/slog"

	"markdown.ninja/stdx/cobra"
	"markdown.ninja/stdx/log/slogx"
)

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(publishCmd)
}

func main() {
	stdlog.SetOutput(os.Stdout)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()
	ctx = slogx.ToCtx(ctx, logger)

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		stdlog.Fatal(err)
	}
}

var rootCmd = &cobra.Command{
	Use:          "mdninja",
	Short:        "Visit https://markdown.ninja for more information.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		return cmd.Help()
	},
}
