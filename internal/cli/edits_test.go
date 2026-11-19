package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/site"
)

func TestChangedRules(t *testing.T) {
	orig := "a { color: red; }\n@media (max-width: 9px) { b { x: 1; } }\n.c { y: 2; }"
	mine := "a {\n  color: red;\n}\n/* mine */\n@media (max-width: 9px) { b { x: 2; } }\n.c { y: 2; }\n.new { z: 3; }"
	got := changedRules(orig, mine)
	want := []string{"@media (max-width: 9px) { b { x: 2; } }", ".new { z: 3; }"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changedRules = %q, want %q", got, want)
	}
}

func TestHandEditsInOutput(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write("documentation/index.md", admitPage)
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	css := ".indoc/site/assets/indoc.css"
	builtin := r.read(css)
	r.write(css, builtin+"\n.topbar { background: hotpink; }\n")
	r.write(".indoc/site/index.html", strings.Replace(r.read(".indoc/site/index.html"), "</body>", "<p>my note</p></body>", 1))

	err := runBundle(r.cfg, "", &bytes.Buffer{})
	var edited *site.EditedError
	if !errors.As(err, &edited) || len(edited.Files) != 2 {
		t.Fatalf("hand edits not detected: %v", err)
	}
	if !strings.Contains(r.read(css), "hotpink") {
		t.Fatal("refused build still overwrote the edit")
	}

	var out bytes.Buffer
	if err := runBundleEdits(r.cfg, "", "keep", &out); err != nil {
		t.Fatal(err)
	}
	if theme := r.read(".indoc/theme/custom.css"); !strings.Contains(theme, ".topbar { background: hotpink; }") || strings.Count(theme, "{") != 1 {
		t.Errorf("theme/custom.css should hold only the new rule:\n%s", theme)
	}
	if !strings.Contains(r.read(".indoc/theme/edited/index.html"), "<p>my note</p>") {
		t.Error("HTML edit not saved")
	}
	if r.read(css) != builtin {
		t.Error("output stylesheet not regenerated")
	}
	if !strings.Contains(r.read(".indoc/site/assets/custom.css"), "hotpink") {
		t.Error("kept rule not applied to the site")
	}
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Errorf("rebuild after keeping: %v", err)
	}

	// discard overwrites.
	r.write(css, "body { display: none; }")
	if err := runBundleEdits(r.cfg, "", "discard", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if r.read(css) != builtin {
		t.Error("discard did not overwrite")
	}
}

func TestOldManifestIsNotAnEdit(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write("documentation/index.md", admitPage)
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// A manifest from before hashes: names only.
	m := filepath.Join(r.root, ".indoc/site/.indoc-site")
	data, _ := os.ReadFile(m)
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		_, name, _ := strings.Cut(line, "  ")
		names = append(names, name)
	}
	os.WriteFile(m, []byte(strings.Join(names, "\n")+"\n"), 0o644)
	r.write(".indoc/site/assets/indoc.css", "changed")
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Errorf("old manifest caused a refusal: %v", err)
	}
}
