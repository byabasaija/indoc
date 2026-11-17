package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/config"
)

func TestCommandsRegistered(t *testing.T) {
	names := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"init", "scan", "check", "ack", "bundle", "preview", "version"} {
		if !names[want] {
			t.Errorf("command %q not registered", want)
		}
	}
	if names["serve"] {
		t.Error("serve was replaced by preview")
	}
}

type repo struct {
	t    *testing.T
	root string
	cfg  string
}

func newRepo(t *testing.T) *repo {
	r := &repo{t: t, root: t.TempDir()}
	r.cfg = filepath.Join(r.root, config.DefaultPath)
	return r
}

func (r *repo) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.root, path)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) read(path string) string {
	r.t.Helper()
	data, err := os.ReadFile(filepath.Join(r.root, path))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(data)
}

const appPy = `import os

@app.get("/health")
def health():
    return os.getenv("DATABASE_URL")

def admit(patient):
    return patient
`

const admitPage = `---
title: Admitting patients
sources: [app/**]
---
Admission is handled by ` + "`app/main.py::admit`" + `.
`

func TestWorkflow(t *testing.T) {
	r := newRepo(t)
	r.write("go.mod", "module example.com/hospital\n")
	r.write("app/main.py", appPy)

	var out bytes.Buffer
	if err := runInit(r.cfg, &out, nil, "", false); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out.String(), "project: hospital") || !strings.Contains(out.String(), "docs/**/*.md") {
		t.Fatalf("init output:\n%s", out.String())
	}
	if !strings.Contains(r.read(".indoc/.gitignore"), "site/") {
		t.Fatal("init did not gitignore build output")
	}
	if err := runInit(r.cfg, &out, nil, "", false); err == nil {
		t.Fatal("second init without --force must fail")
	}

	r.write("docs/index.md", "# Hospital\n\nOverview of ``app/main.py::health``.\n")
	r.write("docs/admit.md", admitPage)

	out.Reset()
	if err := runCheck(r.cfg, &out, false, false, false); !errors.Is(err, errCheckFailed) {
		t.Fatalf("check on unstamped pages: err=%v\n%s", err, out.String())
	}

	out.Reset()
	if err := runBundle(r.cfg, "", &out); err != nil {
		t.Fatalf("bundle: %v\n%s", err, out.String())
	}
	payload := r.read(".indoc/site/docs.js")
	for _, want := range []string{"Admitting patients", "/health", "DATABASE_URL"} {
		if !strings.Contains(payload, want) {
			t.Errorf("site payload missing %q", want)
		}
	}
	if !strings.Contains(r.read(".indoc/lock.json"), "app/main.py::admit") {
		t.Fatal("bundle did not record pages in the lock")
	}

	out.Reset()
	if err := runCheck(r.cfg, &out, false, false, false); err != nil {
		t.Fatalf("check after bundle: %v\n%s", err, out.String())
	}

	// Signature change: stale, fails CI.
	r.write("app/main.py", strings.Replace(appPy, "def admit(patient)", "def admit(patient, ward)", 1))
	out.Reset()
	err := runCheck(r.cfg, &out, false, false, true)
	if !errors.Is(err, errCheckFailed) {
		t.Fatalf("check after signature change: err=%v", err)
	}
	var report struct {
		Failing  int
		Findings []struct{ Page, Kind, Ref string }
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("check --json: %v\n%s", err, out.String())
	}
	if report.Failing == 0 || report.Findings[0].Kind != "stale" {
		t.Fatalf("report = %+v", report)
	}

	// --warn reports but passes.
	out.Reset()
	if err := runCheck(r.cfg, &out, false, true, false); err != nil {
		t.Fatalf("check --warn: %v", err)
	}

	// ack with no args must not silence it; naming the page does.
	out.Reset()
	if err := runAck(r.cfg, &out, nil); err != nil {
		t.Fatal(err)
	}
	if err := runCheck(r.cfg, &out, false, false, false); err == nil {
		t.Fatal("ack without targets silenced a stale page")
	}
	if err := runAck(r.cfg, &out, []string{"docs/admit.md", "docs/index.md"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runCheck(r.cfg, &out, false, false, false); err != nil {
		t.Fatalf("check after ack: %v\n%s", err, out.String())
	}

	// scan reports pages and reference for the agent.
	out.Reset()
	if err := runScan(r.cfg, &out, true); err != nil {
		t.Fatal(err)
	}
	var scan scanReport
	if err := json.Unmarshal(out.Bytes(), &scan); err != nil {
		t.Fatal(err)
	}
	if scan.ProjectName != "hospital" || len(scan.Pages) != 2 || len(scan.Reference) != 2 || scan.Parsed != 1 {
		t.Fatalf("scan = %+v", scan)
	}
}

func TestBundleRefusesBrokenCitations(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write("docs/index.md", "See `app/main.py::discharge`.\n")
	err := runBundle(r.cfg, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "app/main.py::discharge") {
		t.Fatalf("bundle with broken citation: %v", err)
	}
}

func TestInitWithExistingDocsPublishesNothing(t *testing.T) {
	r := newRepo(t)
	r.write("docs/partner-integration.md", "# Partners\n")
	r.write("docs/code-review-gaps.md", "# Security gaps\n")
	var out bytes.Buffer
	if err := runInit(r.cfg, &out, nil, "", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(r.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Pages) != 0 {
		t.Fatalf("existing docs were published by default: %v", cfg.Pages)
	}
	if !strings.Contains(out.String(), "docs/code-review-gaps.md") {
		t.Fatalf("init did not list existing docs:\n%s", out.String())
	}
	if err := runCheck(r.cfg, &out, false, false, false); err == nil || !strings.Contains(err.Error(), "no pages configured") {
		t.Fatalf("check without pages: %v", err)
	}

	if err := runInit(r.cfg, &out, []string{"docs/partner-integration.md"}, "", true); err != nil {
		t.Fatal(err)
	}
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.read(".indoc/site/docs.js"), "Security gaps") {
		t.Fatal("unselected page was published")
	}
}

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
