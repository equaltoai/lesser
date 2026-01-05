package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstantTimeCompare(t *testing.T) {
	assert.True(t, ConstantTimeCompare("abc", "abc"))
	assert.False(t, ConstantTimeCompare("abc", "abd"))
	assert.False(t, ConstantTimeCompare("abc", "abcd"))
}

func TestPadToLength(t *testing.T) {
	assert.Equal(t, "ab", padToLength("abc", 2))
	assert.Equal(t, "abc", padToLength("abc", 3))
	assert.Len(t, padToLength("a", 3), 3)
}

func TestSecureRandInt(t *testing.T) {
	assert.Equal(t, 0, secureRandInt(0))
	assert.Equal(t, 0, secureRandInt(-1))

	n := secureRandInt(10)
	assert.GreaterOrEqual(t, n, 0)
	assert.Less(t, n, 10)
}

func TestTimingSafeTokenValidation(t *testing.T) {
	assert.True(t, TimingSafeTokenValidation("token", "token"))
	assert.False(t, TimingSafeTokenValidation("token", "other"))
	assert.False(t, TimingSafeTokenValidation("token", "token-longer"))
}

func TestValidateAPIKey(t *testing.T) {
	require.NoError(t, ValidateAPIKey("k", func() (string, error) { return "k", nil }))

	boom := errors.New("boom")
	require.ErrorIs(t, ValidateAPIKey("k", func() (string, error) { return "", boom }), boom)

	require.ErrorIs(t, ValidateAPIKey("k", func() (string, error) { return "other", nil }), ErrInvalidAPIKey)
}

func TestTimingSafeStringSliceContains(t *testing.T) {
	assert.True(t, TimingSafeStringSliceContains([]string{"a", "b", "c"}, "b"))
	assert.False(t, TimingSafeStringSliceContains([]string{"a", "b", "c"}, "x"))
}

func TestValidateSessionToken(t *testing.T) {
	assert.NoError(t, ValidateSessionToken("t", func(string) (bool, error) { return true, nil }))

	assert.ErrorIs(t, ValidateSessionToken("t", func(string) (bool, error) { return false, nil }), ErrInvalidToken)

	boom := errors.New("boom")
	assert.ErrorIs(t, ValidateSessionToken("t", func(string) (bool, error) { return false, boom }), boom)
}
