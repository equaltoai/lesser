package providers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenResponse_JSONTagsAndOmitEmpty(t *testing.T) {
	resp := TokenResponse{
		AccessToken: "access",
		TokenType:   "bearer",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "access", m["access_token"])
	require.Equal(t, "bearer", m["token_type"])
	require.NotContains(t, m, "refresh_token")
	require.NotContains(t, m, "expires_in")
	require.NotContains(t, m, "scope")
}

func TestTokenResponse_JSONIncludesOptionalFields(t *testing.T) {
	resp := TokenResponse{
		AccessToken:  "access",
		TokenType:    "bearer",
		RefreshToken: "refresh",
		ExpiresIn:    3600,
		Scope:        "read write",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "refresh", m["refresh_token"])
	require.Equal(t, float64(3600), m["expires_in"])
	require.Equal(t, "read write", m["scope"])
}
