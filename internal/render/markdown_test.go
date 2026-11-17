package render

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	src := "# Getting started\n\nSee [writing](writing-pages.md#citations) and [site](https://example.com).\n\n" +
		"![diagram](img/flow.png)\n\n## Install\n\n## Install\n\n```sh\ngo install x\n```\n\n```mermaid\ngraph TD; A-->B\n```\n\n<script>alert(1)</script>\n"
	res, err := Render(src, Options{
		Link: func(d string) string {
			if strings.HasPrefix(d, "writing-pages.md") {
				return "../writing-pages/" + strings.TrimPrefix(d, "writing-pages.md")
			}
			return ""
		},
		Image: func(s string) string { return "assets/img/" + s[strings.LastIndex(s, "/")+1:] },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<h1 id="getting-started">`, `<h2 id="install">`, `<h2 id="install-2">`,
		`href="../writing-pages/#citations"`, `href="https://example.com"`,
		`src="assets/img/flow.png"`, `<code class="language-sh">`,
	} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("missing %q in\n%s", want, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "<script>") {
		t.Error("raw HTML must be dropped unless allowed")
	}
	if !res.UsesMermaid || len(res.Headings) != 3 || !strings.Contains(res.Text, "go install x") {
		t.Errorf("result = %+v", res)
	}
	allowed, _ := Render("<div class=\"x\">hi</div>\n", Options{AllowHTML: true})
	if !strings.Contains(allowed.HTML, `<div class="x">`) {
		t.Error("AllowHTML must pass raw HTML through")
	}
	if got := Inline("Built by **us**"); got != "Built by <strong>us</strong>" {
		t.Errorf("Inline = %q", got)
	}
}
