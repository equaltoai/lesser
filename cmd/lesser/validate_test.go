package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBaseDomain(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "ok", input: "Example.COM", want: "example.com"},
		{name: "trim dot", input: "example.com.", want: "example.com"},
		{name: "empty", input: "", wantErr: true},
		{name: "no dot", input: "localhost", wantErr: true},
		{name: "slash", input: "example.com/path", wantErr: true},
		{name: "invalid label", input: "ex ample.com", wantErr: true},
		{name: "label too long", input: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example.com", wantErr: true},
		{name: "leading dash", input: "-a.example.com", wantErr: true},
		{name: "trailing dash", input: "a-.example.com", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeBaseDomain(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestEnsureLocalStateDir_UsesInjectedHome(t *testing.T) {
	previousHome := userHomeDirFn
	previousMkdir := mkdirAllFn
	t.Cleanup(func() {
		userHomeDirFn = previousHome
		mkdirAllFn = previousMkdir
	})

	home := t.TempDir()
	userHomeDirFn = func() (string, error) { return home, nil }
	mkdirAllFn = func(path string, perm fs.FileMode) error {
		require.Equal(t, filepath.Join(home, ".lesser", "app", "example.com"), path)
		require.Equal(t, fs.FileMode(0o700), perm)
		return nil
	}

	dir, err := ensureLocalStateDir("app", "example.com")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".lesser", "app", "example.com"), dir)
}

func TestEnsureLocalStateDir_PropagatesHomeError(t *testing.T) {
	previousHome := userHomeDirFn
	t.Cleanup(func() { userHomeDirFn = previousHome })

	userHomeDirFn = func() (string, error) { return "", errors.New("boom") }

	_, err := ensureLocalStateDir("app", "example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve home dir")
}
