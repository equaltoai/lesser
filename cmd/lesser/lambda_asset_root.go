package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

func prepareLambdaAssetRoot(stateDir string) (string, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("create deploy state dir: %w", err)
	}

	assetRoot, err := os.MkdirTemp(stateDir, "deploy-lambda-assets.")
	if err != nil {
		return "", fmt.Errorf("create lambda asset root: %w", err)
	}
	return assetRoot, nil
}

func stageLocalLambdaAssets(repoRoot string, assetRoot string) ([]string, error) {
	lambdaNames, err := releaseassets.CanonicalLambdaNames(repoRoot)
	if err != nil {
		return nil, err
	}

	staged := make([]string, 0, len(lambdaNames))
	for _, lambdaName := range lambdaNames {
		sourcePath := filepath.Join(repoRoot, "bin", lambdaName+".zip")
		targetPath := filepath.Join(assetRoot, "bin", lambdaName+".zip")
		if err := copyFile(targetPath, sourcePath); err != nil {
			return nil, fmt.Errorf("stage lambda asset %s: %w", lambdaName, err)
		}
		staged = append(staged, targetPath)
	}

	return staged, nil
}

var (
	prepareLambdaAssetRootFn = prepareLambdaAssetRoot
	stageLocalLambdaAssetsFn = stageLocalLambdaAssets
)
