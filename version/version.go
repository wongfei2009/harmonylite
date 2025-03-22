// Package version provides version information about HarmonyLite.
package version

import (
	"fmt"
	"runtime"
)

// Info contains version information
type Info struct {
	Version   string `json:"version"`     // Semantic version (e.g., v1.2.3)
	GitCommit string `json:"git_commit"`  // Git commit hash
	GitTag    string `json:"git_tag"`     // Git tag if available
	BuildDate string `json:"build_date"`  // Build timestamp
	GoVersion string `json:"go_version"`  // Go version used for building
	Platform  string `json:"platform"`    // OS/Arch combination
}

// Variables to be populated by ldflags during build
var (
	version   = "dev"
	gitCommit = "unknown"
	gitTag    = "none"
	buildDate = "unknown"
	goVersion = runtime.Version()
	platform  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

// Get returns the version information
func Get() Info {
	return Info{
		Version:   version,
		GitCommit: gitCommit,
		GitTag:    gitTag,
		BuildDate: buildDate,
		GoVersion: goVersion,
		Platform:  platform,
	}
}

// String returns a string representation of version info
func (i Info) String() string {
	return fmt.Sprintf("HarmonyLite %s (git: %s, tag: %s, built: %s, %s, %s)",
		i.Version, i.GitCommit, i.GitTag, i.BuildDate, i.GoVersion, i.Platform)
}

// ShortString returns a short string representation of version info
func (i Info) ShortString() string {
	return fmt.Sprintf("%s (%s)", i.Version, i.GitCommit[:7])
}