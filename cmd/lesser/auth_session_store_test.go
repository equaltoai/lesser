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

func TestAuthSessionStore_CorruptFileFormat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	key := deriveAuthKey(baseURL, "test-secret")

	path, err := authSessionFile(baseURL)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("not-a-session"), 0o600))

	_, err = readAuthSession(baseURL, key)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported session file format")
}

func TestAuthSessionStore_CorruptFileTooShort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	key := deriveAuthKey(baseURL, "test-secret")

	path, err := authSessionFile(baseURL)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))

	corrupt := append(append([]byte(nil), authSessionMagic...), []byte("short")...)
	require.NoError(t, os.WriteFile(path, corrupt, 0o600))

	_, err = readAuthSession(baseURL, key)
	require.Error(t, err)
	require.Contains(t, err.Error(), "corrupt session file")
}

func TestAuthSessionStore_WrongKeyFails(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	keyGood := deriveAuthKey(baseURL, "test-secret")
	keyBad := deriveAuthKey(baseURL, "other-secret")

	require.NoError(t, writeAuthSession(baseURL, keyGood, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-token-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	_, err := readAuthSession(baseURL, keyBad)
	require.Error(t, err)
}

func TestAuthSessionStore_DirectoryPermissions(t *testing.T) {
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

	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestAuthSessionStore_ReadAuthSecret_PrioritizesEnv(t *testing.T) {
	t.Setenv("LESSER_AUTH_SECRET", "from-env")
	secret, err := readAuthSecret(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	require.Equal(t, "from-env", secret)
}

func TestAuthSessionStore_ReadAuthSecret_FromFile(t *testing.T) {
	t.Setenv("LESSER_AUTH_SECRET", "")

	secretFile := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secretFile, []byte("from-file\n"), 0o600))

	secret, err := readAuthSecret(secretFile)
	require.NoError(t, err)
	require.Equal(t, "from-file", secret)
}

func TestAuthSessionStore_ReadAuthSecret_FileError(t *testing.T) {
	t.Setenv("LESSER_AUTH_SECRET", "")
	_, err := readAuthSecret(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestAuthSessionStore_MachineDerivedSecret_DoesNotPanic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NotEmpty(t, machineDerivedSecret())
	_ = readMachineID()
}

func TestAuthSessionStore_WriteAuthSession_NilSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.Error(t, writeAuthSession("https://example.com", make([]byte, 32), nil))
}

func TestAuthSessionStore_DeleteAuthSession_ErrorsOnNonEmptyDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	path, err := authSessionFile(baseURL)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0o600))

	removed, err := deleteAuthSession(baseURL)
	require.Error(t, err)
	require.False(t, removed)
}

func TestAuthSessionStore_EncryptDecrypt_KeyLengthErrors(t *testing.T) {
	_, err := encryptAuthBlob([]byte("x"), []byte("short"), "https://example.com")
	require.Error(t, err)

	_, err = decryptAuthBlob([]byte("x"), []byte("short"), "https://example.com")
	require.Error(t, err)
}

func TestAuthSessionStore_NormalizeBaseURL_Validation(t *testing.T) {
	_, err := normalizeBaseURL("")
	require.Error(t, err)

	_, err = normalizeBaseURL("example.com")
	require.Error(t, err)

	out, err := normalizeBaseURL("https://example.com/")
	require.NoError(t, err)
	require.Equal(t, "https://example.com", out)
}
