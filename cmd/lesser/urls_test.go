package main

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestStageURLs(t *testing.T) {
	urls := stageURLs(naming.StageDev, "example.com")
	require.Contains(t, urls["api"], "dev.example.com")
	require.Contains(t, urls["ws"], "wss://ws.dev.example.com")
	require.Contains(t, urls["media"], "https://media.dev.example.com")
}

func TestPrintStageURLs_NoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		printStageURLs([]naming.Stage{naming.StageDev}, "example.com")
	})
}

