// Package version exposes build metadata for the simulator binary.
//
// The values default to development placeholders and can be overridden at build
// time with -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/Zulut30/local-telegram-client/internal/version.Version=v0.1.0 \
//	  -X github.com/Zulut30/local-telegram-client/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/Zulut30/local-telegram-client/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
//
// When ldflags are not set, Info() falls back to the embedded module build info
// recorded by the Go toolchain (VCS revision and modified flag).
package version

import "runtime/debug"

var (
	// Version is the released semantic version (or "dev").
	Version = "dev"
	// Commit is the short VCS revision the binary was built from.
	Commit = ""
	// Date is the RFC3339 build timestamp.
	Date = ""
)

// Build describes the running binary.
type Build struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	Date      string `json:"date,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
}

// Info returns the build metadata, enriching unset fields from the Go build info
// embedded by the toolchain.
func Info() Build {
	b := Build{Version: Version, Commit: Commit, Date: Date}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	b.GoVersion = info.GoVersion
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if b.Commit == "" {
				b.Commit = shortCommit(setting.Value)
			}
		case "vcs.time":
			if b.Date == "" {
				b.Date = setting.Value
			}
		case "vcs.modified":
			if setting.Value == "true" && b.Commit != "" {
				b.Commit += "-dirty"
			}
		}
	}
	return b
}

func shortCommit(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
