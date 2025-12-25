package main

import (
	"fmt"
	"os/exec"
)

func ensureToolsAvailable() error {
	for _, name := range []string{"aws", "cdk", "go", "pnpm"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("required tool %q not found on PATH", name)
		}
	}
	return nil
}

func ensureAWSCLIToolAvailable() error {
	if _, err := exec.LookPath("aws"); err != nil {
		return fmt.Errorf("required tool %q not found on PATH", "aws")
	}
	return nil
}
