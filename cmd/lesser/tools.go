package main

import (
	"fmt"
	"os/exec"
)

var (
	ensureToolAvailableFn               = ensureToolAvailable
	ensureToolsAvailableFn              = ensureToolsAvailable
	ensureReleaseDeployToolsAvailableFn = ensureReleaseDeployToolsAvailable
)

func ensureToolAvailable(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("required tool %q not found on PATH", name)
	}
	return nil
}

func ensureToolsAvailable() error {
	for _, name := range []string{"aws", "cdk", "go", "pnpm"} {
		if err := ensureToolAvailableFn(name); err != nil {
			return err
		}
	}
	return nil
}

func ensureReleaseDeployToolsAvailable() error {
	for _, name := range []string{"aws", "cdk", "go"} {
		if err := ensureToolAvailableFn(name); err != nil {
			return err
		}
	}
	return nil
}

func ensureAWSCLIToolAvailable() error {
	return ensureToolAvailableFn("aws")
}
