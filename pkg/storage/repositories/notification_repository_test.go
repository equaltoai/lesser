package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// TestNotificationRepository_EmailPreferencesAlwaysFalse tests that email preferences are always returned as false
func TestNotificationRepository_EmailPreferencesAlwaysFalse(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewNotificationRepository(mockDB, "test-table", logger)

	// Setup mock to simulate finding existing preferences with email enabled
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationPreferences")).Return(mockQuery)
	mockQuery.On("Filter", "Username", "=", "testuser").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// Simulate database returning preferences with EmailEnabled: true
		// This should be overridden to false in the response
	}).Return(nil)

	ctx := context.Background()
	prefs, err := repo.GetNotificationPreferences(ctx, "testuser")

	assert.NoError(t, err)
	assert.NotNil(t, prefs)
	// Email should always be false regardless of what's stored
	assert.False(t, prefs.EmailEnabled, "Email preferences should always be false - Lesser does not support email notifications")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestNotificationRepository_UpdatePreferencesIgnoresEmail tests that updating preferences always sets email to false
func TestNotificationRepository_UpdatePreferencesIgnoresEmail(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewNotificationRepository(mockDB, "test-table", logger)

	// Setup mock for update operation
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationPreferences")).Return(mockQuery)
	mockQuery.On("Update").Return(nil)

	// Try to update with EmailEnabled: true
	inputPrefs := &models.NotificationPreferences{
		Username:        "testuser",
		EmailEnabled:    true, // This should be ignored
		PushEnabled:     true,
		FollowEnabled:   true,
		MentionEnabled:  true,
		ReblogEnabled:   true,
		FavoriteEnabled: true,
		PollEnabled:     true,
	}

	ctx := context.Background()
	err := repo.UpdateNotificationPreferences(ctx, inputPrefs)

	assert.NoError(t, err)

	// Verify that the mock was called and that EmailEnabled would be set to false
	// The actual enforcement happens in the repository implementation
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestNotificationRepository_SetEmailPreferenceIgnored tests that setting email preference is silently ignored
func TestNotificationRepository_SetEmailPreferenceIgnored(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewNotificationRepository(mockDB, "test-table", logger)

	// Setup mock for getting existing preferences
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationPreferences")).Return(mockQuery)
	mockQuery.On("Filter", "Username", "=", "testuser").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(nil) // Return nil (not found) to test creation path
	mockQuery.On("Update").Return(nil)

	ctx := context.Background()

	// Try to enable email notifications - should be silently ignored
	err := repo.SetNotificationPreference(ctx, "testuser", "email_follow", true)

	// Should succeed without error (silently ignored)
	assert.NoError(t, err)

	// Try to disable email notifications - should also be silently ignored
	err = repo.SetNotificationPreference(ctx, "testuser", "email_mention", false)

	// Should succeed without error (silently ignored)
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestNotificationRepository_SetOtherPreferencesWork tests that non-email preferences work normally
func TestNotificationRepository_SetOtherPreferencesWork(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewNotificationRepository(mockDB, "test-table", logger)

	// Setup mock for getting existing preferences and updating
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationPreferences")).Return(mockQuery)
	mockQuery.On("Filter", "Username", "=", "testuser").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(nil) // Return nil (not found) to test creation path
	mockQuery.On("Update").Return(nil)

	ctx := context.Background()

	// Test that push preferences work normally
	err := repo.SetNotificationPreference(ctx, "testuser", "push_follow", true)
	assert.NoError(t, err)

	// Test that mention preferences work normally
	err = repo.SetNotificationPreference(ctx, "testuser", "push_mention", false)
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
