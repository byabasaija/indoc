package version

import "fmt"

var (
	// Version is the current semantic version of the indoc binary.
	// Can be overridden at compile time via: -ldflags "-X github.com/byabasaija/indoc/internal/version.Version=v0.1.0"
	Version = "0.8.0-dev"

	// GitCommit is the git SHA at build time.
	GitCommit = "none"

	// BuildDate is the date and time of the build.
	BuildDate = "unknown"
)

// Info returns formatted version information.
func Info() string {
	return fmt.Sprintf("indoc %s (commit: %s, built at: %s)", Version, GitCommit, BuildDate)
}
