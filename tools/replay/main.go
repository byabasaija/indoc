// Command replay measures indoc check against real git history.
//
// It stamps a fixed set of pages at a base commit, then checks out each later
// first-parent commit and runs drift.Check. When a commit fails the check it
// simulates a developer responding (re-recording the flagged pages) so one
// change is not counted again on every later commit.
//
//	go run ./tools/replay run -repo ~/projects/shop -pages ./pages -base <sha> -n 100
//	go run ./tools/replay symbols -repo ~/projects/shop -rev <sha> path/to/file.py
//
// Development tool only; not part of the indoc binary.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/drift"
	"github.com/byabasaija/indoc/internal/pages"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: replay run|symbols ...")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = run(os.Args[2:])
	case "symbols":
		err = symbols(os.Args[2:])
	case "diffsym":
		err = diffsym(os.Args[2:])
	default:
		err = fmt.Errorf("unknown mode %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, ee.Stderr)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func symbols(args []string) error {
	fs := flag.NewFlagSet("symbols", flag.ExitOnError)
	repo := fs.String("repo", ".", "git repository")
	rev := fs.String("rev", "HEAD", "revision")
	fs.Parse(args)
	for _, path := range fs.Args() {
		src, err := git(*repo, "show", *rev+":"+path)
		if err != nil {
			return err
		}
		f := code.Analyze(path, []byte(src))
		fmt.Printf("%s (%d symbols)\n", path, len(f.Symbols))
		for _, s := range f.Symbols {
			fmt.Printf("  %-6d %s\n", s.Line, s.Qual)
		}
	}
	return nil
}

// diffsym prints a cited symbol's source before and after a commit (first
// parent), as a unified diff.
func diffsym(args []string) error {
	fs := flag.NewFlagSet("diffsym", flag.ExitOnError)
	repo := fs.String("repo", ".", "git repository")
	rev := fs.String("rev", "HEAD", "commit")
	fs.Parse(args)
	ref := fs.Arg(0)
	file, sym, ok := strings.Cut(ref, "::")
	if !ok {
		return fmt.Errorf("want path::Symbol, got %q", ref)
	}
	extract := func(r string) string {
		src, err := git(*repo, "show", r+":"+file)
		if err != nil {
			return "(file absent)\n"
		}
		f := code.Analyze(file, []byte(src))
		s, found := f.Lookup(sym)
		if !found {
			return "(symbol absent)\n"
		}
		lines := strings.Split(src, "\n")
		return strings.Join(lines[s.DecoratorLine-1:s.EndLine], "\n") + "\n"
	}
	dir, err := os.MkdirTemp("", "diffsym-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	a, b := filepath.Join(dir, "before"), filepath.Join(dir, "after")
	os.WriteFile(a, []byte(extract(*rev+"^1")), 0o644)
	os.WriteFile(b, []byte(extract(*rev)), 0o644)
	out, _ := exec.Command("git", "diff", "--no-index", "-w", "-U1", a, b).Output()
	fmt.Print(string(out))
	return nil
}

type event struct {
	Commit   string          `json:"commit"`
	Subject  string          `json:"subject"`
	Failing  []drift.Finding `json:"failing,omitempty"`
	Drift    []drift.Finding `json:"drift,omitempty"`
	Retired  []string        `json:"retired,omitempty"` // citations removed after their symbol disappeared
	Restamps []string        `json:"restamped,omitempty"`
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repo := fs.String("repo", ".", "git repository to replay")
	pagesDir := fs.String("pages", "", "directory of Markdown pages (outside the repository)")
	base := fs.String("base", "", "commit to stamp pages at")
	n := fs.Int("n", 100, "number of first-parent commits after base to replay")
	out := fs.String("out", "", "write per-commit events as JSON lines here")
	fs.Parse(args)
	if *pagesDir == "" || *base == "" {
		return fmt.Errorf("-pages and -base are required")
	}

	wt, err := os.MkdirTemp("", "indoc-replay-")
	if err != nil {
		return err
	}
	defer func() {
		git(*repo, "worktree", "remove", "--force", wt)
		os.RemoveAll(wt)
	}()
	if _, err := git(*repo, "worktree", "add", "--detach", wt, *base); err != nil {
		return err
	}

	ps, err := loadPages(*pagesDir)
	if err != nil {
		return err
	}
	r, err := drift.NewRepo(wt, nil)
	if err != nil {
		return err
	}
	lock := drift.NewLock()
	if _, err := drift.Ack(r, ps, lock, nil); err != nil {
		return fmt.Errorf("pages do not stamp cleanly at base: %w", err)
	}
	if fs := drift.Check(r, ps, lock); len(fs) > 0 {
		return fmt.Errorf("pages have findings at base: %+v", fs)
	}

	list, err := git(*repo, "rev-list", "--first-parent", "--reverse", *base+"..HEAD")
	if err != nil {
		return err
	}
	commits := strings.Fields(list)
	if len(commits) > *n {
		commits = commits[:*n]
	}

	var sink *json.Encoder
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		sink = json.NewEncoder(f)
	}

	failingCommits, driftOnly := 0, 0
	kinds := map[string]int{}
	perPage := map[string]int{}
	for _, c := range commits {
		if _, err := git(wt, "checkout", "-q", "--detach", c); err != nil {
			return err
		}
		subject, _ := git(wt, "log", "-1", "--format=%s", c)
		r, err := drift.NewRepo(wt, nil)
		if err != nil {
			return err
		}
		ev := event{Commit: c[:8], Subject: subject}
		flagged := map[string]bool{}
		for _, f := range drift.Check(r, ps, lock) {
			if f.Fails(false) {
				ev.Failing = append(ev.Failing, f)
				flagged[f.Page] = true
				kinds[f.Kind]++
			} else if f.Kind == drift.Drift {
				ev.Drift = append(ev.Drift, f)
			}
		}
		switch {
		case len(ev.Failing) > 0:
			failingCommits++
			for p := range flagged {
				perPage[p]++
			}
		case len(ev.Drift) > 0:
			driftOnly++
		}

		// Simulate the developer's response: fix citations whose symbol is
		// gone, then re-record every page that changed state.
		for i := range ps {
			kept := ps[i].Citations[:0]
			for _, cit := range ps[i].Citations {
				f, ok := r.Analyze(cit.Path)
				if ok && f.Parsed {
					if _, found := f.Lookup(cit.Symbol); !found {
						ev.Retired = append(ev.Retired, ps[i].Path+": "+cit.Ref())
						continue
					}
				} else if !ok {
					ev.Retired = append(ev.Retired, ps[i].Path+": "+cit.Ref())
					continue
				}
				kept = append(kept, cit)
			}
			ps[i].Citations = kept
		}
		var targets []string
		for _, p := range ps {
			if flagged[p.Path] || hasDrift(ev.Drift, p.Path) {
				targets = append(targets, p.Path)
			}
		}
		if len(targets) > 0 {
			if _, err := drift.Ack(r, ps, lock, targets); err != nil {
				return fmt.Errorf("restamp at %s: %w", c[:8], err)
			}
			ev.Restamps = targets
		}
		if sink != nil {
			sink.Encode(ev)
		}
	}

	total := len(commits)
	fmt.Printf("commits replayed:      %d\n", total)
	fmt.Printf("check failed:          %d (%.1f%%)\n", failingCommits, pct(failingCommits, total))
	fmt.Printf("drift only (passes):   %d (%.1f%%)\n", driftOnly, pct(driftOnly, total))
	fmt.Printf("failing findings:      %s\n", fmtCounts(kinds))
	fmt.Printf("failures per page:     %s\n", fmtCounts(perPage))
	return nil
}

func hasDrift(fs []drift.Finding, page string) bool {
	for _, f := range fs {
		if f.Page == page {
			return true
		}
	}
	return false
}

func loadPages(dir string) ([]pages.Page, error) {
	var ps []pages.Page
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		page, err := pages.Parse("pages/"+filepath.ToSlash(rel), raw)
		if err != nil {
			return err
		}
		ps = append(ps, page)
		return nil
	})
	sort.Slice(ps, func(i, j int) bool { return ps[i].Path < ps[j].Path })
	return ps, err
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func fmtCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}
