package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoListLines_IgnoresGoDownloadNoise(t *testing.T) {
	lines := goListLines("go: downloading github.com/gorilla/mux v1.8.1\npkg/a\n\ngo: downloading golang.org/x/text v0.35.0\npkg/b\n")
	require.Equal(t, []string{"pkg/a", "pkg/b"}, lines)
}

func TestListGoPackagesForSecurityTool_IgnoresGoDownloadNoise(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })

	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "go: downloading github.com/gorilla/mux v1.8.1\n" +
			"github.com/equaltoai/lesser/pkg/a\n" +
			"github.com/equaltoai/lesser/pkg/b\n", nil
	}

	pkgs, err := listGoPackagesForSecurityTool(t.TempDir(), map[string]string{"GOCACHE": t.TempDir()})
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/pkg/a",
		"github.com/equaltoai/lesser/pkg/b",
	}, pkgs)
}

func TestListPackagesForOverallCoverage_IgnoresGoDownloadNoise(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })

	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return "go: downloading github.com/gorilla/mux v1.8.1\n" +
			"github.com/equaltoai/lesser/cmd/api\n" +
			"github.com/equaltoai/lesser/pkg/common\n", nil
	}

	pkgs, err := listPackagesForOverallCoverage(repoRoot, t.TempDir(), false, false)
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/cmd/api",
		"github.com/equaltoai/lesser/pkg/common",
	}, pkgs)
}
