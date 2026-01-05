package repositories

import (
	"context"
	"testing"
	"time"

	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestRelationshipRepository_additional_zero_percent_methods(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// NewRelationshipRepositoryWithCostTracking
	mockDB := new(mocks.MockDB)
	repo := NewRelationshipRepositoryWithCostTracking(mockDB, "test-table", logger, (*cost.TrackingService)(nil))
	assert.NotNil(t, repo)

	// resolveLocalDomain with non-empty domain
	cfg := lesserconfig.Get()
	previousDomain := cfg.Domain
	cfg.Domain = "https://example.com/"
	assert.Equal(t, "example.com", resolveLocalDomain())
	cfg.Domain = previousDomain

	// GetFollowRequest success mapping
	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	mockDB2.On("WithContext", mock.Anything).Return(mockDB2)
	mockDB2.On("Model", mock.Anything).Return(mockQuery2)
	mockQuery2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery2)
	mockQuery2.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.RelationshipRecord)
		m.PK = "FOLLOW#alice"
		m.SK = "FOLLOWING#bob"
		m.GSI1PK = "FOLLOW#bob"
		m.GSI1SK = "FOLLOWER#alice"
		m.ActivityID = "act-1"
		m.State = models.RelationshipPending
		m.CreatedAt = time.Now()
		m.UpdatedAt = time.Now()
	}).Once()

	repo2 := NewRelationshipRepository(mockDB2, "test-table", logger)
	got, err := repo2.GetFollowRequest(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.Equal(t, "act-1", got.ActivityID)

	// CreateRelationship (success + conditional failure idempotency)
	mockDB3 := new(mocks.MockDB)
	mockQuery3 := new(mocks.MockQuery)
	mockDB3.On("WithContext", mock.Anything).Return(mockDB3)
	mockDB3.On("Model", mock.Anything).Return(mockQuery3)
	mockQuery3.On("Create").Return(nil).Once()
	repo3 := NewRelationshipRepository(mockDB3, "test-table", logger)
	repo3.EnhancedBaseRepository.SetValidationService(nil)
	repo3.EnhancedBaseRepository.SetPermissionService(nil)
	repo3.EnhancedBaseRepository.SetCachingService(nil)
	repo3.EnhancedBaseRepository.SetEventService(nil)
	assert.NoError(t, repo3.CreateRelationship(ctx, "alice", "bob", "act-1"))

	mockDB4 := new(mocks.MockDB)
	mockQuery4 := new(mocks.MockQuery)
	mockDB4.On("WithContext", mock.Anything).Return(mockDB4)
	mockDB4.On("Model", mock.Anything).Return(mockQuery4)
	mockQuery4.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	repo4 := NewRelationshipRepository(mockDB4, "test-table", logger)
	repo4.EnhancedBaseRepository.SetValidationService(nil)
	repo4.EnhancedBaseRepository.SetPermissionService(nil)
	repo4.EnhancedBaseRepository.SetCachingService(nil)
	repo4.EnhancedBaseRepository.SetEventService(nil)
	assert.NoError(t, repo4.CreateRelationship(ctx, "alice", "bob", "act-2"))

	// GetFollowerCount / GetFollowingCount
	mockDB5 := new(mocks.MockDB)
	mockQuery5 := new(mocks.MockQuery)
	mockDB5.On("WithContext", mock.Anything).Return(mockDB5)
	mockDB5.On("Model", mock.Anything).Return(mockQuery5)
	mockQuery5.On("Index", mock.Anything).Return(mockQuery5).Maybe()
	mockQuery5.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery5).Maybe()
	mockQuery5.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery5).Maybe()
	mockQuery5.On("Count").Return(int64(5), nil).Once()
	mockQuery5.On("Count").Return(int64(7), nil).Once()
	repo5 := NewRelationshipRepository(mockDB5, "test-table", logger)
	followerCount, err := repo5.GetFollowerCount(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, int64(5), followerCount)
	followingCount, err := repo5.GetFollowingCount(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, int64(7), followingCount)

	// GetPendingFollowRequests
	mockDB6 := new(mocks.MockDB)
	mockQuery6 := new(mocks.MockQuery)
	mockDB6.On("WithContext", mock.Anything).Return(mockDB6)
	mockDB6.On("Model", mock.Anything).Return(mockQuery6)
	mockQuery6.On("Index", mock.Anything).Return(mockQuery6)
	mockQuery6.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery6)
	mockQuery6.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery6)
	mockQuery6.On("Limit", mock.Anything).Return(mockQuery6)
	mockQuery6.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.RelationshipRecord)
		*out = []models.RelationshipRecord{{GSI1SK: "FOLLOWER#bob"}}
	}).Once()
	repo6 := NewRelationshipRepository(mockDB6, "test-table", logger)
	reqs, _, err := repo6.GetPendingFollowRequests(ctx, "alice", 10, "")
	assert.NoError(t, err)
	assert.Equal(t, []string{"bob"}, reqs)

	// GetAccountMoves + VerifyMove
	mockDB7 := new(mocks.MockDB)
	mockQuery7 := new(mocks.MockQuery)
	mockDB7.On("WithContext", mock.Anything).Return(mockDB7)
	mockDB7.On("Model", mock.Anything).Return(mockQuery7)
	mockQuery7.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery7)
	mockQuery7.On("Limit", mock.Anything).Return(mockQuery7)
	mockQuery7.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Move)
		*out = []models.Move{{ID: "m1", Actor: "alice", Target: "bob"}}
	}).Once()
	repo7 := NewRelationshipRepository(mockDB7, "test-table", logger)
	moves, err := repo7.GetAccountMoves(ctx, "alice")
	assert.NoError(t, err)
	assert.Len(t, moves, 1)
	mockQuery7.On("Update", mock.Anything).Return(nil).Once()
	assert.NoError(t, repo7.VerifyMove(ctx, "alice", "bob"))

	// AddToCollection covers shared helper call
	mockDB8 := new(mocks.MockDB)
	mockQuery8 := new(mocks.MockQuery)
	mockDB8.On("WithContext", mock.Anything).Return(mockDB8)
	mockDB8.On("Model", mock.Anything).Return(mockQuery8)
	mockQuery8.On("Create").Return(nil).Once()
	repo8 := NewRelationshipRepository(mockDB8, "test-table", logger)
	assert.NoError(t, repo8.AddToCollection(ctx, "c1", &storage.CollectionItem{ItemID: "i1", ItemType: "account", AddedBy: "alice"}))

	// BlockUser validation + success branch
	mockDB9 := new(mocks.MockDB)
	mockQuery9 := new(mocks.MockQuery)
	mockDB9.On("WithContext", mock.Anything).Return(mockDB9)
	mockDB9.On("Model", mock.Anything).Return(mockQuery9)
	mockQuery9.On("Create").Return(nil).Once()
	repo9 := NewRelationshipRepository(mockDB9, "test-table", logger)
	assert.Error(t, repo9.BlockUser(ctx, "", "bob"))
	assert.NoError(t, repo9.BlockUser(ctx, "alice", "bob"))
}
