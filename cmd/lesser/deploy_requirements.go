package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type deploySourceRequirement struct {
	RelativePath string
	Description  string
}

var releaseDeployRequirements = []deploySourceRequirement{
	{
		RelativePath: filepath.Join("infra", "cdk", "cdk.json"),
		Description:  "CDK application source",
	},
	{
		RelativePath: filepath.Join("infra", "cdk", "inventory", "lambdas.go"),
		Description:  "canonical lambda inventory source",
	},
}

var deploySourceRequirements = append(append([]deploySourceRequirement{}, releaseDeployRequirements...), deploySourceRequirement{
	RelativePath: filepath.Join("auth-ui", "package.json"),
	Description:  "auth-ui source",
})

var validateDeploySourceInputsFn = validateDeploySourceInputs
var validateReleaseDeployInputsFn = validateReleaseDeployInputs

func validateDeploySourceInputs(repoRoot string) error {
	return validateDeployRequirements(repoRoot, deploySourceRequirements)
}

func validateReleaseDeployInputs(repoRoot string) error {
	return validateDeployRequirements(repoRoot, releaseDeployRequirements)
}

func validateDeployRequirements(repoRoot string, requirements []deploySourceRequirement) error {
	for _, requirement := range requirements {
		fullPath := filepath.Join(repoRoot, requirement.RelativePath)
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf(
				"deploy requires repo-local %s at %s: %w",
				requirement.Description,
				fullPath,
				err,
			)
		}
		if info.IsDir() {
			return fmt.Errorf(
				"deploy requires repo-local %s file at %s",
				requirement.Description,
				fullPath,
			)
		}
	}

	return nil
}
