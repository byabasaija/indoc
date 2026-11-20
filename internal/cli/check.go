package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/byabasaija/indoc/internal/drift"
	"github.com/spf13/cobra"
)

var (
	checkStrict bool
	checkWarn   bool
	checkJSON   bool
)

// errCheckFailed makes the process exit 1 without repeating the report.
var errCheckFailed = errors.New("docs do not match the code")

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Fail when documentation no longer matches the code (for CI)",
	Long: `check compares every page against .indoc/lock.json and fails when:

  stale       code a page cites or covers changed its signature or was added/removed
  broken      a path::Symbol citation or sources glob does not resolve
  edited      a page changed but was not re-stamped (run indoc ack)
  unstamped   a page is new (run indoc ack)
  removed     a stamped page no longer exists (run indoc ack)

These are reported but fail only with --strict:

  drift       implementation changed under a page's sources; signatures did not
  unanchored  a page has no sources and no citations
  unverified  a citation into a language indoc cannot parse yet

check never writes anything.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheck(cfgFile, cmd.OutOrStdout(), checkStrict, checkWarn, checkJSON)
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkStrict, "strict", false, "also fail on drift, unanchored pages, and unverified citations")
	checkCmd.Flags().BoolVar(&checkWarn, "warn", false, "report problems but always exit 0")
	checkCmd.Flags().BoolVar(&checkJSON, "json", false, "print findings as JSON")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(configPath string, out io.Writer, strict, warn, asJSON bool) error {
	p, err := openProject(configPath)
	if err != nil {
		return err
	}
	findings := drift.Check(p.repo, p.pages, p.lock)
	failing := 0
	for _, f := range findings {
		if f.Fails(strict) {
			failing++
		}
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			Pages    int             `json:"pages"`
			Failing  int             `json:"failing"`
			Findings []drift.Finding `json:"findings"`
		}{len(p.pages), failing, nonNil(findings)}); err != nil {
			return err
		}
	} else {
		printFindings(out, len(p.pages), findings, strict)
	}
	if failing > 0 && !warn {
		return errCheckFailed
	}
	return nil
}

// colors are ANSI styles, empty unless out is a terminal and NO_COLOR is unset.
type colors struct{ bold, red, yellow, green, dim, reset string }

func colorsFor(out io.Writer) colors {
	f, ok := out.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return colors{}
	}
	if info, err := f.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return colors{}
	}
	return colors{bold: "\x1b[1m", red: "\x1b[1;31m", yellow: "\x1b[33m", green: "\x1b[32m", dim: "\x1b[2m", reset: "\x1b[0m"}
}

func printFindings(out io.Writer, pageCount int, findings []drift.Finding, strict bool) {
	c := colorsFor(out)
	if len(findings) == 0 {
		fmt.Fprintf(out, "%s✓%s %d pages match the code.\n", c.green, c.reset, pageCount)
		return
	}
	counts := map[string]int{}
	current := ""
	for _, f := range findings {
		if f.Page != current {
			if current != "" {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, c.bold+f.Page+c.reset)
			current = f.Page
		}
		mark, style := " ", c.yellow
		if f.Fails(strict) {
			mark, style = "✗", c.red
		}
		ref := f.Ref
		if ref != "" {
			ref += "  "
		}
		fmt.Fprintf(out, "  %s%s %-10s%s %s%s%s%s\n", style, mark, strings.ToUpper(f.Kind), c.reset, ref, c.dim, f.Detail, c.reset)
		counts[f.Kind]++
	}

	var parts []string
	for _, k := range []string{drift.Stale, drift.Broken, drift.Edited, drift.Unstamped, drift.Removed, drift.Drift, drift.Unanchored, drift.Unverified} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	fmt.Fprintf(out, "\n%d pages checked: %s\n", pageCount, strings.Join(parts, ", "))
	if counts[drift.Stale] > 0 {
		fmt.Fprintln(out, "Stale: update the page, or run `indoc ack <page>` if it is still correct.")
	}
	if counts[drift.Edited]+counts[drift.Unstamped]+counts[drift.Removed] > 0 {
		fmt.Fprintln(out, "Edited/unstamped/removed: run `indoc ack` to record the current pages.")
	}
}

func nonNil(f []drift.Finding) []drift.Finding {
	if f == nil {
		return []drift.Finding{}
	}
	return f
}
