package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/reference"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// site returns every HTML page of the built site, concatenated.
func (r *repo) site() string {
	r.t.Helper()
	var b strings.Builder
	filepath.WalkDir(filepath.Join(r.root, ".indoc/site"), func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".html") {
			data, _ := os.ReadFile(p)
			b.Write(data)
		}
		return nil
	})
	if b.Len() == 0 {
		r.t.Fatal("no HTML pages were built")
	}
	return b.String()
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
	if !strings.Contains(out.String(), "project: hospital") || !strings.Contains(out.String(), "documentation/**/*.md") {
		t.Fatalf("init output:\n%s", out.String())
	}
	if !strings.Contains(r.read(".indoc/.gitignore"), "site/") {
		t.Fatal("init did not gitignore build output")
	}
	if err := runInit(r.cfg, &out, nil, "", false); err == nil {
		t.Fatal("second init without --force must fail")
	}

	r.write("documentation/index.md", "# Hospital\n\nOverview of ``app/main.py::health``.\n")
	r.write("documentation/admit.md", admitPage)

	out.Reset()
	if err := runCheck(r.cfg, &out, false, false, false); !errors.Is(err, errCheckFailed) {
		t.Fatalf("check on unstamped pages: err=%v\n%s", err, out.String())
	}

	out.Reset()
	if err := runBundle(r.cfg, "", &out); err != nil {
		t.Fatalf("bundle: %v\n%s", err, out.String())
	}
	payload := r.site()
	for _, want := range []string{"Admitting patients", "/health", "DATABASE_URL"} {
		if !strings.Contains(payload, want) {
			t.Errorf("site missing %q", want)
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
	if err := runAck(r.cfg, &out, []string{"documentation/admit.md", "documentation/index.md"}); err != nil {
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
	r.write("documentation/index.md", "See `app/main.py::discharge`.\n")
	err := runBundle(r.cfg, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "app/main.py::discharge") {
		t.Fatalf("bundle with broken citation: %v", err)
	}
}

func TestInitWithExistingDocsPublishesNothing(t *testing.T) {
	r := newRepo(t)
	r.write("documentation/partner-integration.md", "# Partners\n")
	r.write("documentation/code-review-gaps.md", "# Security gaps\n")
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
	if !strings.Contains(out.String(), "documentation/code-review-gaps.md") {
		t.Fatalf("init did not list existing docs:\n%s", out.String())
	}
	if err := runCheck(r.cfg, &out, false, false, false); err == nil || !strings.Contains(err.Error(), "no pages configured") {
		t.Fatalf("check without pages: %v", err)
	}

	if err := runInit(r.cfg, &out, []string{"documentation/partner-integration.md"}, "", true); err != nil {
		t.Fatal(err)
	}
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.site(), "Security gaps") {
		t.Fatal("unselected page was published")
	}
}

// docs/ full of plans and agent notes must not decide where pages go, or be
// published.
func TestInitIgnoresNotesInDocs(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	r.write("docs/plans/migration-plan.md", "# Plan\n")
	r.write("docs/ai-design.md", "# Internal design\n")
	var out bytes.Buffer
	if err := runInit(r.cfg, &out, nil, "", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(r.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Pages) != 1 || cfg.Pages[0] != PagesDir+"/**/*.md" {
		t.Fatalf("pages = %v, want %s/**/*.md", cfg.Pages, PagesDir)
	}
	if !strings.Contains(out.String(), "docs/ai-design.md") {
		t.Fatalf("init did not list unpublished Markdown:\n%s", out.String())
	}
	r.write(PagesDir+"/index.md", "# App\n\nSee `app/main.py::health`.\n")
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if site := r.site(); strings.Contains(site, "Internal design") || strings.Contains(site, "Plan") {
		t.Fatal("notes under docs/ were published")
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

func TestBundleLinksCitationsAndSources(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write(".indoc/config.yaml", r.read(".indoc/config.yaml")+"source_url: https://example.test/{path}#L{line}\n")
	r.write("documentation/index.md", admitPage)
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	payload := r.site()
	for _, want := range []string{
		`href="https://example.test/app/main.py#L7"`, // citation -> def admit
		`href="https://example.test/app/main.py#L3"`, // route source -> @app.get
	} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload missing %s", want)
		}
	}
}

// TestCobraExtractionMatchesCommandTree checks the static Cobra extractor
// against indoc's own command tree at runtime.
func TestCobraExtractionMatchesCommandTree(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	files, err := code.ListFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := reference.Extract(root, files)
	if err != nil {
		t.Fatal(err)
	}
	static := map[string]string{}
	for _, s := range rep.Sections {
		if s.Kind == "commands" {
			for _, e := range s.Entries {
				static[e.Name] = flagNames(e.Detail)
			}
		}
	}

	runtime := map[string]string{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == "help" || c.Name() == "completion" {
			return
		}
		var names []string
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name != "help" {
				names = append(names, f.Name)
			}
		})
		sort.Strings(names)
		runtime[c.CommandPath()] = strings.Join(names, " ")
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)

	if len(static) != len(runtime) {
		t.Errorf("commands: static %d, runtime %d\nstatic: %v\nruntime: %v", len(static), len(runtime), static, runtime)
	}
	for path, flags := range runtime {
		if static[path] != flags {
			t.Errorf("%s: static flags %q, runtime %q", path, static[path], flags)
		}
	}
}

// flagNames turns "-o/--out --json (global)" into "json out".
func flagNames(detail string) string {
	var names []string
	for _, f := range strings.Fields(detail) {
		if f == "(global)" {
			continue
		}
		if i := strings.LastIndex(f, "--"); i >= 0 {
			names = append(names, f[i+2:])
		}
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

func TestTypeScriptStaleness(t *testing.T) {
	r := newRepo(t)
	const api = "export async function createPayment(amount: number): Promise<string> {\n  return charge(amount);\n}\n\nfunction charge(n: number) {\n  return String(n);\n}\n"
	r.write("src/payments.ts", api)
	if err := runInit(r.cfg, &bytes.Buffer{}, nil, "", false); err != nil {
		t.Fatal(err)
	}
	r.write("documentation/index.md", "---\nsources: [src/**]\n---\nPayments start in `src/payments.ts::createPayment`.\n")
	if err := runBundle(r.cfg, "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runCheck(r.cfg, &out, false, false, false); err != nil {
		t.Fatalf("clean check: %v\n%s", err, out.String())
	}

	// A new private helper is drift: check passes.
	r.write("src/payments.ts", api+"\nfunction audit() {}\n")
	out.Reset()
	if err := runCheck(r.cfg, &out, false, false, false); err != nil || !strings.Contains(out.String(), "DRIFT") {
		t.Fatalf("private helper: err=%v\n%s", err, out.String())
	}

	// A signature change on the cited, exported function fails.
	r.write("src/payments.ts", strings.Replace(api, "amount: number", "amount: number, currency: string", 1))
	out.Reset()
	if err := runCheck(r.cfg, &out, false, false, false); !errors.Is(err, errCheckFailed) ||
		!strings.Contains(out.String(), "src/payments.ts::createPayment") {
		t.Fatalf("signature change: err=%v\n%s", err, out.String())
	}
}

// TestMCPWorkflow drives `indoc mcp` the way an agent would: scan sets the
// project up, the agent writes a page, bundle and check run through tools.
func TestMCPWorkflow(t *testing.T) {
	r := newRepo(t)
	r.write("app/main.py", appPy)
	call := func(id int, tool string, args map[string]any) map[string]any {
		t.Helper()
		args["root"] = r.root
		req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{"name": tool, "arguments": args}})
		var out bytes.Buffer
		if err := mcpServer().Serve(bytes.NewReader(append(req, '\n')), &out); err != nil {
			t.Fatal(err)
		}
		var resp map[string]any
		if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
			t.Fatalf("%s: %v\n%s", tool, err, out.String())
		}
		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s: %v", tool, resp)
		}
		return result
	}
	text := func(res map[string]any) string {
		return res["content"].([]any)[0].(map[string]any)["text"].(string)
	}

	// First scan: sets up, no pages yet.
	res := call(1, "indoc_scan", map[string]any{})
	if res["isError"] == true || !strings.Contains(text(res), "Set up indoc") || !strings.Contains(text(res), "propose readers") {
		t.Fatalf("first scan: %s", text(res))
	}
	if _, err := os.Stat(r.cfg); err != nil {
		t.Fatal("scan did not create the config")
	}

	r.write("documentation/index.md", admitPage)
	res = call(2, "indoc_scan", map[string]any{})
	scan := res["structuredContent"].(map[string]any)
	if len(scan["pages"].([]any)) != 1 || scan["map"] == nil {
		t.Fatalf("scan: %v", scan)
	}

	res = call(3, "indoc_bundle", map[string]any{})
	if res["isError"] == true || !strings.Contains(text(res), "Wrote 1 pages") {
		t.Fatalf("bundle: %s", text(res))
	}
	res = call(4, "indoc_check", map[string]any{})
	if res["structuredContent"].(map[string]any)["failing"].(float64) != 0 {
		t.Fatalf("check after bundle: %s", text(res))
	}

	r.write("app/main.py", strings.Replace(appPy, "def admit(patient)", "def admit(patient, ward)", 1))
	res = call(5, "indoc_check", map[string]any{})
	if res["isError"] == true || res["structuredContent"].(map[string]any)["failing"].(float64) != 2 {
		t.Fatalf("check after signature change: %s", text(res))
	}
	res = call(6, "indoc_ack", map[string]any{"pages": []string{"documentation/index.md"}})
	if res["isError"] == true {
		t.Fatalf("ack: %s", text(res))
	}
	if res = call(7, "indoc_check", map[string]any{}); res["structuredContent"].(map[string]any)["failing"].(float64) != 0 {
		t.Fatalf("check after ack: %s", text(res))
	}
}

func TestMCPSetupWithExistingDocs(t *testing.T) {
	r := newRepo(t)
	r.write("documentation/partners.md", "# Partners\n")
	r.write("documentation/review-notes.md", "# Internal\n")
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"indoc_scan","arguments":{"root":"` + r.root + `"}}}` + "\n"
	var out bytes.Buffer
	mcpServer().Serve(strings.NewReader(req), &out)
	if !strings.Contains(out.String(), "documentation/review-notes.md") || !strings.Contains(out.String(), "Ask the user which") {
		t.Fatalf("existing docs must be listed and the user asked: %s", out.String())
	}
	req = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"indoc_setup","arguments":{"root":"` + r.root + `","pages":["documentation/partners.md"]}}}` + "\n"
	out.Reset()
	mcpServer().Serve(strings.NewReader(req), &out)
	cfg, err := config.Load(r.cfg)
	if err != nil || len(cfg.Pages) != 1 || cfg.Pages[0] != "documentation/partners.md" {
		t.Fatalf("setup did not set pages: %+v %v\n%s", cfg, err, out.String())
	}
}
