package main

import (
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestReceiptRoundTrip(t *testing.T) {
	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		[]naming.Stage{naming.StageDev, naming.StageLive},
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages["dev"].Domain = "dev.example.com"

	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	require.NoError(t, writeReceipt(path, receipt))

	readBack, err := readReceipt(path)
	require.NoError(t, err)
	require.Equal(t, receipt.App, readBack.App)
	require.Equal(t, receipt.BaseDomain, readBack.BaseDomain)
	require.Contains(t, readBack.Stages, "dev")
}

func TestReadReceipt_RejectsMissingFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")
	require.NoError(t, writeReceipt(path, &upReceipt{Version: 1}))

	_, err := readReceipt(path)
	require.Error(t, err)
}

func TestWriteReceipt_RejectsNil(t *testing.T) {
	require.Error(t, writeReceipt(filepath.Join(t.TempDir(), "state.json"), nil))
}

