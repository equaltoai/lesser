//go:build !darwin && !linux && !windows
// +build !darwin,!linux,!windows

package main

import "fmt"

func keyringIsAvailable() bool {
	return false
}

func keyringLoadSecret(string) (string, error) {
	return "", errKeyringNotFound
}

func keyringSaveSecret(string, string) error {
	return fmt.Errorf("keyring unsupported on this platform")
}
