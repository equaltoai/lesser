package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildInfo_GettersAndFormatting(t *testing.T) {
	oldVersion := Version
	oldGitCommit := GitCommit
	oldBuildTime := BuildTime
	oldGoVersion := GoVersion
	t.Cleanup(func() {
		Version = oldVersion
		GitCommit = oldGitCommit
		BuildTime = oldBuildTime
		GoVersion = oldGoVersion
	})

	Version = "1.2.3"
	GitCommit = "abc1234"
	BuildTime = "2025-01-01T00:00:00Z"
	GoVersion = "go1.23.0"

	require.Equal(t, Version, GetVersion())
	require.Equal(t, GitCommit, GetGitCommit())
	require.Equal(t, BuildTime, GetBuildTime())
	require.Equal(t, GoVersion, GetGoVersion())

	info := GetBuildInfo()
	require.Equal(t, map[string]string{
		"version":    "1.2.3",
		"git_commit": "abc1234",
		"build_time": "2025-01-01T00:00:00Z",
		"go_version": "go1.23.0",
	}, info)

	require.Equal(t, "1.2.3 (abc1234, built 2025-01-01T00:00:00Z with go1.23.0)", GetFullVersionString())
}
