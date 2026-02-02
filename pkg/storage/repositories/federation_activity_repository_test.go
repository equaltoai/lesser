package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// RecordFederationActivity Tests
// ============================================================================

func TestFederationActivityRepository_RecordFederationActivity_BeforeCreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	// Create an activity that will fail BeforeCreate validation
	// Looking at FederationActivity model - we need to check what causes BeforeCreate to fail
	// From the code, BeforeCreate calls UpdateKeys() which requires Domain
	activity := &models.FederationActivity{
		ID:           "test-id",
		Domain:       "", // Empty domain should cause key generation issues
		ActivityType: "Create",
		ActorID:      "https://example.com/users/alice",
	}

	// Execute - BeforeCreate will fail due to empty domain
	err := repo.RecordFederationActivity(ctx, activity)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation")
}

func TestFederationActivityRepository_RecordFederationActivity_ValidateAndCreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	testErr := errors.New("database error")

	activity := &models.FederationActivity{
		ID:           "test-activity-123",
		Domain:       "mastodon.social",
		ActivityType: "Create",
		ActorID:      "https://mastodon.social/users/alice",
	}

	// Set up mock for ValidateAndCreate to fail
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
	mockQuery.On("Create").Return(testErr)

	// Execute
	err := repo.RecordFederationActivity(ctx, activity)

	// Assert
	require.Error(t, err)
	// Error should be mapped with context
	assert.NotNil(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_RecordFederationActivity_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	activity := &models.FederationActivity{
		ID:           "test-activity-456",
		Domain:       "pleroma.site",
		ActivityType: "Follow",
		ActorID:      "https://pleroma.site/users/bob",
		Success:      true,
		ResponseTime: 150.5,
	}

	// Set up mock for ValidateAndCreate to succeed
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := repo.RecordFederationActivity(ctx, activity)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// UpdateInstanceInfo Tests
// ============================================================================

func TestFederationActivityRepository_UpdateInstanceInfo_UpdateSucceeds(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	info := &models.InstanceInfo{
		Domain:      "mastodon.social",
		Software:    "mastodon",
		Version:     "4.2.0",
		PublicKey:   "-----BEGIN PUBLIC KEY-----",
		SharedInbox: "https://mastodon.social/inbox",
		LastSeen:    time.Now(),
		FirstSeen:   time.Now().Add(-30 * 24 * time.Hour),
	}

	// Set up mock for Update to succeed
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(nil)

	// Execute
	err := repo.UpdateInstanceInfo(ctx, info)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_UpdateInstanceInfo_UpdateFails_CreateSucceeds(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockUpdateQuery := new(mocks.MockQuery)
	mockCreateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	updateErr := errors.New("item not found")

	info := &models.InstanceInfo{
		Domain:   "newinstance.social",
		Software: "pleroma",
		Version:  "2.5.0",
	}

	// Set up mock for Update to fail
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("Update", mock.Anything).Return(updateErr)

	// Set up mock for Create to succeed (fallback)
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil)

	// Execute
	err := repo.UpdateInstanceInfo(ctx, info)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
	mockCreateQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_UpdateInstanceInfo_UpdateFails_CreateFails(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockUpdateQuery := new(mocks.MockQuery)
	mockCreateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	updateErr := errors.New("item not found")
	createErr := errors.New("create failed")

	info := &models.InstanceInfo{
		Domain:   "failedinstance.social",
		Software: "misskey",
		Version:  "13.0",
	}

	// Set up mock for Update to fail
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("Update", mock.Anything).Return(updateErr)

	// Set up mock for Create to fail
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(createErr)

	// Execute
	err := repo.UpdateInstanceInfo(ctx, info)

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
	mockCreateQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_UpdateInstanceInfo_FirstSeenZero(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockUpdateQuery := new(mocks.MockQuery)
	mockCreateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	updateErr := errors.New("not found")

	// Create instance with zero FirstSeen - should be set on create
	info := &models.InstanceInfo{
		Domain:   "newdomain.social",
		Software: "mastodon",
		Version:  "4.2.0",
		// FirstSeen is zero value
	}

	// Set up mock for Update to fail (triggers create)
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("Update", mock.Anything).Return(updateErr)

	// Set up mock for Create
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil)

	// Execute
	err := repo.UpdateInstanceInfo(ctx, info)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// GetFederationActivity Tests
// ============================================================================

func TestFederationActivityRepository_GetFederationActivity(t *testing.T) {
	ctx := context.Background()

	t.Run("returns first activity when found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "fed_activity#example.com").Return(mockQuery)
		mockQuery.On("Where", "SK", "begins_with", "activity#").Return(mockQuery)
		mockQuery.On("Filter", "ID", "=", "act-1").Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]*models.FederationActivity)
			*target = []*models.FederationActivity{{ID: "act-1", Domain: "example.com"}}
		}).Return(nil)

		got, err := repo.GetFederationActivity(ctx, "example.com", "act-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "act-1", got.ID)
	})

	t.Run("returns not found error when empty result set", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "fed_activity#example.com").Return(mockQuery)
		mockQuery.On("Where", "SK", "begins_with", "activity#").Return(mockQuery)
		mockQuery.On("Filter", "ID", "=", "missing").Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]*models.FederationActivity)
			*target = []*models.FederationActivity{}
		}).Return(nil)

		_, err := repo.GetFederationActivity(ctx, "example.com", "missing")
		require.Error(t, err)
	})

	t.Run("returns mapped error on query failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "fed_activity#example.com").Return(mockQuery)
		mockQuery.On("Where", "SK", "begins_with", "activity#").Return(mockQuery)
		mockQuery.On("Filter", "ID", "=", "act-1").Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Return(errors.New("query failed"))

		_, err := repo.GetFederationActivity(ctx, "example.com", "act-1")
		require.Error(t, err)
	})
}

// ============================================================================
// ListByType / ListByActor Tests (GSI helper wiring)
// ============================================================================

func TestFederationActivityRepository_ListByType_And_ListByActor(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()

	_, err := repo.ListByType(ctx, "Create", start, end, 5)
	require.NoError(t, err)

	_, err = repo.ListByActor(ctx, "https://example.com/users/alice", start, end, 5)
	require.NoError(t, err)
}

// ============================================================================
// GetRecentActivities Tests
// ============================================================================

func TestFederationActivityRepository_GetRecentActivities(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
		mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
		mockQuery.On("Limit", 10).Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]*models.FederationActivity)
			*target = []*models.FederationActivity{{ID: "recent-1"}}
		}).Return(nil)

		items, err := repo.GetRecentActivities(ctx, time.Now().Add(-time.Hour), 10)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
		mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
		mockQuery.On("Limit", 10).Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Return(errors.New("query failed"))

		_, err := repo.GetRecentActivities(ctx, time.Now().Add(-time.Hour), 10)
		require.Error(t, err)
	})
}

// ============================================================================
// GetDomainStats Tests - Aggregation Math
// ============================================================================

func TestFederationActivityRepository_GetDomainStats_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	domain := "empty.social"
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()

	// Set up mock to return empty activities
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "fed_activity#empty.social").Return(mockQuery)
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10000).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Run(func(args mock.Arguments) {
		activities := args.Get(0).(*[]*models.FederationActivity)
		*activities = []*models.FederationActivity{}
	}).Return(nil)

	// Execute
	stats, err := repo.GetDomainStats(ctx, domain, startTime, endTime)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, domain, stats.Domain)
	assert.Equal(t, 0, stats.TotalCount)
	assert.Equal(t, 0, stats.SuccessCount)
	assert.Equal(t, 0, stats.ErrorCount)
	assert.Equal(t, 0.0, stats.AvgResponseTime)
	assert.Equal(t, 0, stats.UniqueActorCount)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_GetDomainStats_WithActivities(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	domain := "active.social"
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()

	// Set up mock to return activities with varied data
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "fed_activity#active.social").Return(mockQuery)
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10000).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Run(func(args mock.Arguments) {
		activities := args.Get(0).(*[]*models.FederationActivity)
		*activities = []*models.FederationActivity{
			{
				ID:           "act-1",
				Domain:       domain,
				ActivityType: "Create",
				ActorID:      "alice@active.social",
				InboundSize:  1000,
				OutboundSize: 500,
				Success:      true,
				ResponseTime: 100.0,
			},
			{
				ID:           "act-2",
				Domain:       domain,
				ActivityType: "Follow",
				ActorID:      "bob@active.social",
				InboundSize:  200,
				OutboundSize: 100,
				Success:      true,
				ResponseTime: 200.0,
			},
			{
				ID:           "act-3",
				Domain:       domain,
				ActivityType: "Create",              // Same type as act-1
				ActorID:      "alice@active.social", // Same actor as act-1
				InboundSize:  500,
				OutboundSize: 250,
				Success:      false, // Error
				ResponseTime: 0,     // Errors typically don't have response time counted
			},
		}
	}).Return(nil)

	// Execute
	stats, err := repo.GetDomainStats(ctx, domain, startTime, endTime)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, domain, stats.Domain)
	assert.Equal(t, 3, stats.TotalCount)
	assert.Equal(t, 2, stats.SuccessCount)
	assert.Equal(t, 1, stats.ErrorCount)

	// Inbound: 1000 + 200 + 500 = 1700
	assert.Equal(t, int64(1700), stats.InboundVolume)
	// Outbound: 500 + 100 + 250 = 850
	assert.Equal(t, int64(850), stats.OutboundVolume)

	// Average response time: (100 + 200) / 2 = 150 (only success counts)
	assert.Equal(t, 150.0, stats.AvgResponseTime)

	// Activity types: Create=2, Follow=1
	assert.Equal(t, 2, stats.ActivityTypes["Create"])
	assert.Equal(t, 1, stats.ActivityTypes["Follow"])

	// Unique actors: alice and bob = 2
	assert.Equal(t, 2, stats.UniqueActorCount)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_GetDomainStats_AllErrors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	domain := "error.social"
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()

	// Set up mock to return only failed activities
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "fed_activity#error.social").Return(mockQuery)
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10000).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Run(func(args mock.Arguments) {
		activities := args.Get(0).(*[]*models.FederationActivity)
		*activities = []*models.FederationActivity{
			{
				ID:           "fail-1",
				Domain:       domain,
				ActivityType: "Create",
				ActorID:      "user1",
				Success:      false,
				ResponseTime: 50.0,
			},
			{
				ID:           "fail-2",
				Domain:       domain,
				ActivityType: "Like",
				ActorID:      "user2",
				Success:      false,
				ResponseTime: 75.0,
			},
		}
	}).Return(nil)

	// Execute
	stats, err := repo.GetDomainStats(ctx, domain, startTime, endTime)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 2, stats.TotalCount)
	assert.Equal(t, 0, stats.SuccessCount)
	assert.Equal(t, 2, stats.ErrorCount)
	// Avg response time should be 0 since no success
	assert.Equal(t, 0.0, stats.AvgResponseTime)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_GetDomainStats_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	domain := "query-error.social"
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()
	testErr := errors.New("query failed")

	// Set up mock to return error
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationActivity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "fed_activity#query-error.social").Return(mockQuery)
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Where", "SK", "<=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10000).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationActivity")).Return(testErr)

	// Execute
	stats, err := repo.GetDomainStats(ctx, domain, startTime, endTime)

	// Assert
	require.Error(t, err)
	assert.Nil(t, stats)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// InstanceInfoItem Hook Tests
// ============================================================================

func TestInstanceInfoItem_TableName(t *testing.T) {
	item := InstanceInfoItem{}
	assert.Equal(t, models.MainTableName, item.TableName())
}

func TestInstanceInfoItem_BeforeCreate(t *testing.T) {
	t.Run("sets_timestamps_when_zero", func(t *testing.T) {
		item := &InstanceInfoItem{
			Domain: "test.social",
		}

		err := item.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, item.CreatedAt.IsZero())
		assert.False(t, item.UpdatedAt.IsZero())
	})

	t.Run("preserves_existing_createdAt", func(t *testing.T) {
		existingTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		item := &InstanceInfoItem{
			Domain:    "test.social",
			CreatedAt: existingTime,
		}

		err := item.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingTime, item.CreatedAt)
		assert.False(t, item.UpdatedAt.IsZero())
	})
}

func TestInstanceInfoItem_BeforeUpdate(t *testing.T) {
	item := &InstanceInfoItem{
		Domain:    "test.social",
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	err := item.BeforeUpdate()

	require.NoError(t, err)
	// UpdatedAt should be changed to current time
	assert.True(t, item.UpdatedAt.After(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))
}

// ============================================================================
// GetInstanceInfo Tests
// ============================================================================

func TestFederationActivityRepository_GetInstanceInfo_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	domain := "mastodon.social"
	lastSeen := time.Now()
	firstSeen := time.Now().Add(-30 * 24 * time.Hour)

	// Set up mock
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "instance#mastodon.social").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "info").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*repositories.InstanceInfoItem")).Run(func(args mock.Arguments) {
		item := args.Get(0).(*InstanceInfoItem)
		item.Domain = domain
		item.Software = "mastodon"
		item.Version = "4.2.0"
		item.PublicKey = "-----BEGIN PUBLIC KEY-----"
		item.SharedInbox = "https://mastodon.social/inbox"
		item.LastSeen = lastSeen
		item.FirstSeen = firstSeen
	}).Return(nil)

	// Execute
	result, err := repo.GetInstanceInfo(ctx, domain)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domain, result.Domain)
	assert.Equal(t, "mastodon", result.Software)
	assert.Equal(t, "4.2.0", result.Version)
	assert.Equal(t, "https://mastodon.social/inbox", result.SharedInbox)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationActivityRepository_GetInstanceInfo_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationActivityRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	domain := "nonexistent.social"
	testErr := errors.New("not found")

	// Set up mock
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "instance#nonexistent.social").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "info").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*repositories.InstanceInfoItem")).Return(testErr)

	// Execute
	result, err := repo.GetInstanceInfo(ctx, domain)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
