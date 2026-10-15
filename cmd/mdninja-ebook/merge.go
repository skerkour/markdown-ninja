package main

import (
	"markdown.ninja/cmd/mdninja-ebook/ebook"
	"markdown.ninja/stdx/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge book.pdf cover.pdf content.pdf",
	Short: "Merge 2 PDFs",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		if len(args) != 3 {
			return cmd.Help()
		}

		dest := args[0]
		cover := args[1]
		content := args[2]

		err = ebook.MergePdfs(cmd.Context(), dest, cover, content)
		return err
	},
}
