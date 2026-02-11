//go:build linux
// +build linux

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func keyringIsAvailable() bool {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// We don't care about the result here; we just want to ensure the command doesn't hang on headless systems.
	cmd := exec.CommandContext(ctx, "secret-tool", "lookup", //nolint:gosec // keyring tool invocation
		"service", lesserCLIKeyringServiceName,
		"account", "lesser-cli-keyring-healthcheck",
	)
	_ = cmd.Run()

	return ctx.Err() == nil
}

func keyringLoadSecret(account string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "secret-tool", "lookup", //nolint:gosec // keyring tool invocation
		"service", lesserCLIKeyringServiceName,
		"account", account,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("secret-tool lookup: %w", ctx.Err())
	}
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return "", errKeyringNotFound
		}
		return "", fmt.Errorf("secret-tool lookup failed: %w: %s", err, text)
	}

	secret := strings.TrimSpace(string(output))
	if secret == "" {
		return "", errKeyringNotFound
	}
	return secret, nil
}

func keyringSaveSecret(account string, secret string) error {
	// Best-effort clear to avoid duplicates.
	_ = exec.Command("secret-tool", "clear", //nolint:gosec // keyring tool invocation
		"service", lesserCLIKeyringServiceName,
		"account", account,
	).Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "secret-tool", "store", //nolint:gosec // keyring tool invocation
		"--label="+lesserCLIKeyringItemLabel,
		"service", lesserCLIKeyringServiceName,
		"account", account,
	)
	cmd.Stdin = strings.NewReader(secret)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("secret-tool store: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("secret-tool store failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
