package main

import (
	"context"
	"fmt"
	stdlog "log"
	"os"

	"log/slog"

	"markdown.ninja/cmd/mdninja-ebook/ebook"
	"markdown.ninja/stdx/cobra"
	"markdown.ninja/stdx/log/slogx"
)

var (
	rootCmdFlagConfigPath string
)

func init() {
	rootCmd.Flags().StringVarP(&rootCmdFlagConfigPath, "config", "c", ebook.DefaultConfigPath, fmt.Sprintf("Configuration file (default: %s)", ebook.DefaultConfigPath))

	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(coverCmd)
	rootCmd.AddCommand(epub2azw3Cmd)
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
	Use:           "mdninja-ebook",
	Short:         "Various utilites to generate and manipulate Ebooks. Visit https://markdown.ninja for more information.",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		err = ebook.Generate(cmd.Context(), rootCmdFlagConfigPath)
		return err
	},
}
