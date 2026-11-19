package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestThemeParts(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write("documentation/index.md", admitPage)

	var out bytes.Buffer
	if err := runThemeEject(r.cfg, &out, "footer", false); err != nil {
		t.Fatal(err)
	}
	footer := r.read(".indoc/theme/footer.html")
	if !strings.Contains(footer, "ejected from indoc") || !strings.Contains(footer, `class="site-footer"`) {
		t.Fatalf("ejected footer:\n%s", footer)
	}
	if err := runThemeEject(r.cfg, &out, "footer", false); err == nil {
		t.Error("eject replaced an existing part without --force")
	}
	if err := runThemeEject(r.cfg, &out, "navbar", false); err == nil || !strings.Contains(err.Error(), "parts: header") {
		t.Errorf("unknown part: %v", err)
	}

	r.write(".indoc/theme/footer.html", strings.Replace(footer, `<footer class="site-footer">`, `<footer class="site-footer"><p class="banner">Acme internal</p>`, 1))
	r.write(".indoc/theme/custom.css", ".banner { color: red; }\n")
	r.write(".indoc/config.yaml", r.read(".indoc/config.yaml")+"appearance:\n  font: Inter, sans-serif\n  radius: 4\n  theme: dark\n")
	out.Reset()
	if err := runBundle(r.cfg, "", &out); err != nil {
		t.Fatal(err)
	}
	site := r.site()
	for _, want := range []string{`<p class="banner">Acme internal</p>`, `data-theme="dark"`, `--font:Inter, sans-serif;`, `--radius:4px;`} {
		if !strings.Contains(site, want) {
			t.Errorf("site lacks %s", want)
		}
	}
	if css, _ := os.ReadFile(filepath.Join(r.root, ".indoc/site/assets/custom.css")); !strings.Contains(string(css), ".banner") {
		t.Error("theme/custom.css not used")
	}
	if strings.Contains(out.String(), "warning:") {
		t.Errorf("fresh part reported as outdated: %s", out.String())
	}

	// A part copied from a built-in that has since changed is flagged.
	stale := regexp.MustCompile(`built-in [0-9a-f]{8}`).ReplaceAllString(r.read(".indoc/theme/footer.html"), "built-in 00000000")
	r.write(".indoc/theme/footer.html", stale)
	out.Reset()
	if err := runBundle(r.cfg, "", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "warning: theme part footer was copied from indoc") {
		t.Errorf("outdated part not reported: %s", out.String())
	}

	// A misspelled part stops the build instead of being ignored.
	r.write(".indoc/theme/foter.html", "x")
	if err := runBundle(r.cfg, "", &out); err == nil || !strings.Contains(err.Error(), `"foter" does not exist`) {
		t.Errorf("misspelled part: %v", err)
	}
}

func TestAppearanceValidation(t *testing.T) {
	r := newRepo(t)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	base := r.read(".indoc/config.yaml")
	for _, bad := range []string{"  font: \"x;} body{display:none\"\n", "  radius: 99\n", "  content_width: 100\n", "  theme: blue\n"} {
		r.write(".indoc/config.yaml", base+"appearance:\n"+bad)
		if err := runBundle(r.cfg, "", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "appearance.") {
			t.Errorf("%q accepted: %v", strings.TrimSpace(bad), err)
		}
	}
}
