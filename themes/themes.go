package themes

import (
	"embed"
	"io/fs"

	"markdown.ninja/stdx/set"
)

//go:embed */dist/*
var ThemesFs embed.FS

//go:embed metadata.html
var HeadMetadata []byte

//go:embed styles.html
var HeadStyles []byte

//go:embed default_icons/*
var defaultIconsFs embed.FS

const ScrapingPolicy = `
{{ safeHtml "<!-- Copying, scraping or crawling this website without explicit written permission is forbidden. -->" }}
<!-- {{ safeHtml "Please contact hello[ at ]markdown.ninja if you want your crawler to be unblocked or for API access." }} -->
`

var BuiltInThemes = set.NewFromSlice([]string{
	"blog",
	"docs",
})

func DefaultIconsFs() fs.FS {
	iconsFs, _ := fs.Sub(defaultIconsFs, "default_icons")
	return iconsFs
}
