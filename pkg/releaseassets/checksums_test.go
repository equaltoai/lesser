package releaseassets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteChecksums(t *testing.T) {
	outDir := t.TempDir()
	for _, assetName := range publishedReleaseAssets {
		require.NoError(t, os.WriteFile(filepath.Join(outDir, assetName), []byte(assetName), 0o644))
	}

	require.NoError(t, WriteChecksums(outDir))

	data, err := os.ReadFile(filepath.Join(outDir, ChecksumsFileName))
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, len(publishedReleaseAssets))
	for i, assetName := range publishedReleaseAssets {
		require.True(t, strings.HasSuffix(lines[i], "  "+assetName))
	}
}
