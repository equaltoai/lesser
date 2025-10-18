package auth

import (
	"github.com/equaltoai/lesser/pkg/common"
)

// ValidateToken validates a token and returns the claims
func (m *Middleware) ValidateToken(authHeader string) (*Claims, error) {
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		return nil, ErrMissingAuthHeader
	}

	// Extract bearer token
	token, err := ExtractBearerToken(authHeader)
	if err != nil {
		return nil, err
	}

	// Validate token
	claims, err := m.oauthService.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	return claims, nil
}

// This function has been moved to oauth.go to avoid duplication
