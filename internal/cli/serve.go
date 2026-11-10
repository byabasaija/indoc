package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/spf13/cobra"
)

var (
	serveDir  string
	servePort int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Preview a compiled docs directory on localhost",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := serveDir
		if dir == "" {
			cfg, _, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			dir = cfg.OutputDir
		}
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
			return fmt.Errorf("no index.html in %s; run indoc bundle first", dir)
		}
		addr := fmt.Sprintf("127.0.0.1:%d", servePort)
		fmt.Fprintf(cmd.OutOrStdout(), "Docs at http://%s\n", addr)
		return http.ListenAndServe(addr, http.FileServer(http.Dir(dir)))
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveDir, "dir", "", "compiled docs directory (default from .indoc.yaml or ./static/docs)")
	serveCmd.Flags().IntVar(&servePort, "port", 4173, "localhost port")
	rootCmd.AddCommand(serveCmd)
}
