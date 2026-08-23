package releaseassets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ChecksumsFileName is the published checksum manifest for release assets.
const ChecksumsFileName = "checksums.txt"

var publishedReleaseAssets = []string{
	"lesser-linux-amd64",
	"lesser-linux-arm64",
	"lesser-darwin-amd64",
	"lesser-darwin-arm64",
	LambdaBundleArchiveName,
	LambdaBundleManifestName,
	AuthUIBundleArchiveName,
	DeployAssemblyArchiveName,
	DeployAssemblyManifestName,
	ReleaseManifestName,
}

// PublishedReleaseAssetNames returns the canonical published release asset set.
func PublishedReleaseAssetNames() []string {
	return append([]string(nil), publishedReleaseAssets...)
}

// WriteChecksums writes checksums.txt for the full published release asset set.
func WriteChecksums(outDir string) error {
	lines := make([]string, 0, len(publishedReleaseAssets))
	for _, assetName := range publishedReleaseAssets {
		assetPath := filepath.Join(outDir, assetName)
		sum, err := fileSHA256(assetPath)
		if err != nil {
			return fmt.Errorf("hash release asset %s: %w", assetPath, err)
		}
		lines = append(lines, fmt.Sprintf("%s  %s", sum, assetName))
	}

	content := strings.Join(lines, "\n") + "\n"
	//nolint:gosec // The checksum list is an intentionally public GitHub release artifact with no secret material.
	if err := os.WriteFile(filepath.Join(outDir, ChecksumsFileName), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write checksums: %w", err)
	}
	return nil
}
