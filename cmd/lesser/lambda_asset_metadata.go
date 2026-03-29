package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const lambdaAssetMetadataFileName = "metadata.json"

type lambdaAssetMetadata struct {
	Schema         int       `json:"schema"`
	Mode           string    `json:"mode"`
	Files          []string  `json:"files"`
	ReleaseVersion string    `json:"release_version,omitempty"`
	ReleaseGitSHA  string    `json:"release_git_sha,omitempty"`
	PreparedAt     time.Time `json:"prepared_at"`
}

func writeLambdaAssetMetadata(assetRoot string, metadata lambdaAssetMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lambda asset metadata: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(assetRoot, lambdaAssetMetadataFileName)
	if err := os.MkdirAll(assetRoot, 0o750); err != nil {
		return fmt.Errorf("create lambda asset metadata dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write lambda asset metadata: %w", err)
	}
	return nil
}

func relativeLambdaAssetFiles(assetRoot string, files []string) ([]string, error) {
	relFiles := make([]string, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(assetRoot, file)
		if err != nil {
			return nil, fmt.Errorf("relative lambda asset path %s: %w", file, err)
		}
		if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return nil, fmt.Errorf("lambda asset path %s escapes asset root %s", file, assetRoot)
		}
		relFiles = append(relFiles, filepath.ToSlash(rel))
	}
	slices.Sort(relFiles)
	return relFiles, nil
}
