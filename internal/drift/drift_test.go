package drift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/pages"
)

const dispatcher = `class Dispatcher:
    def dispatch(self, session):
        return self.route(session)

    def route(self, session):
        return "banktransfer"
`

const routingPage = `---
title: Routing
sources:
  - app/checkout/**
---
Sessions enter ` + "`app/checkout/dispatcher.py::Dispatcher.dispatch`" + `.
`

const setupPage = `---
title: Setup
sources:
  - app/settings.cfg
---
Configure it.
`

type fixture struct {
	t    *testing.T
	root string
	lock *Lock
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, root: t.TempDir(), lock: NewLock()}
	f.write("app/checkout/dispatcher.py", dispatcher)
	f.write("app/settings.cfg", "timeout=30\n")
	f.write("docs/routing.md", routingPage)
	f.write("docs/setup.md", setupPage)
	f.ack()
	if got := f.check(); len(got) != 0 {
		t.Fatalf("fresh fixture has findings: %+v", got)
	}
	return f
}

func (f *fixture) write(path, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) load() (*Repo, []pages.Page) {
	f.t.Helper()
	r, err := NewRepo(f.root, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	ps, err := pages.Load(f.root, pages.Match(r.Files, []string{"docs/**/*.md"}))
	if err != nil {
		f.t.Fatal(err)
	}
	return r, ps
}

func (f *fixture) check() []Finding {
	r, ps := f.load()
	return Check(r, ps, f.lock)
}

func (f *fixture) ack(targets ...string) AckResult {
	f.t.Helper()
	r, ps := f.load()
	res, err := Ack(r, ps, f.lock, targets)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func expect(t *testing.T, got []Finding, want ...string) {
	t.Helper()
	var kinds []string
	for _, g := range got {
		kinds = append(kinds, g.Kind+" "+g.Page+" "+g.Ref)
	}
	if strings.Join(kinds, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s\n(full: %+v)", strings.Join(kinds, "\n"), strings.Join(want, "\n"), got)
	}
}

func TestCommentsAndFormattingAreIgnored(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", "# routing\n"+strings.Replace(dispatcher, "(self, session):", "(self,  session):  # note", 1))
	expect(t, f.check())
}

func TestUncitedBodyChangeIsDrift(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, `"banktransfer"`, `"cardpay"`, 1))
	got := f.check()
	expect(t, got, "drift docs/routing.md app/checkout/dispatcher.py")
	if got[0].Fails(false) || !got[0].Fails(true) {
		t.Error("drift must fail only in strict mode")
	}
}

func TestSignatureChangeIsStale(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, "def route(self, session)", "def route(self, session, provider)", 1))
	got := f.check()
	expect(t, got, "stale docs/routing.md app/checkout/dispatcher.py")
	if got[0].Detail != "signature changed: Dispatcher.route" {
		t.Errorf("detail = %q", got[0].Detail)
	}
}

func TestCitedBodyChangeIsDriftUnlessStrict(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, "return self.route(session)", "return None", 1))
	got := f.check()
	expect(t, got,
		"drift docs/routing.md app/checkout/dispatcher.py",
		"drift docs/routing.md app/checkout/dispatcher.py::Dispatcher.dispatch")
	if got[1].Fails(false) {
		t.Error("cited body change on a normal page must not fail check")
	}

	f.write("docs/routing.md", strings.Replace(routingPage, "title: Routing", "title: Routing\nstrict: true", 1))
	f.ack()
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, "return self.route(session)", "return False", 1))
	expect(t, f.check(),
		"drift docs/routing.md app/checkout/dispatcher.py",
		"stale docs/routing.md app/checkout/dispatcher.py::Dispatcher.dispatch")
}

func TestRenamedCitationIsBroken(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, "def dispatch", "def handle", 1))
	expect(t, f.check(),
		"stale docs/routing.md app/checkout/dispatcher.py",
		"broken docs/routing.md app/checkout/dispatcher.py::Dispatcher.dispatch")
	r, ps := f.load()
	if _, err := Ack(r, ps, f.lock, []string{"docs/routing.md"}); err == nil {
		t.Error("ack must refuse a page with broken citations")
	}
}

func TestNewAndRemovedFilesAreStale(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/cardpay.py", "def handle():\n    pass\n")
	expect(t, f.check(), "stale docs/routing.md app/checkout/cardpay.py")

	f.ack("docs/routing.md")
	os.Remove(filepath.Join(f.root, "app/checkout/cardpay.py"))
	expect(t, f.check(), "stale docs/routing.md app/checkout/cardpay.py")
}

func TestUnparsedFileChangeIsDrift(t *testing.T) {
	f := newFixture(t)
	f.write("app/settings.cfg", "timeout=60\n")
	expect(t, f.check(), "drift docs/setup.md app/settings.cfg")
}

func TestEditedPageNeedsAck(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, "def route(self, session)", "def route(self, session, provider)", 1))
	f.write("docs/routing.md", routingPage+"\nRoutes take a provider now.\n")
	expect(t, f.check(), "edited docs/routing.md ")

	res := f.ack()
	if strings.Join(res.Stamped, ",") != "docs/routing.md" {
		t.Errorf("stamped %v", res.Stamped)
	}
	expect(t, f.check())
}

func TestAckWithoutTargetsNeverSilencesUnchangedStalePage(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", strings.Replace(dispatcher, "def route(self, session)", "def route(self, session, provider)", 1))
	if res := f.ack(); len(res.Stamped) != 0 {
		t.Fatalf("ack with no targets stamped unchanged pages: %v", res.Stamped)
	}
	expect(t, f.check(), "stale docs/routing.md app/checkout/dispatcher.py")

	f.ack("docs/routing.md") // reviewed: still correct
	expect(t, f.check())
}

func TestUnstampedRemovedAndUnanchoredPages(t *testing.T) {
	f := newFixture(t)
	f.write("docs/faq.md", "# FAQ\n")
	os.Remove(filepath.Join(f.root, "docs/setup.md"))
	expect(t, f.check(),
		"unanchored docs/faq.md ",
		"unstamped docs/faq.md ",
		"removed docs/setup.md ")

	res := f.ack()
	if strings.Join(res.Dropped, ",") != "docs/setup.md" {
		t.Errorf("dropped %v", res.Dropped)
	}
	expect(t, f.check(), "unanchored docs/faq.md ")
}

func TestSourcesMatchingNothingIsBroken(t *testing.T) {
	f := newFixture(t)
	f.write("docs/typo.md", "---\nsources: [app/usd/**]\n---\nx\n")
	f.ack()
	expect(t, f.check(), "broken docs/typo.md app/usd/**")
}

func TestLockRoundTrip(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.root, ".indoc/lock.json")
	if err := f.lock.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	r, ps := f.load()
	if got := Check(r, ps, loaded); len(got) != 0 {
		t.Fatalf("round-tripped lock has findings: %+v", got)
	}
	missing, err := LoadLock(filepath.Join(f.root, "nope.json"))
	if err != nil || len(missing.Pages) != 0 {
		t.Fatalf("missing lock should be empty, got %v %v", missing, err)
	}
}

func TestPrivateHelpersAreDrift(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/dispatcher.py", dispatcher+"\n    def _hash(self, x):\n        return x\n\n    def __repr__(self):\n        return 'd'\n")
	got := f.check()
	expect(t, got, "stale docs/routing.md app/checkout/dispatcher.py")
	if got[0].Detail != "added: Dispatcher.__repr__" {
		t.Errorf("dunder methods are public; detail = %q", got[0].Detail)
	}

	f.ack("docs/routing.md")
	f.write("app/checkout/dispatcher.py", dispatcher+"\n    def _hash(self, x, salt):\n        return x\n\n    def __repr__(self):\n        return 'd'\n")
	got = f.check()
	expect(t, got, "drift docs/routing.md app/checkout/dispatcher.py")
	if got[0].Detail != "private helpers changed: signature changed: Dispatcher._hash" {
		t.Errorf("detail = %q", got[0].Detail)
	}
}

func TestTestFileChangesAreDrift(t *testing.T) {
	f := newFixture(t)
	f.write("app/checkout/test_dispatcher.py", "def test_route():\n    pass\n")
	f.ack("docs/routing.md")
	f.write("app/checkout/test_dispatcher.py", "def test_route():\n    pass\n\ndef test_new(x):\n    pass\n")
	got := f.check()
	expect(t, got, "drift docs/routing.md app/checkout/test_dispatcher.py")
	if got[0].Detail != "tests changed" {
		t.Errorf("detail = %q", got[0].Detail)
	}
}
