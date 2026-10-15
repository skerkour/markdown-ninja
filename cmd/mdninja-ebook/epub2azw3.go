package main

import (
	"fmt"
	"os"
	"strings"

	"markdown.ninja/cmd/mdninja-ebook/ebook"
	"markdown.ninja/stdx/cobra"
	"markdown.ninja/stdx/log/slogx"
)

var epub2azw3Cmd = &cobra.Command{
	Use:          "epub2azw3 [epub_files...]",
	Short:        "Various utilites to generate and manipulate Ebooks. Visit https://markdown.ninja for more information.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		var epubFiles []string
		ctx := cmd.Context()
		logger := slogx.FromCtx(ctx)

		if len(args) != 0 {
			epubFiles = args
		} else {
			epubFiles, err = listEpubFiles(".")
			if err != nil {
				return
			}
		}

		for _, epubFile := range epubFiles {
			azw3File := strings.TrimSuffix(epubFile, ".epub") + ".azw3"
			err = ebook.ConvertEpubToAzw3(ctx, azw3File, epubFile, nil, nil)
			if err != nil {
				err = fmt.Errorf("error converting epub file (%s) to azw3: %w", epubFile, err)
				return
			}
			logger.Info(fmt.Sprintf("%s sucessfully converted to azw3", epubFile))
		}

		return nil
	},
}

func listEpubFiles(directory string) (epubFiles []string, err error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		err = fmt.Errorf("error opening %s: %w", directory, err)
		return
	}

	epubFiles = make([]string, 0, len(files))

	for _, file := range files {
		fileName := file.Name()
		if file.Type().IsRegular() && strings.HasSuffix(file.Name(), ".epub") {
			epubFiles = append(epubFiles, fileName)
		}
	}

	return epubFiles, nil
}
