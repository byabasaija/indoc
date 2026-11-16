package bundler

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/render"
)

//go:embed ui/index.html ui/style.css ui/app.js
var ui embed.FS

type payload struct {
	ProjectName string          `json:"project_name"`
	Appearance  *doc.Appearance `json:"appearance,omitempty"`
	Pages       []pageView      `json:"pages"`
}

type pageView struct {
	ID       string         `json:"id"`
	Section  string         `json:"section"`
	Title    string         `json:"title"`
	HTML     string         `json:"html"`
	Text     string         `json:"text"`
	Kind     string         `json:"kind,omitempty"`
	Headings []doc.Heading  `json:"headings"`
	Entries  []doc.RefEntry `json:"entries,omitempty"`
}

// Write compiles a document into a static site at outputDir.
func Write(document *doc.Document, outputDir, sourceDir string) error {
	if err := doc.Normalize(document); err != nil {
		return err
	}
	site, err := buildPayload(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", outputDir, err)
	}
	if document.Appearance != nil {
		appearance := *document.Appearance
		if appearance.Logo != "" {
			logo, err := writeLogo(outputDir, sourceDir, appearance.Logo)
			if err != nil {
				return err
			}
			appearance.Logo = logo
		}
		site.Appearance = &appearance
	}
	if err := writeAssets(outputDir, document.ProjectName); err != nil {
		return err
	}
	return writePayload(filepath.Join(outputDir, "docs.js"), site)
}

func writeLogo(outputDir, sourceDir, logo string) (string, error) {
	parsed, err := url.Parse(logo)
	if err != nil {
		return "", fmt.Errorf("invalid appearance.logo: %w", err)
	}
	if parsed.IsAbs() {
		if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
			return "", fmt.Errorf("appearance.logo URL must use http or https")
		}
		if parsed.Hostname() == "" {
			return "", fmt.Errorf("appearance.logo URL must include a host")
		}
		return logo, nil
	}

	extension := strings.ToLower(filepath.Ext(logo))
	switch extension {
	case ".avif", ".gif", ".jpeg", ".jpg", ".png", ".svg", ".webp":
	default:
		return "", fmt.Errorf("appearance.logo must be an http(s) URL or a local image file")
	}
	source := filepath.Join(sourceDir, filepath.Clean(filepath.FromSlash(logo)))
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("failed to read appearance.logo %s: %w", logo, err)
	}
	const relativePath = "assets/logo"
	if err := os.MkdirAll(filepath.Join(outputDir, "assets"), 0755); err != nil {
		return "", fmt.Errorf("failed to create logo directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, relativePath+extension), data, 0644); err != nil {
		return "", fmt.Errorf("failed to write logo: %w", err)
	}
	return relativePath + extension, nil
}

func buildPayload(document *doc.Document) (*payload, error) {
	site := &payload{ProjectName: document.ProjectName}
	overview, err := narrativePage(document.Overview, "overview")
	if err != nil {
		return nil, err
	}
	site.Pages = append(site.Pages, overview)
	for _, guide := range document.Guides {
		page, err := narrativePage(guide, "guides")
		if err != nil {
			return nil, fmt.Errorf("guide %s: %w", guide.ID, err)
		}
		site.Pages = append(site.Pages, page)
	}
	for _, section := range document.Reference {
		page, err := referencePage(section)
		if err != nil {
			return nil, fmt.Errorf("reference %s: %w", section.ID, err)
		}
		site.Pages = append(site.Pages, page)
	}
	return site, nil
}

func narrativePage(page doc.Page, section string) (pageView, error) {
	rendered, headings, err := render.Render(page.Markdown)
	if err != nil {
		return pageView{}, err
	}
	if headings == nil {
		headings = []doc.Heading{}
	}
	return pageView{
		ID:       page.ID,
		Section:  section,
		Title:    page.Title,
		HTML:     rendered,
		Text:     page.Markdown,
		Headings: headings,
	}, nil
}

func referencePage(section doc.RefSection) (pageView, error) {
	view := pageView{
		ID:       section.ID,
		Section:  "reference",
		Title:    section.Title,
		Kind:     section.Kind,
		Headings: []doc.Heading{},
		Entries:  section.Entries,
	}
	var text bytes.Buffer
	if section.Intro != "" {
		rendered, headings, err := render.Render(section.Intro)
		if err != nil {
			return pageView{}, err
		}
		view.HTML = rendered
		view.Headings = headings
		text.WriteString(section.Intro)
		text.WriteByte('\n')
	}
	for _, entry := range section.Entries {
		text.WriteString(entry.Detail)
		text.WriteByte(' ')
		text.WriteString(entry.Name)
		text.WriteByte(' ')
		text.WriteString(entry.Description)
		text.WriteByte('\n')
	}
	view.Text = text.String()
	return view, nil
}

func writeAssets(outputDir, projectName string) error {
	files := []string{"index.html", "style.css", "app.js"}
	for _, name := range files {
		data, err := ui.ReadFile("ui/" + name)
		if err != nil {
			return fmt.Errorf("missing embedded %s: %w", name, err)
		}
		if name == "index.html" {
			data = bytes.ReplaceAll(data, []byte("{{project}}"), []byte(html.EscapeString(projectName)))
		}
		if err := os.WriteFile(filepath.Join(outputDir, name), data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
	}
	return nil
}

func writePayload(path string, site *payload) error {
	var buf bytes.Buffer
	buf.WriteString("window.INDOC_DOCS = ")
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(site); err != nil {
		return fmt.Errorf("failed to encode docs payload: %w", err)
	}
	buf.WriteString(";\n")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}
