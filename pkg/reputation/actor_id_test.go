package reputation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateActorID(t *testing.T) {
	require.NoError(t, ValidateActorID("https://example.com/users/alice"))
	require.NoError(t, ValidateActorID("http://remote.example/@bob"))

	invalid := []string{
		"",
		"acct:alice@example.com",
		"https://evil.example@real.example/users/alice",
		"https://example.com/users/alice?next=https://evil.example",
		"https://example.com/users/alice#main-key",
		"https://example.com/users/../admin",
		"https://example.com/users/%2e%2e/admin",
		"https://example.com/users/alice\nHost: evil.example",
		"https://example.com/" + strings.Repeat("a", 2001),
	}
	for _, actorID := range invalid {
		t.Run(actorID, func(t *testing.T) {
			require.Error(t, ValidateActorID(actorID))
		})
	}
}
