package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRound32NormalizeReadableStatusID_URLValidation(t *testing.T) {
	value, err := normalizeReadableStatusID("https://remote.example/users/bob/statuses/1")
	require.NoError(t, err)
	require.Equal(t, "https://remote.example/users/bob/statuses/1", value)

	_, err = normalizeReadableStatusID("https://")
	require.Error(t, err)

	_, err = normalizeReadableStatusID("https://remote.example/" + strings.Repeat("a", 600))
	require.Error(t, err)
}
