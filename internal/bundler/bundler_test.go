package bundler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/doc"
)

func TestWriteSite(t *testing.T) {
	dir := t.TempDir()
	document := &doc.Document{
		ProjectName: "Hospital API",
		Appearance: &doc.Appearance{
			Logo: "logo.svg",
			Colors: &doc.Colors{
				Primary: "#0969da",
				Accent:  "#1f883d",
			},
		},
		Overview: doc.Page{Markdown: "## Admissions\n\nPatients enter here."},
		Guides: []doc.Page{{
			Title:    "Run it",
			Markdown: "Start uvicorn.",
		}},
		Reference: []doc.RefSection{{
			Kind: doc.KindRoutes,
			Entries: []doc.RefEntry{{
				Name:   "/health",
				Detail: "GET",
				Source: "main.py:4",
			}},
		}},
	}
	logo := filepath.Join(dir, "logo.svg")
	if err := os.WriteFile(logo, []byte("<svg></svg>"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Write(document, dir, dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, name := range []string{"index.html", "style.css", "app.js", "docs.js", "assets/logo.svg"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "Hospital API") {
		t.Fatalf("title was not set: %s", index)
	}
	payload, err := os.ReadFile(filepath.Join(dir, "docs.js"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, want := range []string{"window.INDOC_DOCS", "Hospital API", "/health", "id=\\\"admissions\\\"", `"logo":"assets/logo.svg"`, `"primary":"#0969da"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs.js missing %q\n%s", want, body)
		}
	}
}

func TestWriteSiteRejectsUnsupportedLogoURLScheme(t *testing.T) {
	document := &doc.Document{
		ProjectName: "Demo",
		Appearance:  &doc.Appearance{Logo: "javascript:alert(1)"},
		Overview:    doc.Page{Markdown: "Overview"},
	}
	if err := Write(document, t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("expected unsupported logo URL scheme to fail")
	}
}
