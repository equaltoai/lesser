package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuthService(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.storage)
}

func TestValidateClient(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)
	ctx := context.Background()

	client := &storage.OAuthClient{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		Name:         "Test Client",
		RedirectURIs: []string{"https://example.com/callback"},
	}

	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		setupMock    func()
		wantErr      error
	}{
		{
			name:         "valid client",
			clientID:     "test-client",
			clientSecret: "test-secret",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "test-client").Return(client, nil).Once()
			},
			wantErr: nil,
		},
		{
			name:         "client not found",
			clientID:     "wrong-client",
			clientSecret: "test-secret",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "wrong-client").Return(nil, errors.New("not found")).Once()
			},
			wantErr: ErrInvalidClient,
		},
		{
			name:         "invalid client secret",
			clientID:     "test-client",
			clientSecret: "wrong-secret",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "test-client").Return(client, nil).Once()
			},
			wantErr: ErrInvalidClient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			err := svc.ValidateClient(ctx, tt.clientID, tt.clientSecret)
			assert.Equal(t, tt.wantErr, err)
			mockStore.AssertExpectations(t)
		})
	}
}

func TestValidateRedirectURI(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)
	ctx := context.Background()

	client := &storage.OAuthClient{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		Name:         "Test Client",
		RedirectURIs: []string{"https://example.com/callback", "myapp://callback"},
	}

	tests := []struct {
		name        string
		clientID    string
		redirectURI string
		setupMock   func()
		wantErr     error
	}{
		{
			name:        "valid redirect URI",
			clientID:    "test-client",
			redirectURI: "https://example.com/callback",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "test-client").Return(client, nil).Once()
			},
			wantErr: nil,
		},
		{
			name:        "valid native app URI",
			clientID:    "test-client",
			redirectURI: "myapp://callback",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "test-client").Return(client, nil).Once()
			},
			wantErr: nil,
		},
		{
			name:        "native app URI with path",
			clientID:    "test-client",
			redirectURI: "myapp://callback/path",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "test-client").Return(client, nil).Once()
			},
			wantErr: nil,
		},
		{
			name:        "invalid client ID",
			clientID:    "wrong-client",
			redirectURI: "https://example.com/callback",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "wrong-client").Return(nil, errors.New("not found")).Once()
			},
			wantErr: ErrInvalidClient,
		},
		{
			name:        "invalid redirect URI",
			clientID:    "test-client",
			redirectURI: "https://wrong.com/callback",
			setupMock: func() {
				mockStore.On("GetOAuthClient", ctx, "test-client").Return(client, nil).Once()
			},
			wantErr: ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			err := svc.ValidateRedirectURI(ctx, tt.clientID, tt.redirectURI)
			assert.Equal(t, tt.wantErr, err)
			mockStore.AssertExpectations(t)
		})
	}
}

func TestGenerateAuthorizationCode(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)

	code1, err := svc.GenerateAuthorizationCode()
	require.NoError(t, err)
	assert.NotEmpty(t, code1)

	code2, err := svc.GenerateAuthorizationCode()
	require.NoError(t, err)
	assert.NotEmpty(t, code2)

	// Codes should be unique
	assert.NotEqual(t, code1, code2)
}

func TestVerifyCodeChallenge(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)

	// Test data from RFC 7636 example
	codeVerifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	codeChallengeS256 := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	tests := []struct {
		name            string
		codeChallenge   string
		codeVerifier    string
		challengeMethod string
		wantErr         error
	}{
		{
			name:            "valid S256 challenge",
			codeChallenge:   codeChallengeS256,
			codeVerifier:    codeVerifier,
			challengeMethod: "S256",
			wantErr:         nil,
		},
		{
			name:            "valid plain challenge",
			codeChallenge:   "test-verifier",
			codeVerifier:    "test-verifier",
			challengeMethod: "plain",
			wantErr:         nil,
		},
		{
			name:            "valid plain challenge (empty method)",
			codeChallenge:   "test-verifier",
			codeVerifier:    "test-verifier",
			challengeMethod: "",
			wantErr:         nil,
		},
		{
			name:            "invalid S256 challenge",
			codeChallenge:   "wrong-challenge",
			codeVerifier:    codeVerifier,
			challengeMethod: "S256",
			wantErr:         ErrInvalidCodeChallenge,
		},
		{
			name:            "invalid plain challenge",
			codeChallenge:   "wrong-challenge",
			codeVerifier:    "test-verifier",
			challengeMethod: "plain",
			wantErr:         ErrInvalidCodeChallenge,
		},
		{
			name:            "unsupported method",
			codeChallenge:   "test",
			codeVerifier:    "test",
			challengeMethod: "MD5",
			wantErr:         ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.VerifyCodeChallenge(tt.codeChallenge, tt.codeVerifier, tt.challengeMethod)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestGenerateTokens(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)

	username := "testuser"
	clientID := "test-client"
	scopes := []string{ScopeRead, ScopeWrite}

	accessToken, refreshToken, err := svc.GenerateTokens(username, clientID, scopes)
	require.NoError(t, err)
	assert.NotEmpty(t, accessToken)
	assert.NotEmpty(t, refreshToken)

	// Verify access token
	claims, err := svc.ValidateAccessToken(accessToken)
	require.NoError(t, err)
	assert.Equal(t, username, claims.Username)
	assert.Equal(t, clientID, claims.ClientID)
	assert.Equal(t, scopes, claims.Scopes)
}

func TestValidateAccessToken(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)

	// Generate a valid token
	validToken, _, err := svc.GenerateTokens("testuser", "test-client", []string{ScopeRead})
	require.NoError(t, err)

	// Create an expired token
	expiredClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "testuser",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
		Username: "testuser",
		ClientID: "test-client",
		Scopes:   []string{ScopeRead},
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredTokenString, err := expiredToken.SignedString(svc.jwtSecret)
	require.NoError(t, err)

	// Create a token with wrong signature
	wrongStore := new(mocks.MockStorage)
	wrongSvc := NewOAuthService("wrong-secret", wrongStore)
	wrongToken, _, err := wrongSvc.GenerateTokens("testuser", "test-client", []string{ScopeRead})
	require.NoError(t, err)

	tests := []struct {
		name      string
		token     string
		wantErr   bool
		wantClaim *Claims
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantErr: false,
			wantClaim: &Claims{
				Username: "testuser",
				ClientID: "test-client",
				Scopes:   []string{ScopeRead},
			},
		},
		{
			name:    "expired token",
			token:   expiredTokenString,
			wantErr: true,
		},
		{
			name:    "wrong signature",
			token:   wrongToken,
			wantErr: true,
		},
		{
			name:    "invalid token format",
			token:   "invalid.token.format",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := svc.ValidateAccessToken(tt.token)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrInvalidToken, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantClaim.Username, claims.Username)
				assert.Equal(t, tt.wantClaim.ClientID, claims.ClientID)
				assert.Equal(t, tt.wantClaim.Scopes, claims.Scopes)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantToken  string
		wantErr    error
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer eyJ0eXAiOiJKV1QiLCJhbGc",
			wantToken:  "eyJ0eXAiOiJKV1QiLCJhbGc",
			wantErr:    nil,
		},
		{
			name:       "missing bearer prefix",
			authHeader: "eyJ0eXAiOiJKV1QiLCJhbGc",
			wantToken:  "",
			wantErr:    ErrInvalidToken,
		},
		{
			name:       "wrong auth type",
			authHeader: "Basic dXNlcjpwYXNz",
			wantToken:  "",
			wantErr:    ErrInvalidToken,
		},
		{
			name:       "empty header",
			authHeader: "",
			wantToken:  "",
			wantErr:    ErrInvalidToken,
		},
		{
			name:       "only bearer word",
			authHeader: "Bearer",
			wantToken:  "",
			wantErr:    ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractBearerToken(tt.authHeader)
			assert.Equal(t, tt.wantToken, token)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestValidateScopes(t *testing.T) {
	tests := []struct {
		name    string
		scopes  []string
		wantErr error
	}{
		{
			name:    "valid scopes",
			scopes:  []string{ScopeRead, ScopeWrite},
			wantErr: nil,
		},
		{
			name:    "single valid scope",
			scopes:  []string{ScopeRead},
			wantErr: nil,
		},
		{
			name:    "empty scopes",
			scopes:  []string{},
			wantErr: nil,
		},
		{
			name:    "invalid scope",
			scopes:  []string{ScopeRead, "invalid"},
			wantErr: ErrInvalidScope,
		},
		{
			name:    "all invalid scopes",
			scopes:  []string{"invalid1", "invalid2"},
			wantErr: ErrInvalidScope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScopes(tt.scopes)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestHasScope(t *testing.T) {
	claims := &Claims{
		Scopes: []string{ScopeRead},
	}

	assert.True(t, claims.HasScope(ScopeRead))
	assert.False(t, claims.HasScope(ScopeWrite))
	assert.False(t, claims.HasScope("invalid"))

	// Test with multiple scopes
	claims.Scopes = []string{ScopeRead, ScopeWrite}
	assert.True(t, claims.HasScope(ScopeRead))
	assert.True(t, claims.HasScope(ScopeWrite))
}

func TestDefaultScopes(t *testing.T) {
	scopes := DefaultScopes()
	assert.Equal(t, []string{ScopeRead, ScopeWrite}, scopes)
}

func TestGenerateRefreshToken(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)

	token1, err := svc.generateRefreshToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token1)

	token2, err := svc.generateRefreshToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token2)

	// Tokens should be unique
	assert.NotEqual(t, token1, token2)
}

func TestTokenGeneration(t *testing.T) {
	mockStore := new(mocks.MockStorage)
	svc := NewOAuthService("test-secret", mockStore)

	// Test that generated tokens are properly formatted
	accessToken, refreshToken, err := svc.GenerateTokens("user", "client", []string{ScopeRead})
	require.NoError(t, err)

	// Access token should be a valid JWT
	parts := strings.Split(accessToken, ".")
	assert.Len(t, parts, 3, "JWT should have 3 parts")

	// Refresh token should be base64 encoded
	assert.NotEmpty(t, refreshToken)
	assert.NotContains(t, refreshToken, " ")
	assert.NotContains(t, refreshToken, "\n")
}
