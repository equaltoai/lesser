package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

func deployWorkspaceRoot(stateDir string) string {
	return filepath.Join(stateDir, "deploy")
}

func deployLambdaAssetRoot(stateDir string) string {
	return filepath.Join(deployWorkspaceRoot(stateDir), "lambda-assets")
}

func deployCdkOutputsPath(stateDir string, stackName string) string {
	return filepath.Join(deployWorkspaceRoot(stateDir), "cdk-outputs", stackName+".json")
}

func prepareLambdaAssetRoot(stateDir string) (string, error) {
	assetRoot := deployLambdaAssetRoot(stateDir)
	if err := os.RemoveAll(assetRoot); err != nil {
		return "", fmt.Errorf("reset lambda asset root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(assetRoot, "bin"), 0o750); err != nil {
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
