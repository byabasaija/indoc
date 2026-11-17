package pages

import (
	"reflect"
	"testing"
)

func TestParseFrontmatterAndCitations(t *testing.T) {
	raw := []byte(`---
title: How checkout sessions are routed
order: 3
audience: dev
sources:
  - shop/core/services/checkout/**
---

# Ignored heading

Requests land in ` + "`shop/core/services/checkout/dispatcher.py::CheckoutDispatcher.dispatch`" + `,
which calls shop/core/services/checkout/cardpay/handler.py::CardPaymentHandler.

` + "```rust\nuse std::io::Read; // not.rs::a citation\n```" + `

Again: ` + "`shop/core/services/checkout/dispatcher.py::CheckoutDispatcher.dispatch`" + `
`)
	p, err := Parse("docs/dev/checkout.md", raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "How checkout sessions are routed" || !p.HasOrder || p.Order != 3 || p.Audience != "dev" {
		t.Errorf("frontmatter not applied: %+v", p)
	}
	if !reflect.DeepEqual(p.Sources, []string{"shop/core/services/checkout/**"}) {
		t.Errorf("sources = %v", p.Sources)
	}
	want := []Citation{
		{Path: "shop/core/services/checkout/dispatcher.py", Symbol: "CheckoutDispatcher.dispatch", Line: 11},
		{Path: "shop/core/services/checkout/cardpay/handler.py", Symbol: "CardPaymentHandler", Line: 12},
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
	files := []string{"README.md", "docs/a.md", "docs/code-review-gaps.md", "docs/partner/b.md", "docs/x.pdf"}
	got := Match(files, []string{"docs/partner/**/*.md", "docs/a.md"})
	if !reflect.DeepEqual(got, []string{"docs/a.md", "docs/partner/b.md"}) {
		t.Errorf("Match = %v", got)
	}
}

func TestLinkify(t *testing.T) {
	body := "See `app/x.py::A.run` and app/y.go::Serve.\n" +
		"```\napp/x.py::A.run\n```\n" +
		"Already [`app/x.py::A.run`](http://x) linked; unknown `app/z.py::Nope`.\n" +
		"In parentheses (`app/x.py::A.run`) and [text](app/x.py::A.run).\n"
	got := Linkify(body, func(c Citation) string {
		if c.Path == "app/z.py" {
			return ""
		}
		return "https://src/" + c.Path + "#" + c.Symbol
	})
	want := "See [`app/x.py::A.run`](https://src/app/x.py#A.run) and [app/y.go::Serve](https://src/app/y.go#Serve).\n" +
		"```\napp/x.py::A.run\n```\n" +
		"Already [`app/x.py::A.run`](http://x) linked; unknown `app/z.py::Nope`.\n" +
		"In parentheses ([`app/x.py::A.run`](https://src/app/x.py#A.run)) and [text](app/x.py::A.run).\n"
	if got != want {
		t.Errorf("Linkify:\n%s\nwant:\n%s", got, want)
	}
}
