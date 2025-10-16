package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSession_TableName(t *testing.T) {
	s := &Session{}
	assert.Equal(t, "lesser-main", s.TableName())
}

func TestSession_BeforeUpdate(t *testing.T) {
	session := &Session{
		SessionID: "session123",
		UserID:    "user123",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	// Set up initial state
	err := session.BeforeCreate()
	assert.NoError(t, err)

	originalUpdatedAt := session.UpdatedAt

	// Wait a bit and call BeforeUpdate
	time.Sleep(10 * time.Millisecond)
	err = session.BeforeUpdate()
	assert.NoError(t, err)

	// Check that UpdatedAt was updated
	assert.True(t, session.UpdatedAt.After(originalUpdatedAt))
}

func TestSession_Touch(t *testing.T) {
	// Test with plenty of time remaining (should not extend)
	session := &Session{
		LastUsedAt: time.Now().Add(-1 * time.Hour),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(), // 24 hours remaining, should not extend
	}

	originalLastUsed := session.LastUsedAt
	originalExpiresAt := session.ExpiresAt

	session.Touch()

	// Check that LastUsedAt was updated
	assert.True(t, session.LastUsedAt.After(originalLastUsed))

	// Check that expiry was not extended (more than 12 hours remaining)
	assert.Equal(t, originalExpiresAt, session.ExpiresAt)

	// Test with expiry soon (less than 12 hours)
	session.ExpiresAt = time.Now().Add(6 * time.Hour).Unix()
	originalExpiresAt = session.ExpiresAt

	session.Touch()

	// Check that expiry was extended
	assert.Greater(t, session.ExpiresAt, originalExpiresAt)
}

func TestSession_Revoke(t *testing.T) {
	session := &Session{}
	assert.False(t, session.IsRevoked)
	assert.Nil(t, session.RevokedAt)
	assert.Empty(t, session.RevokeReason)

	reason := "user logout"
	session.Revoke(reason)

	assert.True(t, session.IsRevoked)
	assert.NotNil(t, session.RevokedAt)
	assert.Equal(t, reason, session.RevokeReason)
	assert.True(t, time.Since(*session.RevokedAt) < time.Second)
}

func TestSession_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		session  *Session
		expected bool
	}{
		{
			name: "valid session",
			session: &Session{
				IsRevoked: false,
				ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
			},
			expected: true,
		},
		{
			name: "revoked session",
			session: &Session{
				IsRevoked: true,
				ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
			},
			expected: false,
		},
		{
			name: "expired session",
			session: &Session{
				IsRevoked: false,
				ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
			},
			expected: false,
		},
		{
			name: "revoked and expired session",
			session: &Session{
				IsRevoked: true,
				ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.session.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		session  *Session
		expected bool
	}{
		{
			name: "not expired",
			session: &Session{
				ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
			},
			expected: false,
		},
		{
			name: "expired",
			session: &Session{
				ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.session.IsExpired()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSession_HasScope(t *testing.T) {
	session := &Session{
		Scopes: []string{"read", "write", "admin"},
	}

	assert.True(t, session.HasScope("read"))
	assert.True(t, session.HasScope("write"))
	assert.True(t, session.HasScope("admin"))
	assert.False(t, session.HasScope("delete"))
	assert.False(t, session.HasScope(""))

	// Test empty scopes
	emptySession := &Session{}
	assert.False(t, emptySession.HasScope("read"))
}

func TestSession_ValidateRequest(t *testing.T) {
	session := &Session{}

	// For now, this always returns true
	// In the future, it might validate IP/UserAgent
	result := session.ValidateRequest("192.168.1.1", "Mozilla/5.0")
	assert.True(t, result)
}

func TestSession_Context(t *testing.T) {
	session := &Session{}

	// Test getting non-existent key
	value, exists := session.GetContext("key1")
	assert.Nil(t, value)
	assert.False(t, exists)

	// Test setting and getting
	session.SetContext("key1", "value1")
	session.SetContext("key2", 42)

	value, exists = session.GetContext("key1")
	assert.Equal(t, "value1", value)
	assert.True(t, exists)

	value, exists = session.GetContext("key2")
	assert.Equal(t, 42, value)
	assert.True(t, exists)

	// Test non-existent key
	value, exists = session.GetContext("key3")
	assert.Nil(t, value)
	assert.False(t, exists)
}

func TestSession_RemainingTime(t *testing.T) {
	// Test with 1 hour remaining
	session := &Session{
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	}

	remaining := session.RemainingTime()
	assert.True(t, remaining > 59*time.Minute)
	assert.True(t, remaining <= 60*time.Minute)

	// Test with expired session
	expiredSession := &Session{
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
	}

	remaining = expiredSession.RemainingTime()
	assert.True(t, remaining < 0)
}

func TestGenerateSecureToken(t *testing.T) {
	// Test token generation
	token1, err := generateSecureToken(32)
	assert.NoError(t, err)
	assert.NotEmpty(t, token1)

	token2, err := generateSecureToken(32)
	assert.NoError(t, err)
	assert.NotEmpty(t, token2)

	// Tokens should be different
	assert.NotEqual(t, token1, token2)

	// Test different lengths
	shortToken, err := generateSecureToken(16)
	assert.NoError(t, err)
	assert.NotEmpty(t, shortToken)

	longToken, err := generateSecureToken(64)
	assert.NoError(t, err)
	assert.NotEmpty(t, longToken)

	// Different lengths should produce different length tokens
	assert.NotEqual(t, len(shortToken), len(longToken))
}
