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
		Overview:    doc.Page{Markdown: "## Admissions\n\nPatients enter here."},
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

	if err := Write(document, dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, name := range []string{"index.html", "style.css", "app.js", "docs.js"} {
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
	for _, want := range []string{"window.INDOC_DOCS", "Hospital API", "/health", "id=\\\"admissions\\\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs.js missing %q\n%s", want, body)
		}
	}
}
