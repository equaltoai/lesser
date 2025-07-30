package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestStoreReputation(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	
	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()
	
	reputation := &storage.Reputation{
		ActorID:         "https://example.com/users/alice",
		InstanceURL:     "https://example.com",
		TrustScore:      850,
		ActivityScore:   750,
		ModerationScore: 900,
		CommunityScore:  800,
		TotalScore:      825,
		CalculatedAt:    time.Now(),
		Version:         "1.0",
		TotalPosts:      100,
		TotalFollowers:  50,
		AccountAge:      365,
		VouchCount:      5,
	}
	
	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	
	// Test
	err := repo.StoreReputation(ctx, reputation.ActorID, reputation)
	
	// Assert
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetReputation(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	
	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()
	
	// Test data
	calculatedAt := time.Now()
	reputationData := `{
		"@id": "https://example.com/users/alice",
		"instance": "https://example.com",
		"trustScore": 850,
		"activityScore": 750,
		"moderationScore": 900,
		"communityScore": 800,
		"totalScore": 825,
		"calculatedAt": "` + calculatedAt.Format(time.RFC3339) + `",
		"version": "1.0",
		"totalPosts": 100,
		"totalFollowers": 50,
		"accountAgeDays": 365,
		"vouchCount": 5
	}`
	
	mockRep := models.Reputation{
		PK:             "ACTOR#alice",
		SK:             "REP#" + calculatedAt.Format(time.RFC3339),
		ReputationData: reputationData,
		TotalScore:     825,
		CalculatedAt:   calculatedAt.Format(time.RFC3339),
	}
	
	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", &models.Reputation{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{mockRep}
	}).Return(nil)
	
	// Test
	reputation, err := repo.GetReputation(ctx, "https://example.com/users/alice")
	
	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, reputation)
	assert.Equal(t, 825, reputation.TotalScore)
	assert.Equal(t, "https://example.com/users/alice", reputation.ActorID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetReputationHistory(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	
	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()
	
	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", &models.Reputation{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{} // Empty history
	}).Return(nil)
	
	// Test
	history, err := repo.GetReputationHistory(ctx, "https://example.com/users/alice", 10)
	
	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, history)
	assert.Empty(t, history)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetUserTrustScore(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	
	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()
	
	// Test data
	calculatedAt := time.Now()
	reputationData := `{"totalScore": 825, "calculatedAt": "` + calculatedAt.Format(time.RFC3339) + `"}`
	
	mockRep := models.Reputation{
		ReputationData: reputationData,
		TotalScore:     825,
	}
	
	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", &models.Reputation{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{mockRep}
	}).Return(nil)
	
	// Test
	score, err := repo.GetUserTrustScore(ctx, "https://example.com/users/alice")
	
	// Assert
	assert.NoError(t, err)
	assert.Equal(t, float64(825), score)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetUserTrustScoreNoReputation(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	
	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()
	
	// Set up expectations - no reputation found
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", &models.Reputation{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{} // Empty
	}).Return(nil)
	
	// Test
	score, err := repo.GetUserTrustScore(ctx, "https://example.com/users/alice")
	
	// Assert
	assert.NoError(t, err)
	assert.Equal(t, float64(0), score)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}