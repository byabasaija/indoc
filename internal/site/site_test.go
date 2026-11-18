package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/pages"
)

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
	return err == nil
}

func build(t *testing.T, s *Site) string {
	t.Helper()
	out := t.TempDir()
	if err := Build(s, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func basicSite(t *testing.T) *Site {
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "docs", "img"), 0o755)
	os.WriteFile(filepath.Join(repo, "docs", "img", "flow.png"), []byte("png"), 0o644)
	return &Site{
		Name:     "demo",
		RepoRoot: repo,
		Pages: []Page{
			{Source: "docs/index.md", Title: "Demo", Markdown: "# Demo\n\nSee [setup](guides/setup.md#install).\n"},
			{Source: "docs/guides/setup.md", Title: "Setup", Order: 1, HasOrder: true,
				Markdown: "# Setup\n\n## Install\n\n```sh\nmake\n```\n\n![flow](../img/flow.png)\n\nBack [home](../index.md).\n"},
			{Source: "docs/guides/index.md", Title: "Guides", Markdown: "# Guides\n"},
			{Source: "docs/faq.md", Title: "FAQ", Markdown: "# FAQ\n"},
		},
	}
}

func TestLayoutAndLinks(t *testing.T) {
	out := build(t, basicSite(t))
	for _, f := range []string{"index.html", "guides/index.html", "guides/setup/index.html", "faq/index.html", "404.html",
		"assets/indoc.css", "assets/indoc.js", "assets/prism.js", "assets/search-index.js"} {
		if !exists(out, f) {
			t.Errorf("missing %s", f)
		}
	}
	home := read(t, out, "index.html")
	if !strings.Contains(home, `href="guides/setup/#install"`) {
		t.Errorf("markdown link not rewritten to page URL:\n%s", home)
	}
	setup := read(t, out, "guides/setup/index.html")
	for _, want := range []string{
		`href="../../assets/indoc.css"`, // relative, works from file://
		`href="../../"`,                 // ../index.md
		`<h2 id="install">`,
		`class="language-sh"`,
		`assets/img/flow-`,
		`<p class="nav-title">Guides</p>`, // folder index names the category
	} {
		if !strings.Contains(setup, want) {
			t.Errorf("setup page lacks %q", want)
		}
	}
	if strings.Contains(setup, "mermaid") {
		t.Error("mermaid shipped on a page without diagrams")
	}
	if exists(out, "assets/mermaid.min.js") {
		t.Error("mermaid written for a site without diagrams")
	}
	// Category index comes before its pages; top-level pages before categories.
	nav := setup[strings.Index(setup, `class="sidebar"`):]
	faq, guides, step := strings.Index(nav, ">FAQ<"), strings.Index(nav, ">Guides<"), strings.Index(nav, ">Setup<")
	if !(faq < guides && guides < step) {
		t.Errorf("nav order wrong: faq=%d guides=%d setup=%d", faq, guides, step)
	}
	idx := read(t, out, "assets/search-index.js")
	if !strings.HasPrefix(idx, "window.INDOC_SEARCH") || !strings.Contains(idx, `"guides/setup/"`) {
		t.Errorf("search index: %.200s", idx)
	}
}

func TestMermaidOnlyWhenUsed(t *testing.T) {
	s := basicSite(t)
	s.Pages[3].Markdown = "# FAQ\n\n```mermaid\ngraph TD; A-->B\n```\n"
	out := build(t, s)
	if !exists(out, "assets/mermaid.min.js") {
		t.Fatal("mermaid not written")
	}
	if !strings.Contains(read(t, out, "faq/index.html"), "mermaid.min.js") {
		t.Error("diagram page does not load mermaid")
	}
	if strings.Contains(read(t, out, "guides/setup/index.html"), "mermaid.min.js") {
		t.Error("page without diagrams loads mermaid")
	}
}

func TestLanding(t *testing.T) {
	s := basicSite(t)
	s.Pages[0].Layout = "landing"
	s.Pages[0].Hero = &pages.Hero{Title: "Demo", Tagline: "Docs that **check**",
		Actions: []pages.Action{{Text: "Start", Link: "guides/setup.md"}, {Text: "FAQ", Link: "faq.md"}}}
	s.Pages[0].Features = []pages.Feature{{Title: "Fast", Details: "Very", Link: "faq.md"}}
	s.Pages[0].Featured = []pages.Action{{Text: "Setup", Link: "guides/setup.md"}}
	out := build(t, s)
	home := read(t, out, "index.html")
	for _, want := range []string{
		`class="button primary" href="guides/setup/"`,
		`class="button secondary" href="faq/"`,
		`Docs that <strong>check</strong>`,
		`Learn more`,
		`Featured`,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("landing lacks %q", want)
		}
	}
}

func TestReferenceTables(t *testing.T) {
	s := basicSite(t)
	s.Reference = []doc.RefSection{
		{ID: "routes", Title: "Routes", Kind: doc.KindRoutes, Entries: []doc.RefEntry{
			{Name: "/users/<id>", Detail: "GET, DELETE", Handler: "users.views.detail", Source: "users/urls.py:4", URL: "https://x/users/urls.py#L4"}}},
		{ID: "commands", Title: "Commands", Kind: doc.KindCommands, Entries: []doc.RefEntry{
			{Name: "tool init", Detail: "-f/--force --config (global)"}}},
	}
	out := build(t, s)
	routes := read(t, out, "reference/routes/index.html")
	for _, want := range []string{"method-get", "method-delete", "&lt;id&gt;", "users.views.detail", `href="https://x/users/urls.py#L4"`} {
		if !strings.Contains(routes, want) {
			t.Errorf("routes page lacks %q", want)
		}
	}
	cmds := read(t, out, "reference/commands/index.html")
	if !strings.Contains(cmds, "<span>-f/--force</span>") || !strings.Contains(cmds, "<span>--config (global)</span>") {
		t.Error("command flags are not kept whole")
	}
	if !strings.Contains(read(t, out, "index.html"), `>Reference<`) {
		t.Error("reference group missing from nav")
	}
}

func TestStaleFilesRemoved(t *testing.T) {
	s := basicSite(t)
	out := t.TempDir()
	if err := Build(s, out); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(out, "mine.txt"), []byte("user file"), 0o644)
	s.Pages = s.Pages[:3] // faq.md deleted
	if err := Build(s, out); err != nil {
		t.Fatal(err)
	}
	if exists(out, "faq/index.html") {
		t.Error("page of a deleted source survived")
	}
	if !exists(out, "mine.txt") {
		t.Error("a file indoc did not write was removed")
	}
}

func TestLegacyViewerRemoved(t *testing.T) {
	out := t.TempDir()
	os.WriteFile(filepath.Join(out, "docs.js"), []byte(`window.INDOC_DOCS = {}`), 0o644)
	os.WriteFile(filepath.Join(out, "app.js"), []byte(`old`), 0o644)
	os.WriteFile(filepath.Join(out, "style.css"), []byte(`old`), 0o644)
	if err := Build(basicSite(t), out); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"docs.js", "app.js", "style.css"} {
		if exists(out, f) {
			t.Errorf("legacy %s survived", f)
		}
	}

	// Without the old payload, same-named files belong to someone else.
	out = t.TempDir()
	os.WriteFile(filepath.Join(out, "style.css"), []byte(`mine`), 0o644)
	if err := Build(basicSite(t), out); err != nil {
		t.Fatal(err)
	}
	if !exists(out, "style.css") {
		t.Error("a user's style.css was removed")
	}
}

func TestLogoIsFavicon(t *testing.T) {
	s := basicSite(t)
	os.WriteFile(filepath.Join(s.RepoRoot, "logo.svg"), []byte("<svg/>"), 0o644)
	s.Logo = "logo.svg"
	out := build(t, s)
	page := read(t, out, "guides/setup/index.html")
	if !strings.Contains(page, `<link rel="icon" href="../../assets/logo-`) {
		t.Errorf("logo not used as favicon")
	}
}

func TestLandingSectionOrder(t *testing.T) {
	s := basicSite(t)
	os.WriteFile(filepath.Join(s.RepoRoot, "docs", "img", "acme.svg"), []byte("<svg/>"), 0o644)
	s.Repo = "owner/project"
	home := &s.Pages[0]
	home.Layout = "landing"
	home.Hero = &pages.Hero{Title: "Demo"}
	home.Features = []pages.Feature{{Title: "Fast"}}
	home.Markdown = "Body text."
	home.Sections = []pages.Section{
		{Type: "hero"},
		{Type: "logos", Title: "Used by", Items: []pages.LogoItem{
			{Image: "img/acme.svg", Name: "Acme", Link: "https://acme.test"},
			{Image: "https://cdn.test/beta.png", Name: "Beta"},
		}},
		{Type: "markdown", Title: "Why", Body: "See [setup](guides/setup.md)."},
		{Type: "features"},
		{Type: "contributors"},
	}
	out := build(t, s)
	page := read(t, out, "index.html")
	order := []string{`class="hero"`, `Used by`, `class="section-title">Why`, `class="features"`, `data-contributors="owner/project"`, `Body text.`}
	last := -1
	for _, mark := range order {
		i := strings.Index(page, mark)
		if i < 0 {
			t.Fatalf("missing %q", mark)
		}
		if i < last {
			t.Errorf("%q out of order", mark)
		}
		last = i
	}
	for _, want := range []string{
		`<a href="https://acme.test" title="Acme"><img src="assets/img/acme-`,
		`<img src="https://cdn.test/beta.png" alt="Beta"`,
		`href="guides/setup/"`, // links in a markdown section resolve like the body's
		`data-limit="48"`,      // default
		`>Contributors</h2>`,   // default title
		`github.com/owner/project/graphs/contributors`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("landing lacks %q", want)
		}
	}
	if !strings.Contains(read(t, out, "assets/search-index.js"), "See setup") {
		t.Error("markdown section text is not searchable")
	}
}

func TestLandingSectionErrors(t *testing.T) {
	s := basicSite(t)
	s.Pages[0].Layout = "landing"
	s.Pages[0].Sections = []pages.Section{{Type: "contributors"}}
	if err := Build(s, t.TempDir()); err == nil || !strings.Contains(err.Error(), "contributors needs repo") {
		t.Errorf("contributors without a GitHub repo: %v", err)
	}
	s.Pages[0].Sections = []pages.Section{{Type: "logos", Items: []pages.LogoItem{{Image: "img/missing.svg"}}}}
	if err := Build(s, t.TempDir()); err == nil || !strings.Contains(err.Error(), `image "img/missing.svg" not found`) {
		t.Errorf("missing logo image: %v", err)
	}
}
