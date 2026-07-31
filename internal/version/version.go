// Package version provides shared build-time version information for
// the CLI and the desktop application. The variables are set via -ldflags
// at build time; defaults are used for development builds.
package version

// Build-time variables. Override with:
//
//	go build -ldflags "-X github.com/FNB2026/nas-data-governance/internal/version.Version=0.5.0-beta.1 \
//	  -X github.com/FNB2026/nas-data-governance/internal/version.Commit=abc123 \
//	  -X github.com/FNB2026/nas-data-governance/internal/version.BuildTime=2026-07-28T00:00:00Z \
//	  -X github.com/FNB2026/nas-data-governance/internal/version.Channel=beta"
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
	Channel   = "dev"
)

// Info is the structured version information returned by the desktop
// API and printed by the CLI version command.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Channel   string `json:"channel"`
}

// Get returns the current version info.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		Channel:   Channel,
	}
}
