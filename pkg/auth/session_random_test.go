package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSecureTokenUsesCryptographicRandomness(t *testing.T) {
	first, err := generateSecureToken()
	require.NoError(t, err)
	second, err := generateSecureToken()
	require.NoError(t, err)

	require.Len(t, first, 44)
	require.Len(t, second, 44)
	require.NotEqual(t, first, second)
}

func TestGenerateSecureTokenFailsClosedOnRandomError(t *testing.T) {
	originalReader := sessionRandRead
	sessionRandRead = func([]byte) (int, error) {
		return 0, errors.New("random unavailable")
	}
	defer func() { sessionRandRead = originalReader }()

	token, err := generateSecureToken()
	require.Error(t, err)
	require.Empty(t, token)
}
