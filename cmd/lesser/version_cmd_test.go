package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatVersion(t *testing.T) {
	t.Run("no build info", func(t *testing.T) {
		require.Equal(t, "lesser (unknown)\n", formatVersion(nil, false))
		require.Equal(t, "lesser (unknown)\n", formatVersion(nil, true))
	})

	t.Run("unknown version without revision", func(t *testing.T) {
		buildInfo := &debug.BuildInfo{
			Main: debug.Module{Version: ""},
		}
		require.Equal(t, "lesser (unknown)\n", formatVersion(buildInfo, true))
	})

	t.Run("version without revision", func(t *testing.T) {
		buildInfo := &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
		}
		require.Equal(t, "lesser v1.2.3\n", formatVersion(buildInfo, true))
	})

	t.Run("version with short revision", func(t *testing.T) {
		buildInfo := &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
			},
		}
		require.Equal(t, "lesser v1.2.3 (abc123)\n", formatVersion(buildInfo, true))
	})

	t.Run("version with long revision trims", func(t *testing.T) {
		buildInfo := &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			},
		}
		require.Equal(t, "lesser v1.2.3 (0123456789ab)\n", formatVersion(buildInfo, true))
	})

	t.Run("modified repo prints dirty", func(t *testing.T) {
		buildInfo := &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.modified", Value: "true"},
			},
		}
		require.Equal(t, "lesser v1.2.3 (0123456789ab, dirty)\n", formatVersion(buildInfo, true))
	})
}
