package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func matchAliceReputationPK(pk string) bool {
	return pk == "ACTOR#https://example.com/users/alice" || pk == "ACTOR#alice"
}

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
		Version:         1,
		TotalPosts:      100,
		TotalFollowers:  50,
		AccountAge:      365,
		VouchCount:      5,
	}

	// Set up expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Once()

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
	reputationData := fmt.Sprintf(`{
		"actor_id": "https://example.com/users/alice",
		"instance_url": "https://example.com",
		"trust_score": 850,
		"activity_score": 750,
		"moderation_score": 900,
		"community_score": 800,
		"total_score": 825,
		"calculated_at": "%s",
		"version": 1,
		"total_posts": 100,
		"total_followers": 50,
		"account_age": 365,
		"vouch_count": 5
	}`, calculatedAt.Format(time.RFC3339))

	mockRep := models.Reputation{
		PK:             "ACTOR#alice",
		SK:             "REP#" + calculatedAt.Format(time.RFC3339),
		ReputationData: reputationData,
		TotalScore:     825,
		CalculatedAt:   calculatedAt.Format(time.RFC3339),
	}

	// Set up expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", "ACTOR#https://example.com/users/alice").Return(mockQuery).Maybe()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Maybe()
	mockQuery.On("Limit", 1).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{mockRep}
	}).Return(nil)

	// Test
	reputation, err := repo.GetReputation(ctx, "https://example.com/users/alice")

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, reputation)
	assert.InDelta(t, 825, reputation.TotalScore, 0.001)
	assert.Equal(t, "https://example.com/users/alice", reputation.ActorID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetReputationDoesNotReturnLegacyMismatchedActor(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()

	calculatedAt := time.Now().UTC()
	localReputationData := fmt.Sprintf(`{
		"actor_id": "https://example.com/users/alice",
		"instance_url": "https://example.com",
		"total_score": 825,
		"calculated_at": "%s"
	}`, calculatedAt.Format(time.RFC3339))
	localRep := models.Reputation{
		PK:             "ACTOR#alice",
		SK:             "REP#" + calculatedAt.Format(time.RFC3339),
		ReputationData: localReputationData,
		TotalScore:     825,
		CalculatedAt:   calculatedAt.Format(time.RFC3339),
	}

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", mock.MatchedBy(func(pk string) bool {
		return pk == "ACTOR#https://evil.example/users/alice" || pk == "ACTOR#alice"
	})).Return(mockQuery).Maybe()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Maybe()
	mockQuery.On("Limit", 1).Return(mockQuery).Maybe()
	call := 0
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		if call == 0 {
			*reps = []models.Reputation{}
		} else {
			*reps = []models.Reputation{localRep}
		}
		call++
	}).Return(nil)

	reputation, err := repo.GetReputation(ctx, "https://evil.example/users/alice")

	assert.ErrorIs(t, err, storage.ErrNotFound)
	assert.Nil(t, reputation)
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
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", mock.MatchedBy(matchAliceReputationPK)).Return(mockQuery).Maybe()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Maybe()
	mockQuery.On("Limit", 10).Return(mockQuery).Maybe()
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

func TestGetReputationHistoryReturnsCanonicalRows(t *testing.T) {
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewUserRepository(mockDB, "test-table", logger)
	ctx := context.Background()

	calculatedAt := time.Now().UTC()
	reputationData := fmt.Sprintf(`{
		"actor_id": "https://remote.example/users/alice",
		"instance_url": "https://example.com",
		"total_score": 700,
		"calculated_at": "%s"
	}`, calculatedAt.Format(time.RFC3339))
	mockRep := models.Reputation{
		PK:             "ACTOR#https://remote.example/users/alice",
		SK:             "REP#" + calculatedAt.Format(time.RFC3339),
		ReputationData: reputationData,
		TotalScore:     700,
		CalculatedAt:   calculatedAt.Format(time.RFC3339),
	}

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", "ACTOR#https://remote.example/users/alice").Return(mockQuery).Once()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 1).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{mockRep}
	}).Return(nil).Once()

	history, err := repo.GetReputationHistory(ctx, "https://remote.example/users/alice", 1)

	assert.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, "https://remote.example/users/alice", history[0].ActorID)
	assert.Equal(t, 700.0, history[0].TotalScore)
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
	reputationData := `{"actor_id":"https://example.com/users/alice","total_score": 825, "calculated_at": "` + calculatedAt.Format(time.RFC3339) + `"}`

	mockRep := models.Reputation{
		ReputationData: reputationData,
		TotalScore:     825,
	}

	// Set up expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", "ACTOR#https://example.com/users/alice").Return(mockQuery).Maybe()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Maybe()
	mockQuery.On("Limit", 1).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Reputation")).Run(func(args mock.Arguments) {
		reps := args.Get(0).(*[]models.Reputation)
		*reps = []models.Reputation{mockRep}
	}).Return(nil).Once()

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
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery).Maybe()
	mockQuery.On("Where", "PK", "=", mock.MatchedBy(matchAliceReputationPK)).Return(mockQuery).Maybe()
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "REP#").Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Maybe()
	mockQuery.On("Limit", 1).Return(mockQuery).Maybe()
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
