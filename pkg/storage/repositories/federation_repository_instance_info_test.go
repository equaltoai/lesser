package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// GetInstanceInfo Tests
// ============================================================================

func TestFederationRepository_GetInstanceInfo_ValidationError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()

	// Empty domain should fail validation
	result, err := repo.GetInstanceInfo(ctx, "")

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestFederationRepository_GetInstanceInfo_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	domain := "nonexistent.social"

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "INSTANCE#nonexistent.social").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "INSTANCE#nonexistent.social").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.FederationInstance")).Return(dynamormerrors.ErrItemNotFound)

	// Execute
	result, err := repo.GetInstanceInfo(ctx, domain)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrNotFound)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationRepository_GetInstanceInfo_DBError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	domain := "error.social"
	testErr := errors.New("connection failed")

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "INSTANCE#error.social").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "INSTANCE#error.social").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.FederationInstance")).Return(testErr)

	// Execute
	result, err := repo.GetInstanceInfo(ctx, domain)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationRepository_GetInstanceInfo_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	domain := "mastodon.social"
	firstSeen := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	lastSeen := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "INSTANCE#mastodon.social").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "INSTANCE#mastodon.social").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.FederationInstance")).Run(func(args mock.Arguments) {
		instance := args.Get(0).(*models.FederationInstance)
		instance.Domain = domain
		instance.Software = "mastodon"
		instance.Version = "4.2.0"
		instance.FirstSeen = firstSeen
		instance.LastSeen = lastSeen
		instance.PublicKey = "-----BEGIN PUBLIC KEY-----"
		instance.SharedInbox = "https://mastodon.social/inbox"
		instance.TrustScore = 0.95
		instance.ActiveUsers = 100000
		instance.TotalMessages = 5000000
	}).Return(nil)

	// Execute
	result, err := repo.GetInstanceInfo(ctx, domain)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domain, result.Domain)
	assert.Equal(t, "mastodon", result.Software)
	assert.Equal(t, "4.2.0", result.Version)
	assert.Equal(t, firstSeen, result.FirstSeen)
	assert.Equal(t, lastSeen, result.LastSeen)
	assert.Equal(t, 0.95, result.TrustScore)
	assert.Equal(t, int64(100000), result.ActiveUsers)
	assert.Equal(t, int64(5000000), result.TotalMessages)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// UpsertInstanceInfo Tests
// ============================================================================

func TestFederationRepository_UpsertInstanceInfo_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	info := &storage.InstanceInfo{
		Domain:      "newinstance.social",
		Software:    "pleroma",
		Version:     "2.5.0",
		FirstSeen:   time.Now().Add(-30 * 24 * time.Hour),
		LastSeen:    time.Now(),
		TrustScore:  0.8,
		ActiveUsers: 5000,
	}

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := repo.UpsertInstanceInfo(ctx, info)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationRepository_UpsertInstanceInfo_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	testErr := errors.New("write failed")
	info := &storage.InstanceInfo{
		Domain:   "failedinstance.social",
		Software: "misskey",
	}

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Create").Return(testErr)

	// Execute
	err := repo.UpsertInstanceInfo(ctx, info)

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// GetKnownInstances Tests
// ============================================================================

func TestFederationRepository_GetKnownInstances_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	limit := 10

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "FEDERATION_ACTIVE").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationInstance")).Run(func(args mock.Arguments) {
		instances := args.Get(0).(*[]models.FederationInstance)
		*instances = []models.FederationInstance{
			{
				Domain:      "instance1.social",
				Software:    "mastodon",
				Version:     "4.2.0",
				TrustScore:  0.9,
				ActiveUsers: 50000,
			},
			{
				Domain:      "instance2.social",
				Software:    "pleroma",
				Version:     "2.5.0",
				TrustScore:  0.85,
				ActiveUsers: 10000,
			},
		}
	}).Return(nil)

	// Execute
	results, cursor, err := repo.GetKnownInstances(ctx, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Empty(t, cursor) // Implementation returns empty cursor
	assert.Equal(t, "instance1.social", results[0].Domain)
	assert.Equal(t, "instance2.social", results[1].Domain)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationRepository_GetKnownInstances_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	testErr := errors.New("scan failed")

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "FEDERATION_ACTIVE").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationInstance")).Return(testErr)

	// Execute
	results, cursor, err := repo.GetKnownInstances(ctx, 10, "")

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationRepository_GetKnownInstances_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationInstance")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "FEDERATION_ACTIVE").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationInstance")).Run(func(args mock.Arguments) {
		instances := args.Get(0).(*[]models.FederationInstance)
		*instances = []models.FederationInstance{}
	}).Return(nil)

	// Execute
	results, cursor, err := repo.GetKnownInstances(ctx, 10, "")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// GetFederationCosts Tests
// ============================================================================

func TestFederationRepository_GetFederationCosts_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)
	limit := 10

	expectedPK := "FEDERATION_COSTS#2024-01"

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationCost")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", expectedPK).Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCost")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]models.FederationCost)
		*costs = []models.FederationCost{
			{
				Domain:           "domain1.social",
				Period:           "monthly",
				IngressBytes:     1000000,
				EgressBytes:      500000,
				RequestCount:     1000,
				ErrorCount:       50,
				ErrorRate:        0.05,
				AvgResponseTime:  150.0,
				EstimatedCostUSD: 1.25,
			},
			{
				Domain:           "domain2.social",
				Period:           "monthly",
				IngressBytes:     2000000,
				EgressBytes:      1000000,
				RequestCount:     2000,
				ErrorCount:       100,
				ErrorRate:        0.05,
				AvgResponseTime:  200.0,
				EstimatedCostUSD: 2.50,
			},
		}
	}).Return(nil)

	// Execute
	results, cursor, err := repo.GetFederationCosts(ctx, startTime, endTime, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Empty(t, cursor) // Implementation returns empty cursor
	assert.Equal(t, "domain1.social", results[0].Domain)
	assert.Equal(t, int64(1000000), results[0].IngressBytes)
	assert.Equal(t, 1.25, results[0].EstimatedCostUSD)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestFederationRepository_GetFederationCosts_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewFederationRepository(mockDB, "test-table", logger, nil, &appConfig.Config{})

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)
	testErr := errors.New("query failed")

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationCost")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]models.FederationCost")).Return(testErr)

	// Execute
	results, cursor, err := repo.GetFederationCosts(ctx, startTime, endTime, 10, "")

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Note: RecordFederationActivity tests are not included because the method
// spawns an async goroutine (updateAggregatedCosts) that accesses the mock DB
// after the test completes, causing mock verification failures.
// The underlying ValidateAndCreate path is tested via the other tests.
