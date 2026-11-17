package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultPath)
	want := &Config{Version: "1", ProjectName: "shop", Pages: []string{"docs/**/*.md"}, OutputDir: "shop/static/docs"}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectName != want.ProjectName || got.OutputDir != want.OutputDir || len(got.Pages) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadDefaultsOutputDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("project_name: x\n"), 0o644)
	cfg, err := Load(path)
	if err != nil || cfg.OutputDir != DefaultOutputDir {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("want error for missing config")
	}
}

func TestResolve(t *testing.T) {
	p := Resolve("")
	if p.Root != "." || p.Config != DefaultPath || p.Lock != filepath.Join(".indoc", "lock.json") {
		t.Fatalf("%+v", p)
	}
	p = Resolve("/repo/.indoc/config.yaml")
	if p.Root != "/repo" || p.Lock != "/repo/.indoc/lock.json" {
		t.Fatalf("%+v", p)
	}
}

func TestInferProjectName(t *testing.T) {
	cases := map[string]string{
		"go.mod":         "module github.com/byabasaija/indoc\n\ngo 1.27\n",
		"pyproject.toml": "[build-system]\nname = \"wrong\"\n\n[project]\nname = \"shop\"\n",
		"package.json":   `{"name": "@acme/web"}`,
	}
	want := map[string]string{"go.mod": "indoc", "pyproject.toml": "shop", "package.json": "web"}
	for file, content := range cases {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644)
		if got := InferProjectName(dir); got != want[file] {
			t.Errorf("%s: got %q, want %q", file, got, want[file])
		}
	}
	dir := filepath.Join(t.TempDir(), "plain-dir")
	os.Mkdir(dir, 0o755)
	if got := InferProjectName(dir); got != "plain-dir" {
		t.Errorf("fallback: got %q", got)
	}
}

func TestSourceURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:acme/shop.git":   "https://github.com/acme/shop/blob/HEAD/{path}#L{line}",
		"https://github.com/byabasaija/indoc.git":         "https://github.com/byabasaija/indoc/blob/HEAD/{path}#L{line}",
		"https://user:token@gitlab.com/group/sub/app.git": "https://gitlab.com/group/sub/app/-/blob/HEAD/{path}#L{line}",
		"ssh://git@bitbucket.org:22/team/repo":            "https://bitbucket.org/team/repo/src/HEAD/{path}#lines-{line}",
		"git@git.internal.example:team/repo.git":          "",
		"/local/path/repo":                                "",
	}
	for remote, want := range cases {
		if got := sourceTemplate(remote); got != want {
			t.Errorf("sourceTemplate(%q) = %q, want %q", remote, got, want)
		}
	}

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = git@github.com:other/x.git\n[remote \"origin\"]\n\turl = git@github.com:me/app.git\n"), 0o644)
	if got := InferSourceURL(dir); got != "https://github.com/me/app/blob/HEAD/{path}#L{line}" {
		t.Errorf("InferSourceURL = %q", got)
	}

	c := &Config{SourceURL: "https://github.com/me/app/blob/HEAD/{path}#L{line}"}
	if got := c.SourceLink("app/views.py", 42); got != "https://github.com/me/app/blob/HEAD/app/views.py#L42" {
		t.Errorf("SourceLink = %q", got)
	}
	if got := c.SourceLink("app/views.py", 0); got != "https://github.com/me/app/blob/HEAD/app/views.py" {
		t.Errorf("SourceLink without line = %q", got)
	}
	if (&Config{}).SourceLink("a.py", 1) != "" {
		t.Error("SourceLink without template must be empty")
	}
}
