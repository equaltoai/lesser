package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlatformKeyringSecretWritersAvoidProcessArguments(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)

	darwinSource := readKeyringSource(t, filepath.Join(dir, "auth_keyring_darwin.go"))
	require.NotContains(t, darwinSource, `"-w", secret`)
	require.NotContains(t, darwinSource, `"-w",secret`)
	require.Contains(t, darwinSource, "\"-U\",\n\t\t\"-w\",")
	require.Contains(t, darwinSource, "cmd.Stdin = strings.NewReader(secret")

	windowsSource := readKeyringSource(t, filepath.Join(dir, "auth_keyring_windows.go"))
	require.NotContains(t, windowsSource, "cmdkey")
	require.NotContains(t, windowsSource, "/pass:")
	require.NotContains(t, windowsSource, "$password = '%s'")
	require.Contains(t, windowsSource, "cmd.Stdin = strings.NewReader(encoded)")
	require.Contains(t, windowsSource, "CredWrite")
}

func readKeyringSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
