//go:build darwin
// +build darwin

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func keyringIsAvailable() bool {
	_, err := exec.LookPath("security")
	return err == nil
}

func keyringLoadSecret(account string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", "find-generic-password", //nolint:gosec // keychain tool invocation
		"-s", lesserCLIKeyringServiceName,
		"-a", account,
		"-w",
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("security find-generic-password: %w", ctx.Err())
	}
	if err != nil {
		text := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(text), "could not be found") {
			return "", errKeyringNotFound
		}
		return "", fmt.Errorf("security find-generic-password failed: %w: %s", err, text)
	}

	secret := strings.TrimSpace(string(output))
	if secret == "" {
		return "", errKeyringNotFound
	}
	return secret, nil
}

func keyringSaveSecret(account string, secret string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", "add-generic-password", //nolint:gosec // keychain tool invocation (secret is intended)
		"-s", lesserCLIKeyringServiceName,
		"-a", account,
		"-w", secret,
		"-U",
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("security add-generic-password: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("security add-generic-password failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
