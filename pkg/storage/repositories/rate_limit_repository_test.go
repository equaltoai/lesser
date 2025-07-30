package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRateLimitRepository_NewLoginAttempt(t *testing.T) {
	// Test the LoginAttempt model creation
	identifier := "test-user"
	success := true
	
	attempt := models.NewLoginAttempt(identifier, success)
	
	assert.Equal(t, "RATELIMIT#test-user", attempt.PK)
	assert.Equal(t, "LoginAttempt", attempt.Type)
	assert.Equal(t, success, attempt.Success)
	assert.False(t, attempt.Timestamp.IsZero())
	assert.Greater(t, attempt.TTL, time.Now().Unix())
}

func TestRateLimitRepository_NewRateLimitLockout(t *testing.T) {
	// Test the RateLimitLockout model creation
	identifier := "test-user"
	unlockTime := time.Now().Add(1 * time.Hour)
	
	lockout := models.NewRateLimitLockout(identifier, unlockTime)
	
	assert.Equal(t, "RATELIMIT#test-user", lockout.PK)
	assert.Equal(t, "LOCKOUT", lockout.SK)
	assert.Equal(t, "RateLimitLockout", lockout.Type)
	assert.Equal(t, unlockTime, lockout.UnlockTime)
	assert.Equal(t, unlockTime.Unix(), lockout.TTL)
}

func TestRateLimitRepository_CheckCommunityNoteRateLimit_NoClient(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	
	// Create repository without DynamoDB client
	repo := NewRateLimitRepository(mockDB, "test-table", logger, nil)
	
	// Should fall back to allowing creation when no client
	canCreate, remaining, err := repo.CheckCommunityNoteRateLimit(context.Background(), "test-user", 10)
	
	assert.NoError(t, err)
	assert.True(t, canCreate)
	assert.Equal(t, 10, remaining)
}

func TestRateLimitRepository_ModelUpdateKeys(t *testing.T) {
	// Test that UpdateKeys methods work correctly
	attempt := &models.LoginAttempt{}
	attempt.UpdateKeys()
	assert.Equal(t, "LoginAttempt", attempt.Type)
	
	lockout := &models.RateLimitLockout{}
	lockout.UpdateKeys()
	assert.Equal(t, "RateLimitLockout", lockout.Type)
}