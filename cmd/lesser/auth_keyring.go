package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	lesserCLIKeyringServiceName = "com.equaltoai.lesser"
	lesserCLIKeyringItemLabel   = "Lesser CLI auth session"
)

var (
	errKeyringNotFound = errors.New("keyring secret not found")

	keyringIsAvailableFn    = keyringIsAvailable
	keyringLoadSecretFn     = keyringLoadSecret
	keyringSaveSecretFn     = keyringSaveSecret
	generateKeyringSecretFn = generateKeyringSecret
)

func keyringEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("LESSER_AUTH_KEYRING")))
	if raw == "" {
		return false
	}
	switch raw {
	case "0", "false", "no", "n", "off":
		return false
	default:
		return true
	}
}

func keyringAccountName(baseURL string) string {
	return "cli-auth-secret-" + authBaseURLHash(baseURL)
}

func getOrCreateKeyringSecret(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("base url is required")
	}

	if !keyringIsAvailableFn() {
		return "", fmt.Errorf("keyring unavailable")
	}

	account := keyringAccountName(baseURL)
	secret, err := keyringLoadSecretFn(account)
	if err == nil {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			return "", fmt.Errorf("keyring secret is empty")
		}
		return secret, nil
	}
	if !errors.Is(err, errKeyringNotFound) {
		return "", err
	}

	secret, err = generateKeyringSecretFn()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("generated keyring secret is empty")
	}

	if err := keyringSaveSecretFn(account, secret); err != nil {
		return "", err
	}
	return secret, nil
}

func generateKeyringSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
