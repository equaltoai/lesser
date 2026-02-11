package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthSessionStore_EncryptsAndDecrypts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	key := deriveAuthKey(baseURL, "test-secret")

	session := &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-token-1",
		Username:     "alice",
		Scopes:       []string{"read", "write"},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	require.NoError(t, writeAuthSession(baseURL, key, session))

	path, err := authSessionFile(baseURL)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), session.RefreshToken)

	roundTrip, err := readAuthSession(baseURL, key)
	require.NoError(t, err)
	require.Equal(t, session.ClientID, roundTrip.ClientID)
	require.Equal(t, session.Username, roundTrip.Username)
	require.Equal(t, session.RefreshToken, roundTrip.RefreshToken)
	require.Equal(t, session.Scopes, roundTrip.Scopes)
}

func TestAuthSessionStore_Delete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	key := deriveAuthKey(baseURL, "test-secret")

	require.NoError(t, writeAuthSession(baseURL, key, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-token-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	path, err := authSessionFile(baseURL)
	require.NoError(t, err)
	require.FileExists(t, path)

	removed, err := deleteAuthSession(baseURL)
	require.NoError(t, err)
	require.True(t, removed)
	require.NoFileExists(t, path)

	// ensure we don't accidentally remove the parent directory; it's fine to keep it.
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
