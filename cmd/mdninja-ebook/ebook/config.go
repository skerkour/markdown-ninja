package ebook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"markdown.ninja/stdx/yaml"
)

const (
	DefaultConfigPath         = "markdown_ninja_book.yaml"
	pandocTmpDirectoryPattern = "markdown_ninja-ebook-pandoc-*"
	tmpWorkingDirPattern      = "markdown_ninja-ebook-*"
)

type Config struct {
	BookID   string   `yaml:"book_id"`
	Title    string   `yaml:"title"`
	Subtitle string   `yaml:"subtitle"`
	Author   string   `yaml:"author"`
	Version  string   `yaml:"version"`
	Cover    string   `yaml:"cover"`
	Tags     []string `yaml:"tags"`
	Chapters []string `yaml:"chapters"`
	// DistDir is the destination directory where the ebooks files will be generated
	DistDir  string `yaml:"dist"`
	Filename string `yaml:"filename"`

	tmpWorkingDir string `yaml:"-"`
}

func loadConfig(_ctx context.Context, configPath string) (config Config, err error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		err = fmt.Errorf("error reading configuration file (%s): %w", configPath, err)
		return
	}

	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		err = fmt.Errorf("error parsing configuration file (%s): %w", configPath, err)
		return
	}

	if config.Filename == "" {
		config.Filename, err = bookTitleToFileName(config.Title)
		if err != nil {
			return
		}
	}

	if config.DistDir == "" {
		config.DistDir = "ebooks"
	}
	if !filepath.IsAbs(config.DistDir) {
		var workingDir string
		workingDir, err = os.Getwd()
		if err != nil {
			err = fmt.Errorf("error getting working directory: %w", err)
			return
		}
		config.DistDir = filepath.Join(workingDir, config.DistDir)
	}

	return
}

func ebooksSandboxEnv() []string {
	now := time.Now().UTC()
	currentYear, _, _ := now.Date()
	currentLocation := now.Location()
	firstDayOfYear := time.Date(currentYear, time.January, 1, 0, 0, 0, 0, currentLocation)

	return []string{
		"TZ=UTC",
		"LANGUAGE=en_US:en",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"SHELL=/bin/bash",
		"TMP=/tmp",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		// SOURCE_DATE_EPOCH is used to make pandoc build (ex: epubs) reproducibles
		// See https://pandoc.org/MANUAL.html#reproducible-builds
		fmt.Sprintf("SOURCE_DATE_EPOCH=%d", firstDayOfYear.Unix()),
	}
}

func bookTitleToFileName(title string) (fileName string, err error) {
	title = strings.TrimSpace(title)

	ascii, err := toAscii(title)
	if err != nil {
		return
	}

	for _, c := range ascii {
		if c > unicode.MaxASCII {
			// not ascii
			continue
		}

		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			fileName += string(unicode.ToLower(c))
		} else if c == ' ' {
			fileName += "_"
		}
	}
	return
}

func toAscii(input string) (output string, err error) {
	output, _, err = transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), input)
	if err != nil {
		err = fmt.Errorf("error converting string to (%s) to ascii: %w", input, err)
		return
	}

	return
}
