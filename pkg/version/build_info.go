// Package version provides build information and version details for Lesser.
package version

import (
	"runtime/debug"
	"time"
)

var (
	// Version is the application version, set by build flags or defaults to "dev"
	Version = "dev"
	// GitCommit is the Git commit hash, extracted from build info or "unknown"
	GitCommit = "unknown"
	// BuildTime is the build timestamp, extracted from build info or "unknown"
	BuildTime = "unknown"
	// GoVersion is the Go version used to build the application
	GoVersion = "unknown"
)

func init() {
	// Try to extract build information from debug info
	if info, ok := debug.ReadBuildInfo(); ok {
		GoVersion = info.GoVersion

		// Parse VCS information from build settings
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				// Use short commit hash (first 7 characters)
				if len(setting.Value) > 7 {
					GitCommit = setting.Value[:7]
				} else {
					GitCommit = setting.Value
				}
			case "vcs.time":
				// Parse VCS timestamp
				if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					BuildTime = t.Format("2006-01-02T15:04:05Z")
				}
			}
		}
	}
}

// GetBuildInfo returns comprehensive build information as a map
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"git_commit": GitCommit,
		"build_time": BuildTime,
		"go_version": GoVersion,
	}
}

// GetVersion returns the current application version
func GetVersion() string {
	return Version
}

// GetGitCommit returns the Git commit hash
func GetGitCommit() string {
	return GitCommit
}

// GetBuildTime returns the build timestamp
func GetBuildTime() string {
	return BuildTime
}

// GetGoVersion returns the Go version used to build
func GetGoVersion() string {
	return GoVersion
}

// GetFullVersionString returns a formatted version string with all details
func GetFullVersionString() string {
	return Version + " (" + GitCommit + ", built " + BuildTime + " with " + GoVersion + ")"
}