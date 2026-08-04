package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestScopedDelegationCredentialMintAndValidation(t *testing.T) {
	t.Parallel()

	service := NewOAuthService("delegation-test-secret", nil, nil, nil)
	mint := func(expiresAt time.Time, principal, agent, contentClass string) string {
		token, err := service.generateAccessTokenWithMetadata(agent, DelegatedAgentRuntimeClientID, []string{ScopeWrite}, accessTokenMetadata{
			ExpiresAt:              expiresAt,
			ClientClass:            ClientClassAgent,
			IsAgent:                true,
			DelegatedBy:            "@owner",
			DelegationPrincipal:    principal,
			DelegationAgent:        agent,
			DelegationContentClass: contentClass,
		})
		require.NoError(t, err)
		return token
	}

	t.Run("signed happy path binds principal agent scope and expiry", func(t *testing.T) {
		claims, err := service.ValidateAccessToken(mint(time.Now().Add(time.Hour), "owner", "agent", DelegationContentClassNote))
		require.NoError(t, err)
		principal, present, err := ValidateDelegationAttestation(claims, DelegationContentClassNote)
		require.NoError(t, err)
		require.True(t, present)
		require.Equal(t, "@owner", principal)
	})

	t.Run("expired signed credential is rejected", func(t *testing.T) {
		_, err := service.ValidateAccessToken(mint(time.Now().Add(-time.Minute), "owner", "agent", DelegationContentClassNote))
		require.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("wrong scope fails closed", func(t *testing.T) {
		claims, err := service.ValidateAccessToken(mint(time.Now().Add(time.Hour), "owner", "agent", DelegationContentClassNote))
		require.NoError(t, err)
		_, present, err := ValidateDelegationAttestation(claims, DelegationContentClassDirectMessage)
		require.True(t, present)
		require.ErrorIs(t, err, ErrInvalidDelegationCredential)
	})

	t.Run("wrong principal fails closed", func(t *testing.T) {
		claims, err := service.ValidateAccessToken(mint(time.Now().Add(time.Hour), "mallory", "agent", DelegationContentClassNote))
		require.NoError(t, err)
		_, present, err := ValidateDelegationAttestation(claims, DelegationContentClassNote)
		require.True(t, present)
		require.ErrorIs(t, err, ErrInvalidDelegationCredential)
	})

	t.Run("credential minted for another agent fails closed", func(t *testing.T) {
		claims, err := service.ValidateAccessToken(mint(time.Now().Add(time.Hour), "owner", "agent-a", DelegationContentClassNote))
		require.NoError(t, err)
		claims.Username = "agent-b"
		_, present, err := ValidateDelegationAttestation(claims, DelegationContentClassNote)
		require.True(t, present)
		require.ErrorIs(t, err, ErrInvalidDelegationCredential)
	})

	t.Run("partial signed binding fails closed", func(t *testing.T) {
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			Username:         "agent", IsAgent: true, DelegatedBy: "@owner", DelegationPrincipal: "owner",
		}
		_, present, err := ValidateDelegationAttestation(claims, DelegationContentClassNote)
		require.True(t, present)
		require.ErrorIs(t, err, ErrInvalidDelegationCredential)
	})

	t.Run("legacy agent token has no scoped attestation", func(t *testing.T) {
		principal, present, err := ValidateDelegationAttestation(&Claims{Username: "agent", IsAgent: true, DelegatedBy: "@owner"}, DelegationContentClassNote)
		require.NoError(t, err)
		require.False(t, present)
		require.Empty(t, principal)
	})
}
