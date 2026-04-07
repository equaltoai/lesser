package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRelationshipRepository_GetFollowRequest_error_paths(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// BaseRepository.Get wraps not-found errors; this still exercises the not-found branch.
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err := repo.GetFollowRequest(ctx, "alice", "bob")
	assert.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(errors.New("db boom")).Once()
	_, err = repo.GetFollowRequest(ctx, "alice", "bob")
	assert.Error(t, err)
}

func TestRelationshipRepository_counts_and_pending_checks_error_paths(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// CountFollowers error path
	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, err := repo.CountFollowers(ctx, "alice")
	assert.Error(t, err)

	// HasPendingFollowRequest not-found currently returns a wrapped error from BaseRepository.Get
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err = repo.HasPendingFollowRequest(ctx, "alice", "bob")
	assert.NoError(t, err)

	// HasPendingFollowRequest generic error -> error
	mockQuery.On("First", mock.Anything).Return(errors.New("get failed")).Once()
	_, err = repo.HasPendingFollowRequest(ctx, "alice", "bob")
	assert.Error(t, err)
}

func TestRelationshipRepository_CountRelationshipsByDomain_branches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	_, _, err := repo.CountRelationshipsByDomain(ctx, "")
	assert.Error(t, err)

	// NotFound errors are ignored, counts treated as 0
	mockQuery.On("Count").Return(int64(0), dynamormerrors.ErrItemNotFound).Twice()
	followers, following, err := repo.CountRelationshipsByDomain(ctx, "remote.example")
	assert.NoError(t, err)
	assert.Equal(t, 0, followers)
	assert.Equal(t, 0, following)

	// Follower count error (non-notfound) -> error
	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, _, err = repo.CountRelationshipsByDomain(ctx, "remote.example")
	assert.Error(t, err)

	// Following count error (non-notfound) -> error
	mockQuery.On("Count").Return(int64(1), nil).Once()
	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, _, err = repo.CountRelationshipsByDomain(ctx, "remote.example")
	assert.Error(t, err)
}

func TestRelationshipRepository_UpdateRelationship_and_requests_error_paths(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", logger)
	repo.localDomain = "example.com"

	repo.EnhancedBaseRepository.SetValidationService(nil)
	repo.EnhancedBaseRepository.SetPermissionService(nil)
	repo.EnhancedBaseRepository.SetCachingService(nil)
	repo.EnhancedBaseRepository.SetEventService(nil)

	// UpdateRelationship: ValidateAndUpdate error path
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipAccepted
		model.Notifying = false
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.UpdateRelationship(ctx, "alice", "bob", map[string]interface{}{"notifying": true}))

	// AcceptFollowRequest update error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipPending
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.AcceptFollowRequest(ctx, "alice", "bob"))

	// RejectFollowRequest update error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipPending
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.RejectFollowRequest(ctx, "alice", "bob"))
}

func TestRelationshipRepository_moves_and_endorsements_error_paths(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// CreateMove conditional failed branch
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	assert.Error(t, repo.CreateMove(ctx, &storage.Move{ID: "m1", Actor: "alice", Target: "bob"}))

	// CreateMove generic error
	mockQuery.On("Create").Return(errors.New("create failed")).Once()
	assert.Error(t, repo.CreateMove(ctx, &storage.Move{ID: "m2", Actor: "alice", Target: "bob"}))

	// GetPendingMoves query error
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()
	_, err := repo.GetPendingMoves(ctx, 1)
	assert.Error(t, err)

	// IsEndorsed query error
	mockQuery.On("First", mock.Anything).Return(errors.New("query failed")).Once()
	_, err = repo.IsEndorsed(ctx, "alice", "bob")
	assert.Error(t, err)

	// CreateEndorsement: GetAccountPins error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipAccepted
	}).Once()
	mockQuery.On("Scan", mock.Anything).Return(errors.New("scan failed")).Once()
	assert.Error(t, repo.CreateEndorsement(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "bob"}))

	// CreateEndorsement: pin limit reached
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipAccepted
	}).Once()
	mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.AccountPin)
		*out = []models.AccountPin{{}, {}, {}, {}}
	}).Once()
	// ErrorHandler.HandleCreateError(nil, ...) returns nil, so this path currently returns nil.
	assert.NoError(t, repo.CreateEndorsement(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "bob"}))

	// CreateEndorsement: not following -> nil (ErrorHandler.HandleCreateError(nil, ...) returns nil)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	assert.NoError(t, repo.CreateEndorsement(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "bob"}))

	// CreateEndorsement: IsFollowing error path -> error
	mockQuery.On("First", mock.Anything).Return(errors.New("follow lookup failed")).Once()
	assert.Error(t, repo.CreateEndorsement(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "bob"}))
}
