package accounts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoredUsernameMatches_LegacyMixedCaseAccountOwnership(t *testing.T) {
	require.True(t, storedUsernameMatches("Medic", "medic"))
	require.True(t, storedUsernameMatches("medic", "Medic"))
	require.True(t, storedUsernameMatches("Arch", "ARCH"))
	require.True(t, storedUsernameMatches("  Medic  ", " medic "))
	require.False(t, storedUsernameMatches("Medic", "Healer"))
	require.False(t, storedUsernameMatches("", "medic"))
	require.False(t, storedUsernameMatches("Medic", ""))
}
