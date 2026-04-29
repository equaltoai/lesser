//go:build darwin
// +build darwin

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
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

	cmd := exec.CommandContext(ctx, "security", "add-generic-password", //nolint:gosec // keychain tool invocation
		"-s", lesserCLIKeyringServiceName,
		"-a", account,
		"-U",
		"-w",
	)
	cmd.Stdin = strings.NewReader(macOSKeychainPromptInput(secret))
	// Detach from the controlling terminal so the Keychain prompt path
	// (getpass(3) inside SecurityTool) cannot open /dev/tty and must fall
	// back to reading the supplied Stdin pipe.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("security add-generic-password: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("security add-generic-password failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// macOSKeychainPromptInput returns the input expected by `security add-generic-password -w`
// when -w is provided without an argv value. That prompt path asks for the
// password twice, so feed matching entries while keeping the secret out of the
// process argument list.
func macOSKeychainPromptInput(secret string) string {
	return secret + "\n" + secret + "\n"
}
