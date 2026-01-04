package common

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashOAuthClientSecret(t *testing.T) {
	t.Run("empty secret returns error", func(t *testing.T) {
		hash, err := HashOAuthClientSecret("")
		require.Error(t, err)
		assert.ErrorIs(t, err, errOAuthClientSecretEmpty)
		assert.Empty(t, hash)
	})

	t.Run("too long secret returns error", func(t *testing.T) {
		tooLong := make([]byte, 73)
		for i := range tooLong {
			tooLong[i] = 'a'
		}
		hash, err := HashOAuthClientSecret(string(tooLong))
		require.Error(t, err)
		assert.ErrorIs(t, err, errOAuthClientSecretTooLong)
		assert.Empty(t, hash)
	})

	t.Run("returns versioned bcrypt hash", func(t *testing.T) {
		hash, err := HashOAuthClientSecret("secret")
		require.NoError(t, err)
		require.NotEmpty(t, hash)
		assert.Contains(t, hash, OAuthClientSecretHashPrefix)

		valid, needsMigration, err := VerifyOAuthClientSecret("secret", hash)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.False(t, needsMigration)
	})
}

func TestVerifyOAuthClientSecret(t *testing.T) {
	t.Run("empty inputs return false without error", func(t *testing.T) {
		valid, needsMigration, err := VerifyOAuthClientSecret("", "")
		require.NoError(t, err)
		assert.False(t, valid)
		assert.False(t, needsMigration)
	})

	t.Run("prefixed bcrypt hash validates and does not require migration", func(t *testing.T) {
		stored, err := HashOAuthClientSecret("secret")
		require.NoError(t, err)

		valid, needsMigration, err := VerifyOAuthClientSecret("secret", stored)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.False(t, needsMigration)

		valid, needsMigration, err = VerifyOAuthClientSecret("wrong", stored)
		require.NoError(t, err)
		assert.False(t, valid)
		assert.False(t, needsMigration)
	})

	t.Run("raw bcrypt hash validates", func(t *testing.T) {
		hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
		require.NoError(t, err)

		valid, needsMigration, err := VerifyOAuthClientSecret("secret", string(hash))
		require.NoError(t, err)
		assert.True(t, valid)
		assert.False(t, needsMigration)
	})

	t.Run("invalid bcrypt hash returns an error", func(t *testing.T) {
		valid, needsMigration, err := VerifyOAuthClientSecret("secret", OAuthClientSecretHashPrefix+"not-a-hash")
		require.Error(t, err)
		assert.False(t, valid)
		assert.False(t, needsMigration)
	})

	t.Run("legacy plaintext validates and flags migration", func(t *testing.T) {
		valid, needsMigration, err := VerifyOAuthClientSecret("secret", "secret")
		require.NoError(t, err)
		assert.True(t, valid)
		assert.True(t, needsMigration)

		valid, needsMigration, err = VerifyOAuthClientSecret("wrong", "secret")
		require.NoError(t, err)
		assert.False(t, valid)
		assert.False(t, needsMigration)
	})
}
