package lift

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerAuthHelpers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)
	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"read", "write"}, "sess-1")

	t.Run("GetBearerToken", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, token, h.getBearerTokenLift(ctx))
	})

	t.Run("AuthenticateWithScopeSuccess", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)
		claims, authErr := h.authenticateWithScope(ctx, "read")
		require.NoError(t, authErr)
		require.NotNil(t, claims)
	})

	t.Run("AuthenticateWithScopeInvalidRequired", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)
		_, authErr := h.authenticateWithScope(ctx, "invalid-scope")
		require.Error(t, authErr)
	})

	t.Run("GetOptionalAuthenticatedUser", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)
		username := h.getOptionalAuthenticatedUser(ctx)
		require.Equal(t, "alice", username)
	})
}
