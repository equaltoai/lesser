package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
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

func TestRateLimitRepository_CheckCommunityNoteRateLimit_WithinLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	// Create repository with DynamORM
	repo := NewRateLimitRepository(mockDB, "test-table", logger, nil)

	// Mock the query to return 5 existing notes (under limit of 10)
	notes := []*models.CommunityNote{
		{ID: "note1", AuthorID: "test-user"},
		{ID: "note2", AuthorID: "test-user"},
		{ID: "note3", AuthorID: "test-user"},
		{ID: "note4", AuthorID: "test-user"},
		{ID: "note5", AuthorID: "test-user"},
	}

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CommunityNote")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "AUTHOR#test-user#NOTES").Return(mockQuery)
	mockQuery.On("Where", "gsi3SK", ">", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.CommunityNote")).Run(func(args mock.Arguments) {
		result := args.Get(0).(*[]*models.CommunityNote)
		*result = notes
	}).Return(nil)

	canCreate, remaining, err := repo.CheckCommunityNoteRateLimit(context.Background(), "test-user", 10)

	assert.NoError(t, err)
	assert.True(t, canCreate)
	assert.Equal(t, 5, remaining)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
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
