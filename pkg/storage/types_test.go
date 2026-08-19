package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshTokenReplacementHashIsStableAndNonSecret(t *testing.T) {
	hash := RefreshTokenReplacementHash(" successor-token ")
	require.Len(t, hash, 64)
	require.Equal(t, RefreshTokenReplacementHash("successor-token"), hash)
	require.NotEqual(t, "successor-token", hash)
}
