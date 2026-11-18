package pages

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseFrontmatterAndCitations(t *testing.T) {
	raw := []byte(`---
title: How checkout charges a card
order: 3
audience: dev
sources:
  - shop/checkout/**
---

# Ignored heading

Requests land in ` + "`shop/checkout/dispatcher.py::CheckoutService.dispatch`" + `,
which calls shop/checkout/wallet/handler.py::WalletHandler.

` + "```rust\nuse std::io::Read; // not.rs::a citation\n```" + `

Again: ` + "`shop/checkout/dispatcher.py::CheckoutService.dispatch`" + `
`)
	p, err := Parse("docs/dev/checkout.md", raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "How checkout charges a card" || !p.HasOrder || p.Order != 3 || p.Audience != "dev" {
		t.Errorf("frontmatter not applied: %+v", p)
	}
	if !reflect.DeepEqual(p.Sources, []string{"shop/checkout/**"}) {
		t.Errorf("sources = %v", p.Sources)
	}
	want := []Citation{
		{Path: "shop/checkout/dispatcher.py", Symbol: "CheckoutService.dispatch", Line: 11},
		{Path: "shop/checkout/wallet/handler.py", Symbol: "WalletHandler", Line: 12},
	}
	if !reflect.DeepEqual(p.Citations, want) {
		t.Errorf("citations = %+v\nwant %+v", p.Citations, want)
	}
}

func TestParseDocusaurusKeysAndFallbackTitles(t *testing.T) {
	p, err := Parse("docs/intro.md", []byte("---\nsidebar_label: Intro\nsidebar_position: 2\n---\nHello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Intro" || p.Order != 2 || !p.HasOrder {
		t.Errorf("docusaurus keys not read: %+v", p)
	}
	if p.Anchored() {
		t.Error("page without sources or citations should be unanchored")
	}

	p, _ = Parse("docs/getting-started.md", []byte("# Getting started fast\n\ntext"))
	if p.Title != "Getting started fast" {
		t.Errorf("H1 title = %q", p.Title)
	}
	p, _ = Parse("docs/partner/index.md", []byte("no heading"))
	if p.Title != "Partner" {
		t.Errorf("index title = %q", p.Title)
	}
	p, _ = Parse("docs/setup_guide.md", []byte("no heading"))
	if p.Title != "Setup guide" {
		t.Errorf("filename title = %q", p.Title)
	}
}

func TestParseRejectsBadFrontmatter(t *testing.T) {
	if _, err := Parse("a.md", []byte("---\nsources: [\n---\nx")); err == nil {
		t.Error("want YAML error")
	}
	if _, err := Parse("a.md", []byte("---\nsources: ['src/[']\n---\nx")); err == nil {
		t.Error("want invalid glob error")
	}
}

func TestMatch(t *testing.T) {
	files := []string{"README.md", "docs/a.md", "docs/internal-review-notes.md", "docs/partner/b.md", "docs/x.pdf"}
	got := Match(files, []string{"docs/partner/**/*.md", "docs/a.md"})
	if !reflect.DeepEqual(got, []string{"docs/a.md", "docs/partner/b.md"}) {
		t.Errorf("Match = %v", got)
	}
}

func TestRenderCitations(t *testing.T) {
	body := "See `app/x.py::A.run` and app/y.go::Serve.\n" +
		"```\napp/x.py::A.run\n```\n" +
		"Already [`app/x.py::A.run`](http://x) linked.\n" +
		"Charges the card (`app/x.py::A.run`). Retries (see `app/x.py::A.run`, `app/y.go::Serve`) twice.\n"
	href := func(c Citation) string { return "https://src/" + c.Path }
	none := func(Citation) string { return "" }
	for _, tc := range []struct {
		style string
		href  func(Citation) string
		want  string
	}{
		{CitationsShort, href, "See [`A.run`](https://src/app/x.py \"app/x.py::A.run\") and [`Serve`](https://src/app/y.go \"app/y.go::Serve\").\n" +
			"```\napp/x.py::A.run\n```\n" +
			"Already [`app/x.py::A.run`](http://x) linked.\n" +
			"Charges the card ([`A.run`](https://src/app/x.py \"app/x.py::A.run\")). Retries (see [`A.run`](https://src/app/x.py \"app/x.py::A.run\"), [`Serve`](https://src/app/y.go \"app/y.go::Serve\")) twice.\n"},
		{CitationsFull, none, "See `app/x.py::A.run` and `app/y.go::Serve`.\n" +
			"```\napp/x.py::A.run\n```\n" +
			"Already [`app/x.py::A.run`](http://x) linked.\n" +
			"Charges the card (`app/x.py::A.run`). Retries (see `app/x.py::A.run`, `app/y.go::Serve`) twice.\n"},
		{CitationsHidden, href, "See `A.run` and `Serve`.\n" +
			"```\napp/x.py::A.run\n```\n" +
			"Already [`app/x.py::A.run`](http://x) linked.\n" +
			"Charges the card. Retries twice.\n"},
	} {
		if got := RenderCitations(body, tc.style, tc.href); got != tc.want {
			t.Errorf("%s:\n%s\nwant:\n%s", tc.style, got, tc.want)
		}
	}
	if _, err := Parse("x.md", []byte("---\ncitations: tiny\n---\n")); err == nil || !strings.Contains(err.Error(), "citations must be one of") {
		t.Errorf("invalid citations style: %v", err)
	}
}

func TestLandingSections(t *testing.T) {
	raw := "---\nlayout: landing\nsections:\n  - hero\n  - type: markdown\n    title: Why\n    body: Because.\n  - features\n  - type: contributors\n    limit: 12\n  - type: logos\n    items:\n      - image: a.svg\n        name: A\n---\n# Home\n"
	p, err := Parse("documentation/index.md", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, s := range p.Sections {
		types = append(types, s.Type)
	}
	if strings.Join(types, ",") != "hero,markdown,features,contributors,logos" || p.Sections[1].Title != "Why" || p.Sections[3].Limit != 12 {
		t.Fatalf("sections = %+v", p.Sections)
	}

	for want, bad := range map[string]string{
		`unknown type "contributers"`: "  - contributers\n",
		"hero appears twice":          "  - hero\n  - hero\n",
		"markdown needs a body":       "  - type: markdown\n",
		"logos needs items":           "  - logos\n",
		"must be owner/repo":          "  - type: contributors\n    repo: just-a-name\n",
		"limit must be between":       "  - type: contributors\n    limit: 500\n",
		"image is required":           "  - type: logos\n    items:\n      - name: A\n",
	} {
		_, err := Parse("x.md", []byte("---\nsections:\n"+bad+"---\n"))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", strings.TrimSpace(bad), err, want)
		}
	}
}
