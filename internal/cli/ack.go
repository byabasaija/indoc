package cli

import (
	"fmt"
	"io"

	"github.com/byabasaija/indoc/internal/drift"
	"github.com/spf13/cobra"
)

var ackCmd = &cobra.Command{
	Use:   "ack [page...]",
	Short: "Record pages as verified against the current code",
	Long: `ack updates .indoc/lock.json.

With no arguments it records pages that are new or were edited, and forgets
pages that were deleted. It never re-records an unchanged page, so it cannot
silence a stale warning by accident.

With page paths it records those pages even if unchanged. Use this after
reviewing a stale page and confirming it is still correct.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAck(cfgFile, cmd.OutOrStdout(), args)
	},
}

func init() {
	rootCmd.AddCommand(ackCmd)
}

func runAck(configPath string, out io.Writer, targets []string) error {
	p, err := openProject(configPath)
	if err != nil {
		return err
	}
	res, ackErr := drift.Ack(p.repo, p.pages, p.lock, targets)
	if len(res.Stamped)+len(res.Dropped) > 0 {
		if err := p.lock.Save(p.paths.Lock); err != nil {
			return err
		}
	}
	for _, s := range res.Stamped {
		fmt.Fprintf(out, "recorded  %s\n", s)
	}
	for _, d := range res.Dropped {
		fmt.Fprintf(out, "forgot    %s\n", d)
	}
	if ackErr != nil {
		return ackErr
	}
	if len(res.Stamped)+len(res.Dropped) == 0 {
		fmt.Fprintln(out, "Lock is up to date. To confirm an unchanged stale page, name it: indoc ack <page>")
	}
	return nil
}
