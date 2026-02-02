package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadModulePath_Round15_RejectsMissingModuleLine(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("go 1.25\n"), 0o644))
	_, err := readModulePath(repoRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "module path")
}

func TestFilterCoverageData_Round15_HandlesColonlessAndScannerError(t *testing.T) {
	t.Run("passes through colonless lines", func(t *testing.T) {
		in := strings.NewReader("mode: set\nthis line has no colon\npkg/file.go:1.1,1.2 1 1\n")
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		require.NoError(t, filterCoverageData(in, w, t.TempDir(), ""))
		require.NoError(t, w.Flush())
		require.Contains(t, buf.String(), "this line has no colon\n")
	})

	t.Run("returns error on oversized token", func(t *testing.T) {
		longLine := strings.Repeat("x", 70000) + "\n"
		in := strings.NewReader(longLine)
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		err := filterCoverageData(in, w, t.TempDir(), "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "read coverprofile")
	})
}
