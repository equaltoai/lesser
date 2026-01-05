package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerifyOAuthClientSecret(t *testing.T) {
	t.Parallel()

	secret := "super-secret"
	hashed, err := HashOAuthClientSecret(secret)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hashed, OAuthClientSecretHashPrefix))

	ok, needsMigration, err := VerifyOAuthClientSecret(secret, hashed)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, needsMigration)

	ok, needsMigration, err = VerifyOAuthClientSecret("wrong", hashed)
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, needsMigration)
}

func TestVerifyOAuthClientSecret_LegacyPlaintextNeedsMigration(t *testing.T) {
	t.Parallel()

	secret := "legacy-secret"
	ok, needsMigration, err := VerifyOAuthClientSecret(secret, secret)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, needsMigration)
}

func TestVerifyOAuthClientSecret_RawBcryptHashSupported(t *testing.T) {
	t.Parallel()

	secret := "raw-hash-secret"
	raw, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	require.NoError(t, err)

	ok, needsMigration, err := VerifyOAuthClientSecret(secret, string(raw))
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, needsMigration)
}
