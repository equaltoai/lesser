// Package common provides shared helpers across the Lesser codebase.
// revive:disable-next-line var-naming // package name is a longstanding public import path
package common

import (
	"crypto/subtle"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	// OAuthClientSecretHashPrefix marks hashed secrets stored for OAuth clients.
	// Prefixing avoids ambiguity with legacy plaintext values.
	OAuthClientSecretHashPrefix = "bcrypt:"
)

var (
	errOAuthClientSecretEmpty   = errors.New("oauth client secret is empty")
	errOAuthClientSecretTooLong = errors.New("oauth client secret is too long for bcrypt")
)

// HashOAuthClientSecret returns a versioned hash string suitable for storage.
func HashOAuthClientSecret(secret string) (string, error) {
	if secret == "" {
		return "", errOAuthClientSecretEmpty
	}
	// bcrypt truncates passwords at 72 bytes, which would silently weaken security.
	// Secrets should always be below this limit.
	if len(secret) > 72 {
		return "", errOAuthClientSecretTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return OAuthClientSecretHashPrefix + string(hash), nil
}

func oauthClientSecretHashPayload(stored string) (string, bool) {
	if strings.HasPrefix(stored, OAuthClientSecretHashPrefix) {
		return strings.TrimPrefix(stored, OAuthClientSecretHashPrefix), true
	}

	// Support legacy (or future) raw bcrypt hashes without the prefix.
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return stored, true
	}

	return "", false
}

// VerifyOAuthClientSecret verifies a provided secret against a stored value.
// It returns (valid, needsMigration, err). needsMigration is true when the stored
// value is a legacy plaintext secret and the verification succeeded.
func VerifyOAuthClientSecret(providedSecret, storedValue string) (bool, bool, error) {
	if providedSecret == "" || storedValue == "" {
		return false, false, nil
	}

	if hash, ok := oauthClientSecretHashPayload(storedValue); ok {
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(providedSecret))
		if err == nil {
			return true, false, nil
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, false, nil
		}
		return false, false, err
	}

	// Legacy plaintext comparison (best effort constant-time).
	if subtle.ConstantTimeCompare([]byte(providedSecret), []byte(storedValue)) == 1 {
		return true, true, nil
	}
	return false, false, nil
}
