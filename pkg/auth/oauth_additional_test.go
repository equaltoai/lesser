package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type oauthAccountRepoStub struct {
	clients map[string]*storage.OAuthClient
	err     error
}

func (s *oauthAccountRepoStub) GetOAuthClient(_ context.Context, clientID string) (*storage.OAuthClient, error) {
	if s.err != nil {
		return nil, s.err
	}
	if c, ok := s.clients[clientID]; ok {
		return c, nil
	}
	return nil, errNotFound
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

var errNotFound = errSentinel("not found")

func TestOAuthService_ValidateClientRedirectURIAndScopes(t *testing.T) {
	t.Parallel()

	repo := &oauthAccountRepoStub{
		clients: map[string]*storage.OAuthClient{
			"client-1": {
				ClientID:     "client-1",
				ClientSecret: "secret",
				RedirectURIs: []string{"https://example.com/callback", "urn:ietf:wg:oauth:2.0:oob"},
				Scopes:       []string{ScopeRead, ScopeWrite},
			},
			"client-empty-scopes": {
				ClientID:     "client-empty-scopes",
				ClientSecret: "secret",
				RedirectURIs: []string{"https://example.com/callback"},
				Scopes:       nil,
			},
		},
	}

	svc := &OAuthService{
		jwtSecret:   []byte("test-secret"),
		accountRepo: repo,
	}

	require.Equal(t, ErrInvalidRequest, svc.ValidateClient(context.Background(), "", "x"))
	require.Equal(t, ErrInvalidClient, svc.ValidateClient(context.Background(), "missing", "x"))
	require.Equal(t, ErrInvalidClient, svc.ValidateClient(context.Background(), "client-1", ""))
	require.Equal(t, ErrInvalidClient, svc.ValidateClient(context.Background(), "client-1", "wrong"))
	require.NoError(t, svc.ValidateClient(context.Background(), "client-1", "secret"))

	require.Equal(t, ErrInvalidRequest, svc.ValidateRedirectURI(context.Background(), "", "https://example.com/callback"))
	require.Equal(t, ErrInvalidClient, svc.ValidateRedirectURI(context.Background(), "missing", "https://example.com/callback"))
	require.NoError(t, svc.ValidateRedirectURI(context.Background(), "client-1", "https://example.com/callback"))
	require.NoError(t, svc.ValidateRedirectURI(context.Background(), "client-1", "urn:ietf:wg:oauth:2.0:oob"))

	// OOB URI must be explicitly registered.
	repo.clients["client-no-oob"] = &storage.OAuthClient{
		ClientID:     "client-no-oob",
		ClientSecret: "secret",
		RedirectURIs: []string{"https://example.com/callback"},
		Scopes:       []string{ScopeRead},
	}
	require.Equal(t, ErrInvalidRequest, svc.ValidateRedirectURI(context.Background(), "client-no-oob", "urn:ietf:wg:oauth:2.0:oob"))
	require.Equal(t, ErrInvalidRequest, svc.ValidateRedirectURI(context.Background(), "client-no-oob", "https://example.com/other"))

	// ValidateScopes uses client scopes; empty request defaults to read.
	require.NoError(t, svc.ValidateScopes(context.Background(), "client-1", nil))
	require.NoError(t, svc.ValidateScopes(context.Background(), "client-1", []string{ScopeWrite}))
	require.Equal(t, ErrInvalidScope, svc.ValidateScopes(context.Background(), "client-1", []string{"admin"}))

	// If client has no registered scopes, allow default Mastodon scopes (e.g. push).
	require.NoError(t, svc.ValidateScopes(context.Background(), "client-empty-scopes", []string{"push"}))

	// Missing account repo is treated as invalid client.
	svcNil := &OAuthService{jwtSecret: []byte("test-secret")}
	require.Equal(t, ErrInvalidClient, svcNil.ValidateClient(context.Background(), "client-1", "secret"))
	require.Equal(t, ErrInvalidClient, svcNil.ValidateRedirectURI(context.Background(), "client-1", "https://example.com/callback"))
	require.Equal(t, ErrInvalidClient, svcNil.ValidateScopes(context.Background(), "client-1", []string{ScopeRead}))
}

func TestOAuthService_GenerateTokensWithContext_AndEnhancedValidation(t *testing.T) {
	t.Parallel()

	svc := &OAuthService{jwtSecret: []byte("test-secret")}

	access, _, err := svc.GenerateTokensWithContext(
		"alice",
		"client-1",
		"sid-1",
		"did-1",
		"192.0.2.10",
		"ua",
		[]string{ScopeRead},
		3,
	)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessTokenWithContext(access, "sid-1", "192.0.2.10", 3)
	require.NoError(t, err)
	require.Equal(t, "alice", claims.Username)
	require.Equal(t, "client-1", claims.ClientID)
	require.Equal(t, "sid-1", claims.SessionID)
	require.Equal(t, "did-1", claims.DeviceID)
	require.Equal(t, 3, claims.TokenVersion)
	require.Equal(t, "192.0.2.10", claims.IPAddress)
	require.Equal(t, "ua", claims.UserAgent)

	_, err = svc.ValidateAccessTokenWithContext(access, "sid-wrong", "192.0.2.10", 3)
	require.ErrorIs(t, err, ErrSessionIDMismatch)
	_, err = svc.ValidateAccessTokenWithContext(access, "sid-1", "203.0.113.5", 3)
	require.ErrorIs(t, err, ErrIPAddressMismatch)
	_, err = svc.ValidateAccessTokenWithContext(access, "sid-1", "192.0.2.10", 9)
	require.ErrorIs(t, err, ErrTokenVersionMismatch)

	// Unexpired tokens are validated by explicit expiry and context, not a hidden age wall.
	old := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-48 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-48 * time.Hour)),
			ID:        "jti",
		},
		Username: "alice",
		ClientID: "client-1",
		Scopes:   []string{ScopeRead},
	}
	oldToken := jwt.NewWithClaims(jwt.SigningMethodHS256, old)
	oldTokenString, err := oldToken.SignedString(svc.jwtSecret)
	require.NoError(t, err)
	oldClaims, err := svc.ValidateAccessTokenWithContext(oldTokenString, "", "", 0)
	require.NoError(t, err)
	require.Equal(t, "alice", oldClaims.Username)

	expired := old
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Minute))
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expired)
	expiredTokenString, err := expiredToken.SignedString(svc.jwtSecret)
	require.NoError(t, err)
	_, err = svc.ValidateAccessTokenWithContext(expiredTokenString, "", "", 0)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Unexpected signing method returns ErrInvalidToken.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsToken := jwt.NewWithClaims(jwt.SigningMethodRS256, old)
	rsTokenString, err := rsToken.SignedString(rsaKey)
	require.NoError(t, err)
	_, err = svc.ValidateAccessTokenWithContext(rsTokenString, "", "", 0)
	require.ErrorIs(t, err, ErrInvalidToken)
}
