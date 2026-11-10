package render

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

var headingOpen = regexp.MustCompile(`<h([1-6])>`)

// Render converts Markdown into HTML and a table-of-contents list.
func Render(src string) (string, []doc.Heading, error) {
	source := []byte(src)
	parsed := markdown.Parser().Parse(text.NewReader(source))

	var headings []doc.Heading
	used := map[string]bool{}
	err := ast.Walk(parsed, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := n.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		title := strings.TrimSpace(nodeText(heading, source))
		id := uniqueID(doc.Slug(title), used)
		headings = append(headings, doc.Heading{ID: id, Text: title, Level: heading.Level})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}

	var buf bytes.Buffer
	if err := markdown.Renderer().Render(&buf, source, parsed); err != nil {
		return "", nil, err
	}
	return injectHeadingIDs(buf.String(), headings), headings, nil
}

func nodeText(n ast.Node, source []byte) string {
	var b strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch node := child.(type) {
		case *ast.Text:
			b.Write(node.Segment.Value(source))
		case *ast.String:
			b.Write(node.Value)
		default:
			b.WriteString(nodeText(child, source))
		}
	}
	return b.String()
}

func uniqueID(base string, used map[string]bool) string {
	if base == "" {
		base = "section"
	}
	if !used[base] {
		used[base] = true
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

func injectHeadingIDs(html string, headings []doc.Heading) string {
	index := 0
	return headingOpen.ReplaceAllStringFunc(html, func(match string) string {
		if index >= len(headings) {
			return match
		}
		level := match[2 : len(match)-1]
		id := headings[index].ID
		index++
		return `<h` + level + ` id="` + id + `">`
	})
}
