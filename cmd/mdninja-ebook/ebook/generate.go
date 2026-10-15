package ebook

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"markdown.ninja/cmd/mdninja-ebook/pandoc"
	"markdown.ninja/stdx/log/slogx"
)

// Generate ebooks from the given configuration file
func Generate(ctx context.Context, configPath string) (err error) {
	config, err := loadConfig(ctx, configPath)
	if err != nil {
		return
	}

	// workingDir is used to copy all the files to avoid any data loss
	tmpWorkingDir, err := os.MkdirTemp("", tmpWorkingDirPattern)
	if err != nil {
		err = fmt.Errorf("error creating tmp working directory: %w", err)
		return
	}
	defer os.RemoveAll(tmpWorkingDir)
	config.tmpWorkingDir = tmpWorkingDir

	err = copyDir(config.tmpWorkingDir, ".")
	if err != nil {
		err = fmt.Errorf("copying data to tmp working dir: %w", err)
		return
	}

	if config.Cover == "" {
		config.Cover, err = tmpFile(config.tmpWorkingDir, "markdown-ninja-ebook-cover-*")
		if err != nil {
			err = fmt.Errorf("error creating tmp cover: %w", err)
			return
		}

		err = os.WriteFile(config.Cover, pandoc.DefaultCover, 0600)
		if err != nil {
			err = fmt.Errorf("error writing tmp cover: %w", err)
			return
		}
	}

	// pandocTmpDir is used to store the temporary configuration files for pandoc.
	// The directory is removed once the function exit
	pandocTmpDir, err := os.MkdirTemp(config.tmpWorkingDir, pandocTmpDirectoryPattern)
	if err != nil {
		err = fmt.Errorf("error creating tmp directory: %w", err)
		return
	}

	pandocFiles, err := generatePandocFiles(config, pandocTmpDir)
	if err != nil {
		return
	}

	if config.DistDir != "" {
		err = os.MkdirAll(config.DistDir, os.ModeDir|0700)
		if err != nil {
			err = fmt.Errorf("error creating dist directory (%s): %w", config.DistDir, err)
			return
		}
	}

	err = generateEbooks(ctx, config, pandocFiles)
	if err != nil {
		return
	}

	return
}

func generateEbooks(ctx context.Context, config Config, pandocFiles pandocFiles) (err error) {
	logger := slogx.FromCtx(ctx)

	distFilePdf := filepath.Join(config.DistDir, config.Filename+".pdf")
	distFileEpub := filepath.Join(config.DistDir, config.Filename+".epub")
	distFileAzw3 := filepath.Join(config.DistDir, config.Filename+".azw3")

	// first delete existing files if they already exist
	os.Remove(distFilePdf)
	os.Remove(distFileEpub)
	os.Remove(distFileAzw3)

	err = ebookToEpub(ctx, config, pandocFiles, distFileEpub)
	if err != nil {
		return
	}
	logger.Info("Epub successfully generated", slog.String("file", distFileEpub))

	err = ConvertEpubToAzw3(ctx, distFileAzw3, distFileEpub, &config.Cover, &config.tmpWorkingDir)
	if err != nil {
		return
	}
	logger.Info("Azw3 successfully generated", slog.String("file", distFileAzw3))

	err = ebookToPdf(ctx, config, pandocFiles, distFilePdf, config.Cover)
	if err != nil {
		return
	}
	logger.Info("PDF successfully generated", slog.String("file", distFilePdf))

	return
}
