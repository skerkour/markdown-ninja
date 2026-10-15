package service

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"golang.org/x/net/html"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/yaml"
	"markdown.ninja/themes"
)

type parsedTheme struct {
	IndexTemplate *template.Template
	Assets        fs.FS
	// Hash is a BLAKE3 hash of all the files
	// of the theme to detect when the theme has changed to be able to handle chaching and ETags correctly
	Hash []byte

	// SpecialPages
	SpecialPages []*regexp.Regexp
}

type themeConfig struct {
	Name         string   `yaml:"name"`
	SpecialPages []string `yaml:"special_pages"`
}

func loadThemes() (ret map[string]parsedTheme, err error) {
	templateFuncs := template.FuncMap{
		"avail": avail,
		// We need the formatDate because if we use .Page.Date.Format "..." in the template, vite
		// (the frontend bundling tool) crashes
		// it also enables us to have unified formatting
		"formatDate": formatDate,
		"safeHtml":   safeHtml,
	}

	ret = make(map[string]parsedTheme, len(themes.BuiltInThemes))
	for themeName := range themes.BuiltInThemes.Iter() {
		// check that the dist directory exists
		_, err = themes.ThemesFs.ReadDir(filepath.Join(themeName, "dist"))
		if err != nil {
			err = fmt.Errorf("dist directory is missing for theme: %s", themeName)
			return
		}

		var themeFs fs.FS
		themeFs, err = fs.Sub(themes.ThemesFs, filepath.Join(themeName, "dist"))
		if err != nil {
			err = fmt.Errorf("error loading subFs for theme %s: %w", themeName, err)
			return
		}

		var theme parsedTheme
		theme, err = loadTheme(themeName, themeFs, templateFuncs)
		if err != nil {
			return
		}

		ret[themeName] = theme
	}

	return
}

func loadTheme(themeName string, themeFS fs.FS, templateFuncs template.FuncMap) (theme parsedTheme, err error) {
	themeConfigData, err := fs.ReadFile(themeFS, "markdown_ninja_theme.yml")
	if err != nil {
		err = fmt.Errorf("error reading markdown_ninja_theme.yml for theme %s: %w", themeName, err)
		return
	}

	var themeConfig themeConfig
	err = yaml.Unmarshal(themeConfigData, &themeConfig)
	if err != nil {
		err = fmt.Errorf("error parsing markdown_ninja_theme.yml for theme %s: %w", themeName, err)
		return
	}

	theme.SpecialPages = make([]*regexp.Regexp, 0, len(themeConfig.SpecialPages))
	for _, specialPagePath := range themeConfig.SpecialPages {
		var specialPagePathRegex *regexp.Regexp
		specialPagePathRegex, err = regexp.Compile("^" + specialPagePath + "$")
		if err != nil {
			err = fmt.Errorf("parsing theme's special page regexp (%s): %w", specialPagePath, err)
			return
		}
		theme.SpecialPages = append(theme.SpecialPages, specialPagePathRegex)
	}

	indexHtmlData, err := fs.ReadFile(themeFS, "index.html")
	if err != nil {
		err = fmt.Errorf("error reading index.html for theme %s: %w", themeName, err)
		return
	}

	indexHtmlData, err = injectMetdataToIndexHtml(indexHtmlData)
	if err != nil {
		return
	}

	indexTemplate, err := template.New("index.html").
		Funcs(templateFuncs).
		Parse(string(indexHtmlData))
	// ParseFS(themeFS, "index.html")
	if err != nil {
		err = fmt.Errorf("error parsing index.html template for theme %s: %w", themeName, err)
		return
	}

	theme.IndexTemplate = indexTemplate

	themeHasher := blake3.New()
	err = hashThemeFiles(themeFS, themeHasher)
	if err != nil {
		return
	}
	theme.Hash = themeHasher.Sum(nil)

	theme.Assets, err = fs.Sub(themeFS, "theme")
	if err != nil {
		err = websites.ErrOpeningThemeAssets(err)
		return
	}
	return
}

func hashThemeFiles(theme fs.FS, hasher io.Writer) (err error) {
	// it's okay to use a single hasher for all the files because fs.WalkDir is deterministic
	err = fs.WalkDir(theme, ".", func(path string, fileEntry fs.DirEntry, errWalk error) error {
		if errWalk != nil {
			return fmt.Errorf("site.hashThemeFiles: error processing file %s: %w", path, errWalk)
		}

		if fileEntry.IsDir() || !fileEntry.Type().IsRegular() {
			return nil
		}

		file, errWalk := theme.Open(path)
		if errWalk != nil {
			return fmt.Errorf("site.hashThemeFiles: error opening file %s: %w", path, errWalk)
		}
		defer file.Close()

		_, errWalk = io.Copy(hasher, file)
		if errWalk != nil {
			return fmt.Errorf("site.hashThemeFiles: error hashing file %s: %w", path, errWalk)
		}

		return nil
	})
	return err
}

// Allowed elements in <head>
// - link rel="icon"
// - link rel="stylesheet"
// - meta
// - script
func injectMetdataToIndexHtml(indexHtmlData []byte) ([]byte, error) {
	htmlNodes, err := html.Parse(bytes.NewReader(indexHtmlData))
	if err != nil {
		return nil, fmt.Errorf("parsing index.html HTML: %w", err)
	}

	htmlOut := bytes.NewBuffer(make([]byte, 0, len(indexHtmlData)))
	body := bytes.NewBuffer(make([]byte, 0, len(indexHtmlData)))
	scriptsAndStyles := bytes.NewBuffer(make([]byte, 0, 300))

	htmlOut.WriteString("<!DOCTYPE html>\n")
	htmlOut.WriteString("<html>\n")
	htmlOut.WriteString("  <head>\n")
	htmlOut.WriteString(themes.ScrapingPolicy)

	var parseHtml func(node *html.Node, inHead bool)
	parseHtml = func(node *html.Node, inHead bool) {
		if node.Type == html.ElementNode && node.Data == "body" {
			html.Render(body, node)
		} else if node.Type == html.ElementNode && inHead {
			switch node.Data {
			case "link":
				for _, attr := range node.Attr {
					if attr.Key == "rel" {
						if strings.Contains(attr.Val, "icon") {
							htmlOut.WriteString("    ")
							html.Render(htmlOut, node)
							htmlOut.WriteByte('\n')
						} else if attr.Val == "stylesheet" {
							scriptsAndStyles.WriteString("    ")
							html.Render(scriptsAndStyles, node)
							scriptsAndStyles.WriteByte('\n')
						}
					}

				}
			case "meta":
				htmlOut.WriteString("    ")
				html.Render(htmlOut, node)
				htmlOut.WriteByte('\n')
			case "script":
				scriptsAndStyles.WriteString("    ")
				html.Render(scriptsAndStyles, node)
				scriptsAndStyles.WriteByte('\n')
			}

			// node.Data == "link"
			// fmt.Println(node.Attr)
			// buffer := bytes.NewBuffer(make([]byte, 0, 100))
			// html.Render(buffer, node)
			// fmt.Println(buffer.String())
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			parseHtml(child, node.Data == "head")
		}
	}

	parseHtml(htmlNodes, false)

	htmlOut.WriteByte('\n')
	htmlOut.Write(themes.HeadMetadata)
	htmlOut.WriteString("\n\n")
	htmlOut.WriteString(`
	<script>
	  window.__markdown_ninja_data = {{ .MarkdownNinjaData }};
	</script>
`)
	htmlOut.Write(scriptsAndStyles.Bytes())
	htmlOut.WriteString("\n\n")
	htmlOut.Write(themes.HeadStyles)
	htmlOut.WriteString("{{ .Header }}\n")
	htmlOut.WriteString("\n  </head>\n")
	htmlOut.Write(body.Bytes())
	htmlOut.WriteString("\n{{ .Footer }}\n")
	htmlOut.WriteString("\n</html>")

	return htmlOut.Bytes(), nil
}

func avail(name string, data any) bool {
	if data == nil {
		return false
	}

	v := reflect.ValueOf(data)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	return v.FieldByName(name).IsValid()
}

func formatDate(date time.Time) string {
	return date.Format(time.RFC3339)
}

func safeHtml(s string) template.HTML {
	return template.HTML(s)
}
