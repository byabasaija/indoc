package cli

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var previewPort int

var previewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Serve the docs on localhost and rebuild when pages change",
	Long: `preview builds the site into a temporary directory, serves it on 127.0.0.1,
and rebuilds whenever a page or the config changes. Refresh the browser to see
the result.

It never writes to output_dir or the lock, so it is safe to leave running.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPreview(cfgFile, previewPort, cmd.OutOrStdout())
	},
}

func init() {
	previewCmd.Flags().IntVar(&previewPort, "port", 4173, "localhost port")
	rootCmd.AddCommand(previewCmd)
}

func runPreview(configPath string, port int, out io.Writer) error {
	dir, err := os.MkdirTemp("", "indoc-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	build := func() (string, error) {
		p, err := openProject(configPath)
		if err != nil {
			return "", err
		}
		if err := buildSite(p, dir); err != nil {
			return "", err
		}
		return fingerprint(p), nil
	}
	last, err := build()
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Previewing at http://%s (Ctrl+C to stop)\n", ln.Addr())
	go func() {
		for range time.Tick(700 * time.Millisecond) {
			p, err := openProject(configPath)
			if err != nil {
				continue
			}
			if fp := fingerprint(p); fp != last {
				if _, err := build(); err != nil {
					fmt.Fprintf(out, "rebuild failed: %v\n", err)
				} else {
					fmt.Fprintf(out, "rebuilt %s\n", time.Now().Format("15:04:05"))
				}
				last = fp
			}
		}
	}()
	return http.Serve(ln, http.FileServer(http.Dir(dir)))
}

// fingerprint changes whenever a page or the config file changes.
func fingerprint(p *project) string {
	var b strings.Builder
	stat := func(path string) {
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", path, info.Size(), info.ModTime().UnixNano())
		}
	}
	stat(p.paths.Config)
	stat(filepath.Join(filepath.Dir(p.paths.Config), "custom.css"))
	stat(filepath.Join(filepath.Dir(p.paths.Config), "head.html"))
	if entries, err := os.ReadDir(themeDir(p.paths)); err == nil {
		for _, e := range entries {
			stat(filepath.Join(themeDir(p.paths), e.Name()))
		}
	}
	for _, pg := range p.pages {
		stat(filepath.Join(p.paths.Root, filepath.FromSlash(pg.Path)))
	}
	if p.cfg.Appearance != nil && p.cfg.Appearance.Logo != "" {
		stat(filepath.Join(p.paths.Root, p.cfg.Appearance.Logo))
	}
	return b.String()
}
