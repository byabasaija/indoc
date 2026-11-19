package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/config"
)

func publish(t *testing.T, r *repo, target, path, access string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runPublish(r.cfg, &out, target, path, access); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestPublishApp(t *testing.T) {
	r := newRepo(t)
	r.write("pyproject.toml", "[project]\nname = \"shop\"\ndependencies = [\"django>=5\"]\n")
	r.write("manage.py", "")
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	out := publish(t, r, "app", "guide", "login")
	for _, want := range []string{"Served by Django at /guide/", "def docs(request, path):", "request.user.is_staff", `re_path(r"^guide/(?P<path>.*)$", docs)`, "indoc bundle"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "to .gitignore") {
		t.Error("asked to gitignore .indoc/site, which init already ignores")
	}
	cfg, _ := config.Load(r.cfg)
	if cfg.Publish.Path != "/guide/" || cfg.Publish.Access != "login" || !cfg.FileURLs() {
		t.Errorf("config: %+v urls=%s", cfg.Publish, cfg.URLs)
	}
	// Without flags it repeats the instructions for the saved target.
	if again := publish(t, r, "", "", ""); !strings.Contains(again, "Served by Django at /guide/") {
		t.Errorf("repeat: %s", again)
	}
}

func TestPublishNextUsesPublicDir(t *testing.T) {
	r := newRepo(t)
	r.write("package.json", `{"name":"web","dependencies":{"next":"15"}}`)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	out := publish(t, r, "app", "/docs/", "public")
	cfg, _ := config.Load(r.cfg)
	if cfg.OutputDir != "public/docs" || !strings.Contains(out, "Add public/docs/ to .gitignore") || !strings.Contains(out, "prebuild") {
		t.Errorf("output_dir=%s\n%s", cfg.OutputDir, out)
	}
}

func TestPublishPages(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write(".indoc/config.yaml", r.read(".indoc/config.yaml")+"source_url: https://github.com/Acme/shop/blob/HEAD/{path}#L{line}\n")
	out := publish(t, r, "pages", "", "")
	wf := r.read(pagesWorkflow)
	for _, want := range []string{"actions/deploy-pages", "command: bundle", "fetch-depth: 0", "byabasaija/indoc@v"} {
		if !strings.Contains(wf, want) {
			t.Errorf("workflow lacks %q", want)
		}
	}
	if !strings.Contains(out, "Source: GitHub Actions") || !strings.Contains(out, "https://acme.github.io/shop/") {
		t.Errorf("output: %s", out)
	}
	// An existing workflow is never overwritten.
	os.WriteFile(filepath.Join(r.root, pagesWorkflow), []byte("mine"), 0o644)
	if out := publish(t, r, "pages", "", ""); !strings.Contains(out, "left unchanged") || r.read(pagesWorkflow) != "mine" {
		t.Errorf("workflow overwritten: %s", out)
	}
}

func TestPublishRejectsBadInput(t *testing.T) {
	r := newRepo(t)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	before := r.read(".indoc/config.yaml")
	for _, args := range [][3]string{{"cloud", "", ""}, {"app", "/guide/", "admins"}} {
		if err := runPublish(r.cfg, &bytes.Buffer{}, args[0], args[1], args[2]); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if r.read(".indoc/config.yaml") != before {
		t.Error("rejected settings were saved")
	}
}
