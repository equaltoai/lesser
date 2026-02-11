package main

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyringEnabled_ParsesEnv(t *testing.T) {
	t.Setenv("LESSER_AUTH_KEYRING", "")
	require.False(t, keyringEnabled())

	t.Setenv("LESSER_AUTH_KEYRING", "0")
	require.False(t, keyringEnabled())

	t.Setenv("LESSER_AUTH_KEYRING", "false")
	require.False(t, keyringEnabled())

	t.Setenv("LESSER_AUTH_KEYRING", "no")
	require.False(t, keyringEnabled())

	t.Setenv("LESSER_AUTH_KEYRING", "1")
	require.True(t, keyringEnabled())

	t.Setenv("LESSER_AUTH_KEYRING", "true")
	require.True(t, keyringEnabled())

	t.Setenv("LESSER_AUTH_KEYRING", "auto")
	require.True(t, keyringEnabled())
}

func TestGenerateKeyringSecret_ReturnsBase64(t *testing.T) {
	secret, err := generateKeyringSecret()
	require.NoError(t, err)
	require.NotEmpty(t, secret)

	raw, err := base64.StdEncoding.DecodeString(secret)
	require.NoError(t, err)
	require.Len(t, raw, 32)
}

func TestGetOrCreateKeyringSecret_ReturnsExisting(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	origSave := keyringSaveSecretFn
	origGenerate := generateKeyringSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
		keyringSaveSecretFn = origSave
		generateKeyringSecretFn = origGenerate
	})

	baseURL := "https://example.com"
	expectedAccount := keyringAccountName(baseURL)

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(account string) (string, error) {
		require.Equal(t, expectedAccount, account)
		return "existing-secret", nil
	}
	keyringSaveSecretFn = func(string, string) error {
		t.Fatalf("unexpected save")
		return nil
	}
	generateKeyringSecretFn = func() (string, error) {
		t.Fatalf("unexpected generate")
		return "", nil
	}

	secret, err := getOrCreateKeyringSecret(baseURL)
	require.NoError(t, err)
	require.Equal(t, "existing-secret", secret)
}

func TestGetOrCreateKeyringSecret_CreatesWhenMissing(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	origSave := keyringSaveSecretFn
	origGenerate := generateKeyringSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
		keyringSaveSecretFn = origSave
		generateKeyringSecretFn = origGenerate
	})

	baseURL := "https://example.com"
	expectedAccount := keyringAccountName(baseURL)

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(account string) (string, error) {
		require.Equal(t, expectedAccount, account)
		return "", errKeyringNotFound
	}

	var saved bool
	keyringSaveSecretFn = func(account, secret string) error {
		require.Equal(t, expectedAccount, account)
		require.Equal(t, "new-secret", secret)
		saved = true
		return nil
	}
	generateKeyringSecretFn = func() (string, error) { return "new-secret", nil }

	secret, err := getOrCreateKeyringSecret(baseURL)
	require.NoError(t, err)
	require.Equal(t, "new-secret", secret)
	require.True(t, saved)
}

func TestGetOrCreateKeyringSecret_ErrorsOnEmptyBaseURL(t *testing.T) {
	_, err := getOrCreateKeyringSecret("")
	require.Error(t, err)
}

func TestGetOrCreateKeyringSecret_Unavailable(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	t.Cleanup(func() { keyringIsAvailableFn = origAvailable })

	keyringIsAvailableFn = func() bool { return false }

	_, err := getOrCreateKeyringSecret("https://example.com")
	require.Error(t, err)
}

func TestGetOrCreateKeyringSecret_LoadError(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
	})

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(string) (string, error) { return "", errors.New("boom") }

	_, err := getOrCreateKeyringSecret("https://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestGetOrCreateKeyringSecret_EmptyLoadedSecret(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
	})

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(string) (string, error) { return "   ", nil }

	_, err := getOrCreateKeyringSecret("https://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestGetOrCreateKeyringSecret_GenerateError(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	origGenerate := generateKeyringSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
		generateKeyringSecretFn = origGenerate
	})

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(string) (string, error) { return "", errKeyringNotFound }
	generateKeyringSecretFn = func() (string, error) { return "", errors.New("nope") }

	_, err := getOrCreateKeyringSecret("https://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "nope")
}

func TestGetOrCreateKeyringSecret_EmptyGeneratedSecret(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	origGenerate := generateKeyringSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
		generateKeyringSecretFn = origGenerate
	})

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(string) (string, error) { return "", errKeyringNotFound }
	generateKeyringSecretFn = func() (string, error) { return "   ", nil }

	_, err := getOrCreateKeyringSecret("https://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "generated")
	require.Contains(t, err.Error(), "empty")
}

func TestGetOrCreateKeyringSecret_SaveError(t *testing.T) {
	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	origSave := keyringSaveSecretFn
	origGenerate := generateKeyringSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
		keyringSaveSecretFn = origSave
		generateKeyringSecretFn = origGenerate
	})

	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(string) (string, error) { return "", errKeyringNotFound }
	generateKeyringSecretFn = func() (string, error) { return "new-secret", nil }
	keyringSaveSecretFn = func(string, string) error { return errors.New("save-fail") }

	_, err := getOrCreateKeyringSecret("https://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "save-fail")
}

func TestDeriveAuthKey_UsesKeyringSecretWhenEnabled(t *testing.T) {
	t.Setenv("LESSER_AUTH_KEYRING", "1")
	t.Setenv("HOME", t.TempDir())

	origAvailable := keyringIsAvailableFn
	origLoad := keyringLoadSecretFn
	t.Cleanup(func() {
		keyringIsAvailableFn = origAvailable
		keyringLoadSecretFn = origLoad
	})

	baseURL := "https://example.com"
	keyringIsAvailableFn = func() bool { return true }
	keyringLoadSecretFn = func(string) (string, error) { return "kr-secret", nil }

	key := deriveAuthKey(baseURL, "")
	expected := sha256.Sum256([]byte("kr-secret|" + baseURL))
	require.Equal(t, expected[:], key)
}

func TestDeriveAuthKey_KeyringUnavailableFallsBackToMachineSecret(t *testing.T) {
	t.Setenv("LESSER_AUTH_KEYRING", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USER", "bob")

	origAvailable := keyringIsAvailableFn
	t.Cleanup(func() { keyringIsAvailableFn = origAvailable })

	baseURL := "https://example.com"
	keyringIsAvailableFn = func() bool { return false }

	material := machineDerivedSecret()
	expected := sha256.Sum256([]byte(material + "|" + baseURL))
	require.Equal(t, expected[:], deriveAuthKey(baseURL, ""))
}
