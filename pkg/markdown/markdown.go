package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
	highlighting "markdown.ninja/stdx/goldmark-highlighting"
)

func newMarkdownRenderer(extenders ...goldmark.Extender) goldmark.Markdown {
	exts := []goldmark.Extender{
		extension.GFM,
		extension.Footnote,
		HTMLCommentsParserExtension,
		SnippetsParserExtension,
		highlighting.NewHighlighting(
			highlighting.WithStyle("monokai"),
		),
	}
	exts = append(exts, extenders...)

	return goldmark.New(
		goldmark.WithExtensions(exts...),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			htmlrenderer.WithHardWraps(),
			htmlrenderer.WithXHTML(),
			htmlrenderer.WithUnsafe(),
		),
	)
}

func ToHtmlPage(contentMarkdown, websiteBaseUrl string) (string, error) {
	htmlBuffer := bytes.NewBuffer(make([]byte, 0, len(contentMarkdown)))
	markdownRenderer := newMarkdownRenderer(NewAbsoluteUrlsExtension(websiteBaseUrl, true, false))

	err := markdownRenderer.Convert([]byte(contentMarkdown), htmlBuffer)
	if err != nil {
		return "", ErrMarkdownIsNotValid(err)
	}

	htmlBytes := removeNewsletterTags(htmlBuffer.Bytes())
	return string(htmlBytes), nil
	// return parseAndModifyHtmlLinksAndImages(websiteBaseUrl, markdownToHtmlBuffer.Bytes())
}

func ToHtmlEmail(websiteBaseUrl, contentMarkdown string) (string, error) {
	markdownToHtmlBuffer := bytes.NewBuffer(make([]byte, 0, len(contentMarkdown)))
	markdownRenderer := newMarkdownRenderer(NewAbsoluteUrlsExtension(websiteBaseUrl, true, true))

	err := markdownRenderer.Convert([]byte(contentMarkdown), markdownToHtmlBuffer)
	if err != nil {
		return "", ErrMarkdownIsNotValid(err)
	}

	return markdownToHtmlBuffer.String(), nil
	// return parseAndModifyHtmlLinksAndImages(websiteBaseUrl, markdownToHtmlBuffer.Bytes())
}
