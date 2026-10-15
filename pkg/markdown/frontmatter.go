package markdown

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	mdparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/stdx/yaml"
)

var ErrFrontmatterIsMissing = errs.NotFound("Frontmatter is missing")

var (
	frontMatterDelimiterYaml = []byte("---")
)

type Frontmatter struct {
	Source string
	Data   map[string]any
	err    error
	node   gast.Node
}

func NewEmtpyFrontmatter() *Frontmatter {
	return &Frontmatter{
		Source: "",
		Data:   map[string]any{},
	}
}

var frontmatterContextKey = mdparser.NewContextKey()

func GetFrontmatter(ctx mdparser.Context) (frontmatter *Frontmatter, err error) {
	frontmatterData := ctx.Get(frontmatterContextKey)
	if frontmatterData == nil {
		err = ErrFrontmatterIsMissing
		return
	}
	frontmatter = frontmatterData.(*Frontmatter)
	if frontmatter.err != nil {
		err = frontmatter.err
		return
	}
	return frontmatter, nil
}

type frontmatterParser struct {
}

var defaultFrontmatterParser = &frontmatterParser{}

func newFrontmatterParser() mdparser.BlockParser {
	return defaultFrontmatterParser
}

func (parser *frontmatterParser) isFrontmatterDelimiter(line []byte, isStartDelimiter bool) bool {
	line = bytes.TrimSpace(line)

	return bytes.Equal(line, frontMatterDelimiterYaml)
}

func (parser *frontmatterParser) Trigger() []byte {
	return []byte{'+', '-'}
}

func (parser *frontmatterParser) Open(parent gast.Node, reader text.Reader, pc mdparser.Context) (gast.Node, mdparser.State) {
	// front matter must be at the beginning of the document.
	lineNum, _ := reader.Position()
	if lineNum != 0 {
		return nil, mdparser.NoChildren
	}

	line, _ := reader.PeekLine()
	if parser.isFrontmatterDelimiter(line, true) {
		return gast.NewTextBlock(), mdparser.NoChildren
	}

	return nil, mdparser.NoChildren
}

func (parser *frontmatterParser) Continue(node gast.Node, reader text.Reader, pc mdparser.Context) mdparser.State {
	line, segment := reader.PeekLine()

	if parser.isFrontmatterDelimiter(line, false) {
		reader.Advance(segment.Len())
		return mdparser.Close
	}

	node.Lines().Append(segment)
	return mdparser.Continue | mdparser.NoChildren
}

func (parser *frontmatterParser) Close(node gast.Node, reader text.Reader, pc mdparser.Context) {
	lines := node.Lines()
	var buf bytes.Buffer

	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		buf.Write(segment.Value(reader.Source()))
	}

	frontmatter := &Frontmatter{
		Source: strings.TrimSpace(buf.String()),
		node:   node,
		Data:   map[string]any{},
		err:    nil,
	}

	frontmatter.err = yaml.Unmarshal([]byte(frontmatter.Source), &frontmatter.Data)

	pc.Set(frontmatterContextKey, frontmatter)

	if frontmatter.err == nil {
		node.Parent().RemoveChild(node.Parent(), node)
	}
}

func (b *frontmatterParser) CanInterruptParagraph() bool {
	return false
}

func (b *frontmatterParser) CanAcceptIndentedLine() bool {
	return false
}

type frontmatterGoldmarkExtension struct {
}

var FrontmatterExtension = &frontmatterGoldmarkExtension{}

func (e *frontmatterGoldmarkExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		mdparser.WithBlockParsers(
			util.Prioritized(newFrontmatterParser(), 0),
		),
	)
}
