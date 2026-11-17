// Package render converts page Markdown to HTML for the site: GitHub
// Flavored Markdown, heading anchors, link and image rewriting, and plain
// text for the search index.
package render

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// Options control rendering.
type Options struct {
	// Link rewrites a link destination; returning "" keeps it.
	Link func(dest string) string
	// Image rewrites an image source; returning "" keeps it.
	Image func(src string) string
	// AllowHTML passes raw HTML in the Markdown through. Off by default:
	// pages are often written by agents.
	AllowHTML bool
}

// Result is a rendered page.
type Result struct {
	HTML        string
	Headings    []doc.Heading
	Text        string // plain text, for search
	UsesMermaid bool
}

var (
	safe   = goldmark.New(goldmark.WithExtensions(extension.GFM))
	unsafe = goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe()))
)

// Render converts Markdown to HTML.
func Render(src string, opts Options) (Result, error) {
	md := safe
	if opts.AllowHTML {
		md = unsafe
	}
	source := []byte(src)
	root := md.Parser().Parse(text.NewReader(source))

	var res Result
	var plain strings.Builder
	used := map[string]bool{}
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			title := strings.TrimSpace(nodeText(node, source))
			id := uniqueID(doc.Slug(title), used)
			node.SetAttributeString("id", []byte(id))
			res.Headings = append(res.Headings, doc.Heading{ID: id, Text: title, Level: node.Level})
		case *ast.Link:
			if opts.Link != nil {
				if dest := opts.Link(string(node.Destination)); dest != "" {
					node.Destination = []byte(dest)
				}
			}
		case *ast.Image:
			if opts.Image != nil {
				if dest := opts.Image(string(node.Destination)); dest != "" {
					node.Destination = []byte(dest)
				}
			}
		case *ast.FencedCodeBlock:
			if string(node.Language(source)) == "mermaid" {
				res.UsesMermaid = true
			}
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				plain.Write(seg.Value(source))
			}
			plain.WriteByte('\n')
		case *ast.Text:
			plain.Write(node.Segment.Value(source))
			if node.SoftLineBreak() || node.HardLineBreak() {
				plain.WriteByte(' ')
			}
		case *ast.Paragraph:
			plain.WriteByte('\n')
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Result{}, err
	}
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, root); err != nil {
		return Result{}, err
	}
	if res.Headings == nil {
		res.Headings = []doc.Heading{}
	}
	res.HTML = buf.String()
	res.Text = strings.Join(strings.Fields(plain.String()), " ")
	return res, nil
}

// Inline renders a short Markdown fragment (a footer, a tagline) without the
// surrounding paragraph.
func Inline(src string) string {
	var buf bytes.Buffer
	if err := safe.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	out := strings.TrimSpace(buf.String())
	out = strings.TrimPrefix(out, "<p>")
	return strings.TrimSuffix(out, "</p>")
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
