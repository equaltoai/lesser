package activitypub

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{name: "empty", in: "", want: ErrEmptyDomain},
		{name: "ip rejected", in: "127.0.0.1", want: ErrIPAddressAsDomain},
		{name: "bad format", in: "not a domain", want: ErrInvalidDomainFormat},
		{name: "consecutive dots", in: "example..com", want: ErrInvalidDomainFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomain(tt.in)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.want))
		})
	}

	require.NoError(t, ValidateDomain("example.com"))
}

func TestValidateActorID(t *testing.T) {
	t.Run("malformed URL", func(t *testing.T) {
		err := ValidateActorID("://bad")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidActorIDURL)
	})

	t.Run("invalid scheme", func(t *testing.T) {
		err := ValidateActorID("ftp://example.com/users/alice")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrActorIDScheme)
	})

	t.Run("invalid domain", func(t *testing.T) {
		err := ValidateActorID("https://127.0.0.1/users/alice")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidDomainInActorID)
	})

	t.Run("missing path", func(t *testing.T) {
		err := ValidateActorID("https://example.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrActorIDMissingPath)
	})

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, ValidateActorID("https://example.com/users/alice"))
	})
}

func TestValidateWebfinger(t *testing.T) {
	t.Run("bad format", func(t *testing.T) {
		err := ValidateWebfinger("alice@example.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidWebfingerFormat)
	})

	t.Run("bad username", func(t *testing.T) {
		err := ValidateWebfinger("acct:alice bob@example.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidUsernameInWebfinger)
	})

	t.Run("bad domain", func(t *testing.T) {
		err := ValidateWebfinger("acct:alice@127.0.0.1")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidDomainInWebfinger)
	})

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, ValidateWebfinger("acct:alice@example.com"))
	})
}
