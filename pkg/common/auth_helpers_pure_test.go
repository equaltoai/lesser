package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockClaims implements the Claims interface for testing
type mockClaims struct {
	scopes   map[string]bool
	username string
}

func (m mockClaims) HasScope(scope string) bool {
	return m.scopes[scope]
}

func (m mockClaims) GetUsername() string {
	return m.username
}

func newMockClaims(username string, scopes ...string) mockClaims {
	scopeMap := make(map[string]bool)
	for _, s := range scopes {
		scopeMap[s] = true
	}
	return mockClaims{scopes: scopeMap, username: username}
}

func TestHasAnyScope(t *testing.T) {
	tests := []struct {
		name     string
		claims   Claims
		scopes   []string
		expected bool
	}{
		{
			name:     "nil claims returns false",
			claims:   nil,
			scopes:   []string{ScopeRead},
			expected: false,
		},
		{
			name:     "empty scopes returns false",
			claims:   newMockClaims("user", ScopeRead),
			scopes:   []string{},
			expected: false,
		},
		{
			name:     "has matching scope",
			claims:   newMockClaims("user", ScopeRead, ScopeWrite),
			scopes:   []string{ScopeRead},
			expected: true,
		},
		{
			name:     "has one of multiple scopes",
			claims:   newMockClaims("user", ScopeWrite),
			scopes:   []string{ScopeRead, ScopeWrite},
			expected: true,
		},
		{
			name:     "no matching scopes",
			claims:   newMockClaims("user", ScopeRead),
			scopes:   []string{ScopeWrite, AdminWrite},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasAnyScope(tt.claims, tt.scopes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasAllScopes(t *testing.T) {
	tests := []struct {
		name     string
		claims   Claims
		scopes   []string
		expected bool
	}{
		{
			name:     "nil claims returns false",
			claims:   nil,
			scopes:   []string{ScopeRead},
			expected: false,
		},
		{
			name:     "empty scopes returns true",
			claims:   newMockClaims("user", ScopeRead),
			scopes:   []string{},
			expected: true,
		},
		{
			name:     "has all scopes",
			claims:   newMockClaims("user", ScopeRead, ScopeWrite),
			scopes:   []string{ScopeRead, ScopeWrite},
			expected: true,
		},
		{
			name:     "missing one scope",
			claims:   newMockClaims("user", ScopeRead),
			scopes:   []string{ScopeRead, ScopeWrite},
			expected: false,
		},
		{
			name:     "has superset of required scopes",
			claims:   newMockClaims("user", ScopeRead, ScopeWrite, AdminRead),
			scopes:   []string{ScopeRead},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasAllScopes(tt.claims, tt.scopes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateReadScopes(t *testing.T) {
	t.Run("has read scope", func(t *testing.T) {
		claims := newMockClaims("user", ScopeRead)
		assert.True(t, ValidateReadScopes(claims))
	})

	t.Run("has read:accounts scope", func(t *testing.T) {
		claims := newMockClaims("user", "read:accounts")
		assert.True(t, ValidateReadScopes(claims))
	})

	t.Run("no read scopes", func(t *testing.T) {
		claims := newMockClaims("user", ScopeWrite)
		assert.False(t, ValidateReadScopes(claims))
	})
}

func TestValidateWriteScopes(t *testing.T) {
	t.Run("has write scope", func(t *testing.T) {
		claims := newMockClaims("user", ScopeWrite)
		assert.True(t, ValidateWriteScopes(claims))
	})

	t.Run("has write:statuses scope", func(t *testing.T) {
		claims := newMockClaims("user", "write:statuses")
		assert.True(t, ValidateWriteScopes(claims))
	})

	t.Run("no write scopes", func(t *testing.T) {
		claims := newMockClaims("user", ScopeRead)
		assert.False(t, ValidateWriteScopes(claims))
	})
}

func TestValidateFollowScopes(t *testing.T) {
	t.Run("has write:follows scope", func(t *testing.T) {
		claims := newMockClaims("user", WriteFollows)
		assert.True(t, ValidateFollowScopes(claims))
	})

	t.Run("has write scope", func(t *testing.T) {
		claims := newMockClaims("user", ScopeWrite)
		assert.True(t, ValidateFollowScopes(claims))
	})

	t.Run("no follow scopes", func(t *testing.T) {
		claims := newMockClaims("user", ScopeRead)
		assert.False(t, ValidateFollowScopes(claims))
	})
}

func TestValidateBlockScopes(t *testing.T) {
	t.Run("has write scope", func(t *testing.T) {
		claims := newMockClaims("user", ScopeWrite)
		assert.True(t, ValidateBlockScopes(claims))
	})

	t.Run("has write:blocks scope", func(t *testing.T) {
		claims := newMockClaims("user", "write:blocks")
		assert.True(t, ValidateBlockScopes(claims))
	})

	t.Run("no block scopes", func(t *testing.T) {
		claims := newMockClaims("user", ScopeRead)
		assert.False(t, ValidateBlockScopes(claims))
	})
}

func TestValidateAdminScopes(t *testing.T) {
	t.Run("has admin:read scope", func(t *testing.T) {
		claims := newMockClaims("admin", AdminRead)
		assert.True(t, ValidateAdminScopes(claims))
	})

	t.Run("has admin:write scope", func(t *testing.T) {
		claims := newMockClaims("admin", AdminWrite)
		assert.True(t, ValidateAdminScopes(claims))
	})

	t.Run("no admin scopes", func(t *testing.T) {
		claims := newMockClaims("user", ScopeRead, ScopeWrite)
		assert.False(t, ValidateAdminScopes(claims))
	})
}

func TestAuthContext(t *testing.T) {
	claims := newMockClaims("testuser", ScopeRead)
	ctx := AuthContext{
		Username: "testuser",
		Claims:   claims,
	}

	assert.Equal(t, "testuser", ctx.Username)
	assert.NotNil(t, ctx.Claims)
	assert.True(t, ctx.Claims.HasScope(ScopeRead))
}

func TestAuthenticationResult(t *testing.T) {
	t.Run("successful result", func(t *testing.T) {
		claims := newMockClaims("testuser", ScopeRead)
		result := AuthenticationResult{
			Context: &AuthContext{
				Username: "testuser",
				Claims:   claims,
			},
			Error:     nil,
			ErrorCode: 0,
		}

		assert.NotNil(t, result.Context)
		assert.NoError(t, result.Error)
		assert.Equal(t, 0, result.ErrorCode)
	})

	t.Run("error result", func(t *testing.T) {
		result := AuthenticationResult{
			Context:   nil,
			Error:     assert.AnError,
			ErrorCode: 401,
		}

		assert.Nil(t, result.Context)
		assert.Error(t, result.Error)
		assert.Equal(t, 401, result.ErrorCode)
	})
}
