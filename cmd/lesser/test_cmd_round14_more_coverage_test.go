package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterGeneratedFilesFromCoverProfile_Round14_ErrorBranches(t *testing.T) {
	repoRoot := t.TempDir()
	coverProfilePath := filepath.Join(repoRoot, "coverage.out")

	require.Error(t, filterGeneratedFilesFromCoverProfile(repoRoot, coverProfilePath))

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))
	require.Error(t, filterGeneratedFilesFromCoverProfile(repoRoot, coverProfilePath))
}

func TestListPackagesForAllCoverage_Round14_IncludeToolsFlag(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })

	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return strings.Join([]string{
			"github.com/equaltoai/lesser/cmd/api",
			"github.com/equaltoai/lesser/pkg/common",
			"github.com/equaltoai/lesser/tools/coverage_scoreboard",
		}, "\n"), nil
	}

	withoutTools, err := listPackagesForAllCoverage(repoRoot, t.TempDir(), false)
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/cmd/api",
		"github.com/equaltoai/lesser/pkg/common",
	}, withoutTools)

	withTools, err := listPackagesForAllCoverage(repoRoot, t.TempDir(), true)
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/cmd/api",
		"github.com/equaltoai/lesser/pkg/common",
		"github.com/equaltoai/lesser/tools/coverage_scoreboard",
	}, withTools)
}

func TestListPackagesForOverallCoverage_Round14_IncludeFlags(t *testing.T) {
	previousCapture := captureCommandOutputFn
	t.Cleanup(func() { captureCommandOutputFn = previousCapture })

	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module github.com/equaltoai/lesser\n"), 0o644))

	captureCommandOutputFn = func(context.Context, string, map[string]string, string, ...string) (string, error) {
		return strings.Join([]string{
			"github.com/equaltoai/lesser/cmd/api",
			"github.com/equaltoai/lesser/pkg/common",
			"github.com/equaltoai/lesser/pkg/testing/harness",
			"github.com/equaltoai/lesser/tools/coverage_scoreboard",
		}, "\n"), nil
	}

	defaults, err := listPackagesForOverallCoverage(repoRoot, t.TempDir(), false, false)
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/cmd/api",
		"github.com/equaltoai/lesser/pkg/common",
	}, defaults)

	withTesting, err := listPackagesForOverallCoverage(repoRoot, t.TempDir(), false, true)
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/cmd/api",
		"github.com/equaltoai/lesser/pkg/common",
		"github.com/equaltoai/lesser/pkg/testing/harness",
	}, withTesting)

	withTools, err := listPackagesForOverallCoverage(repoRoot, t.TempDir(), true, false)
	require.NoError(t, err)
	require.Equal(t, []string{
		"github.com/equaltoai/lesser/cmd/api",
		"github.com/equaltoai/lesser/pkg/common",
		"github.com/equaltoai/lesser/tools/coverage_scoreboard",
	}, withTools)
}

