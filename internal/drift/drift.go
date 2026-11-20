// Package drift records what each documentation page was last verified
// against (the lock) and reports pages whose code has changed since.
//
// Rules (ROADMAP section 11), measured on real history (section 12):
//
//	cited symbol's signature changed           stale  (check fails)
//	surface changed under sources globs        stale  (check fails)
//	cited symbol's body changed                drift  (fails on strict pages, or with --strict)
//	body-only change under sources globs       drift  (fails with --strict)
//	any change to a test file under sources    drift  (fails with --strict)
//	comments, docstrings, logging, formatting  ignored
//
// A page is compared only while its content matches the lock. Editing a page
// makes it "edited" until re-stamped, and nothing re-stamps an unchanged page
// except an explicit ack.
package drift

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/glob"
	"github.com/byabasaija/indoc/internal/pages"
)

// LockVersion is the lock file format version.
const LockVersion = 1

// Lock is the committed record of what each page was verified against.
type Lock struct {
	Version int                 `json:"version"`
	Pages   map[string]PageLock `json:"pages"`
}

// PageLock is one page's verified state.
type PageLock struct {
	Page      string                `json:"page"`
	Citations map[string]SymbolLock `json:"citations,omitempty"`
	Sources   map[string]FileLock   `json:"sources,omitempty"`
}

// SymbolLock is a cited symbol's hashes.
type SymbolLock struct {
	Surface string `json:"surface"`
	Body    string `json:"body"`
	Raw     string `json:"raw,omitempty"`    // the file's exact bytes
	Hasher  string `json:"hasher,omitempty"` // how Surface and Body were computed
}

// FileLock is a covered file's hashes. Symbols maps qualified names to
// surface hashes and is present only for parsed languages.
type FileLock struct {
	Body    string            `json:"body"`
	Symbols map[string]string `json:"symbols,omitempty"`
	Private []string          `json:"private,omitempty"` // private symbol keys, so removed ones are classified too
	Raw     string            `json:"raw,omitempty"`     // the file's exact bytes
	Hasher  string            `json:"hasher,omitempty"`  // how Body and Symbols were computed
}

// Hashes from a different hasher (a newer grammar or different
// normalization rules) cannot be compared. If the file's bytes are unchanged
// nothing changed; otherwise the change is reported as drift, never stale.
const olderLock = "changed; lock was written by a different indoc version, so signatures were not compared (run indoc ack after reviewing)"

// NewLock returns an empty lock.
func NewLock() *Lock { return &Lock{Version: LockVersion, Pages: map[string]PageLock{}} }

// LoadLock reads a lock file. A missing file is an empty lock.
func LoadLock(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewLock(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if l.Version != LockVersion {
		return nil, fmt.Errorf("%s: unsupported lock version %d", path, l.Version)
	}
	if l.Pages == nil {
		l.Pages = map[string]PageLock{}
	}
	return &l, nil
}

// Save writes the lock deterministically.
func (l *Lock) Save(path string) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Repo reads and analyzes files under Root, caching results.
type Repo struct {
	Root  string
	Files []string // from code.ListFiles
	cache map[string]*analysis
}

type analysis struct {
	file code.File
	ok   bool
}

// NewRepo lists files under root.
func NewRepo(root string, exclude []string) (*Repo, error) {
	files, err := code.ListFiles(root, exclude)
	if err != nil {
		return nil, err
	}
	return &Repo{Root: root, Files: files, cache: map[string]*analysis{}}, nil
}

// Analyze returns the analysis of a repository file; ok is false when it
// cannot be read.
func (r *Repo) Analyze(path string) (code.File, bool) {
	if a, hit := r.cache[path]; hit {
		return a.file, a.ok
	}
	src, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(path)))
	a := &analysis{ok: err == nil}
	if a.ok {
		a.file = code.Analyze(path, src)
	}
	r.cache[path] = a
	return a.file, a.ok
}

// covered returns repository files matched by a page's sources, excluding
// documentation pages themselves.
func (r *Repo) covered(p pages.Page, pageSet map[string]bool) []string {
	var out []string
	for _, f := range r.Files {
		if pageSet[f] {
			continue
		}
		for _, g := range p.Sources {
			if glob.Match(g, f) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Finding kinds.
const (
	Stale      = "stale"      // code the page describes changed
	Drift      = "drift"      // implementation changed under covered files
	Broken     = "broken"     // a citation or sources glob does not resolve
	Unverified = "unverified" // citation into a language without a parser
	Unstamped  = "unstamped"  // page not in the lock
	Edited     = "edited"     // page changed since it was stamped
	Removed    = "removed"    // page in the lock no longer exists
	Unanchored = "unanchored" // page has no sources and no citations
)

// Finding is one problem with one page.
type Finding struct {
	Page   string `json:"page"`
	Kind   string `json:"kind"`
	Ref    string `json:"ref,omitempty"` // file or citation concerned
	Detail string `json:"detail"`
}

// Fails reports whether a finding fails `indoc check`.
func (f Finding) Fails(strict bool) bool {
	switch f.Kind {
	case Drift, Unanchored, Unverified:
		return strict
	}
	return true
}

// Check compares pages against the lock.
func Check(r *Repo, ps []pages.Page, lock *Lock) []Finding {
	pageSet := pathSet(ps)
	var out []Finding
	for _, p := range ps {
		out = append(out, checkCitations(r, p)...)
		covered := r.covered(p, pageSet)
		if len(p.Sources) > 0 && len(covered) == 0 {
			out = append(out, Finding{Page: p.Path, Kind: Broken, Ref: strings.Join(p.Sources, ", "), Detail: "sources match no files"})
		}
		if !p.Anchored() {
			out = append(out, Finding{Page: p.Path, Kind: Unanchored, Detail: "no sources and no citations; changes to code cannot be detected"})
		}

		entry, stamped := lock.Pages[p.Path]
		switch {
		case !stamped:
			out = append(out, Finding{Page: p.Path, Kind: Unstamped, Detail: "not in the lock; run indoc ack"})
			continue
		case entry.Page != hashBytes(p.Raw):
			out = append(out, Finding{Page: p.Path, Kind: Edited, Detail: "changed since last stamped; run indoc ack"})
			continue
		}
		out = append(out, compareCitations(r, p, entry)...)
		out = append(out, compareSources(r, p, entry, covered)...)
	}
	for path := range lock.Pages {
		if !pageSet[path] {
			out = append(out, Finding{Page: path, Kind: Removed, Detail: "no longer a page; run indoc ack"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

func checkCitations(r *Repo, p pages.Page) []Finding {
	var out []Finding
	for _, c := range p.Citations {
		f, ok := r.Analyze(c.Path)
		switch {
		case !ok:
			out = append(out, Finding{Page: p.Path, Kind: Broken, Ref: c.Ref(), Detail: fmt.Sprintf("line %d: file not found", c.Line)})
		case !f.Parsed:
			out = append(out, Finding{Page: p.Path, Kind: Unverified, Ref: c.Ref(), Detail: fmt.Sprintf("line %d: no parser for %s files; symbol not verified", c.Line, filepath.Ext(c.Path))})
		default:
			if _, found := f.Lookup(c.Symbol); !found {
				out = append(out, Finding{Page: p.Path, Kind: Broken, Ref: c.Ref(), Detail: fmt.Sprintf("line %d: symbol not found", c.Line)})
			}
		}
	}
	return out
}

func compareCitations(r *Repo, p pages.Page, entry PageLock) []Finding {
	var out []Finding
	for _, c := range p.Citations {
		was, recorded := entry.Citations[c.Ref()]
		if !recorded {
			continue
		}
		f, ok := r.Analyze(c.Path)
		if !ok || !f.Parsed {
			continue
		}
		sym, found := f.Lookup(c.Symbol)
		if !found {
			continue // reported as broken
		}
		if was.Hasher != f.Hasher {
			if was.Raw != f.Raw {
				out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: c.Ref(), Detail: "file " + olderLock})
			}
			continue
		}
		switch {
		case sym.Surface != was.Surface:
			out = append(out, Finding{Page: p.Path, Kind: Stale, Ref: c.Ref(), Detail: "cited symbol's signature changed"})
		case sym.Body != was.Body && p.Strict:
			out = append(out, Finding{Page: p.Path, Kind: Stale, Ref: c.Ref(), Detail: "cited symbol's implementation changed (strict page)"})
		case sym.Body != was.Body:
			out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: c.Ref(), Detail: "cited symbol's implementation changed; check the page still describes it"})
		}
	}
	return out
}

func compareSources(r *Repo, p pages.Page, entry PageLock, covered []string) []Finding {
	var out []Finding
	now := map[string]bool{}
	for _, path := range covered {
		now[path] = true
		was, recorded := entry.Sources[path]
		if !recorded {
			kind := Stale
			if code.IsTest(path) {
				kind = Drift // tests are not the documented interface
			}
			out = append(out, Finding{Page: p.Path, Kind: kind, Ref: path, Detail: "new file under sources"})
			continue
		}
		f, ok := r.Analyze(path)
		if !ok {
			continue
		}
		if was.Hasher != f.Hasher {
			if was.Raw != f.Raw {
				out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: path, Detail: "file " + olderLock})
			}
			continue
		}
		if f.Body == was.Body {
			continue
		}
		if !f.Parsed || was.Symbols == nil {
			out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: path, Detail: "file changed"})
			continue
		}
		if code.IsTest(path) {
			out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: path, Detail: "tests changed"})
			continue
		}
		public, private, skipped := surfaceChanges(was, f, cited(p, path))
		switch {
		case public == "" && private == "" && skipped:
			// Only cited symbols changed; their citations report it.
		case public != "":
			out = append(out, Finding{Page: p.Path, Kind: Stale, Ref: path, Detail: public})
		case private != "":
			out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: path, Detail: "private helpers changed: " + private})
		default:
			out = append(out, Finding{Page: p.Path, Kind: Drift, Ref: path, Detail: "implementation changed; signatures unchanged"})
		}
	}
	for path := range entry.Sources {
		if !now[path] {
			kind := Stale
			if code.IsTest(path) {
				kind = Drift
			}
			out = append(out, Finding{Page: p.Path, Kind: kind, Ref: path, Detail: "file removed from sources"})
		}
	}
	return out
}

// cited returns the symbols the page cites in path.
func cited(p pages.Page, path string) map[string]bool {
	out := map[string]bool{}
	for _, c := range p.Citations {
		if c.Path == path {
			out[c.Symbol] = true
		}
	}
	return out
}

// surfaceChanges describes added, removed, and re-signed symbols, split into
// public ones (stale) and private ones (drift). Either may be "". Symbols in
// skip are left out, because their citations already report them; skipped
// says whether any were.
func surfaceChanges(was FileLock, f code.File, skip map[string]bool) (public, private string, skipped bool) {
	now := f.Surfaces()
	privateNow := f.Private()
	privateWas := map[string]bool{}
	for _, q := range was.Private {
		privateWas[q] = true
	}
	type group struct{ changed, added, removed []string }
	var pub, priv group
	pick := func(q string, isPrivate bool) *group {
		if isPrivate {
			return &priv
		}
		return &pub
	}
	for q, s := range now {
		old, ok := was.Symbols[q]
		switch {
		case !ok:
			g := pick(q, privateNow[q])
			g.added = append(g.added, q)
		case old != s && skip[q]:
			skipped = true
		case old != s:
			g := pick(q, privateNow[q] && privateWas[q])
			g.changed = append(g.changed, q)
		}
	}
	for q := range was.Symbols {
		if _, ok := now[q]; !ok {
			g := pick(q, privateWas[q])
			g.removed = append(g.removed, q)
		}
	}
	return describe(pub.changed, pub.added, pub.removed), describe(priv.changed, priv.added, priv.removed), skipped
}

func describe(changed, added, removed []string) string {
	var parts []string
	for _, group := range []struct {
		label string
		names []string
	}{{"signature changed", changed}, {"added", added}, {"removed", removed}} {
		if len(group.names) == 0 {
			continue
		}
		sort.Strings(group.names)
		names := group.names
		suffix := ""
		if len(names) > 5 {
			suffix = fmt.Sprintf(" and %d more", len(names)-5)
			names = names[:5]
		}
		parts = append(parts, group.label+": "+strings.Join(names, ", ")+suffix)
	}
	return strings.Join(parts, "; ")
}

// Stamp records a page's current state. It fails if any citation does not
// resolve, so a broken page can never be marked verified.
func Stamp(r *Repo, p pages.Page, pageSet map[string]bool) (PageLock, error) {
	entry := PageLock{Page: hashBytes(p.Raw)}
	var broken []string
	for _, c := range p.Citations {
		f, ok := r.Analyze(c.Path)
		if !ok {
			broken = append(broken, c.Ref()+" (file not found)")
			continue
		}
		if !f.Parsed {
			continue // unverifiable; nothing to record
		}
		sym, found := f.Lookup(c.Symbol)
		if !found {
			broken = append(broken, c.Ref()+" (symbol not found)")
			continue
		}
		if entry.Citations == nil {
			entry.Citations = map[string]SymbolLock{}
		}
		entry.Citations[c.Ref()] = SymbolLock{Surface: sym.Surface, Body: sym.Body, Raw: f.Raw, Hasher: f.Hasher}
	}
	if len(broken) > 0 {
		return PageLock{}, fmt.Errorf("%s has broken citations: %s", p.Path, strings.Join(broken, ", "))
	}
	for _, path := range r.covered(p, pageSet) {
		f, ok := r.Analyze(path)
		if !ok {
			continue
		}
		fl := FileLock{Body: f.Body, Raw: f.Raw, Hasher: f.Hasher}
		if f.Parsed {
			fl.Symbols = f.Surfaces()
			for q := range f.Private() {
				fl.Private = append(fl.Private, q)
			}
			sort.Strings(fl.Private)
		}
		if entry.Sources == nil {
			entry.Sources = map[string]FileLock{}
		}
		entry.Sources[path] = fl
	}
	return entry, nil
}

// AckResult lists what Ack changed.
type AckResult struct {
	Stamped []string
	Dropped []string
}

// Ack updates the lock. With no targets it stamps pages that are new or whose
// content changed, and drops pages that no longer exist. Targets are stamped
// even when unchanged: "reviewed, still correct".
func Ack(r *Repo, ps []pages.Page, lock *Lock, targets []string) (AckResult, error) {
	pageSet := pathSet(ps)
	byPath := map[string]pages.Page{}
	for _, p := range ps {
		byPath[p.Path] = p
	}
	var res AckResult
	var errs []string
	stamp := func(p pages.Page) {
		entry, err := Stamp(r, p, pageSet)
		if err != nil {
			errs = append(errs, err.Error())
			return
		}
		lock.Pages[p.Path] = entry
		res.Stamped = append(res.Stamped, p.Path)
	}

	if len(targets) > 0 {
		for _, t := range targets {
			t = filepath.ToSlash(filepath.Clean(t))
			p, ok := byPath[t]
			if !ok {
				errs = append(errs, t+" is not a page (check pages: in .indoc/config.yaml)")
				continue
			}
			stamp(p)
		}
	} else {
		for _, p := range ps {
			entry, ok := lock.Pages[p.Path]
			switch {
			case !ok || entry.Page != hashBytes(p.Raw):
				stamp(p)
			case upgradable(r, entry):
				// Written by another indoc version and nothing changed since:
				// re-recording loses no information.
				stamp(p)
			}
		}
		for path := range lock.Pages {
			if !pageSet[path] {
				delete(lock.Pages, path)
				res.Dropped = append(res.Dropped, path)
			}
		}
		sort.Strings(res.Dropped)
	}
	if len(errs) > 0 {
		return res, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return res, nil
}

// upgradable reports a lock entry with hashes from a different hasher whose
// files are byte-for-byte unchanged.
func upgradable(r *Repo, entry PageLock) bool {
	outdated := false
	check := func(path, raw, hasher string) bool {
		f, ok := r.Analyze(path)
		if !ok || f.Raw != raw {
			return false
		}
		if f.Hasher != hasher {
			outdated = true
		}
		return true
	}
	for ref, c := range entry.Citations {
		path, _, _ := strings.Cut(ref, "::")
		if !check(path, c.Raw, c.Hasher) {
			return false
		}
	}
	for path, fl := range entry.Sources {
		if !check(path, fl.Raw, fl.Hasher) {
			return false
		}
	}
	return outdated
}

// PageSet returns the set of page paths.
func PageSet(ps []pages.Page) map[string]bool { return pathSet(ps) }

func pathSet(ps []pages.Page) map[string]bool {
	set := make(map[string]bool, len(ps))
	for _, p := range ps {
		set[p.Path] = true
	}
	return set
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}
