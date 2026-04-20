package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashPasswordAndStrengthHelperEdges(t *testing.T) {
	t.Parallel()

	_, err := HashPassword("short")
	require.ErrorIs(t, err, ErrPasswordTooShort)

	_, err = HashPassword(strings.Repeat("a", 73))
	require.ErrorIs(t, err, ErrPasswordTooLong)

	hash, err := HashPassword("ValidPass123!")
	require.NoError(t, err)
	require.NoError(t, VerifyPassword("ValidPass123!", hash))

	cfg := PasswordStrengthConfig{
		MinLength:               0,
		LongLength:              0,
		SequentialPatternMinRun: 0,
		LongBonus:               2,
	}
	require.Equal(t, 0, PasswordStrengthWithConfig("short", cfg))
	require.True(t, hasSequentialRun("abcd", 1))
	require.False(t, hasSequentialRun("abx", 4))

	require.Equal(t, "Very Weak", PasswordStrengthLabel(-1))
	require.Equal(t, "Very Strong", PasswordStrengthLabel(99))

	hints := GeneratePasswordHint("ALLUPPER123!!!")
	require.Contains(t, hints, "Add lowercase letters")
}
