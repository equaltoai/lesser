package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveReceiptPath_Explicit(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	require.NoError(t, writeReceipt(path, newUpReceipt("app", "example.com", "prof", "acct", "us-east-1", nil, hostedZone{})))

	got, err := resolveReceiptPath("app", "example.com", path)
	require.NoError(t, err)
	require.Equal(t, path, got)
}

func TestResolveReceiptPath_DefaultHome(t *testing.T) {
	previousHome := userHomeDirFn
	t.Cleanup(func() { userHomeDirFn = previousHome })

	home := t.TempDir()
	userHomeDirFn = func() (string, error) { return home, nil }

	statePath := filepath.Join(home, ".lesser", "app", "example.com", "state.json")
	require.NoError(t, writeReceipt(statePath, newUpReceipt("app", "example.com", "prof", "acct", "us-east-1", nil, hostedZone{})))

	got, err := resolveReceiptPath("app", "example.com", "")
	require.NoError(t, err)
	require.Equal(t, statePath, got)
}

func TestResolveReceiptPath_ExplicitMissingErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	_, err := resolveReceiptPath("app", "example.com", path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "deployment receipt not found at "+path)
}

func TestResolveReceiptPath_DefaultHomeMissingErrors(t *testing.T) {
	previousHome := userHomeDirFn
	t.Cleanup(func() { userHomeDirFn = previousHome })

	home := t.TempDir()
	userHomeDirFn = func() (string, error) { return home, nil }

	_, err := resolveReceiptPath("app", "example.com", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "deployment receipt not found at")
}

func TestResolveReceiptPath_DefaultHomeErrorPropagates(t *testing.T) {
	previousHome := userHomeDirFn
	t.Cleanup(func() { userHomeDirFn = previousHome })

	userHomeDirFn = func() (string, error) { return "", errors.New("boom") }

	_, err := resolveReceiptPath("app", "example.com", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve home dir")
}
