package repositories

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func setupPermissiveDBAndQuery() (*mocks.MockDB, *mocks.MockQuery) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()

	return mockDB, mockQuery
}

func TestRelationshipRepository_domain_helpers_and_updates(t *testing.T) {
	assert.Equal(t, "example.com", normalizeDomain("https://Example.com/"))
	assert.Equal(t, "example.com", normalizeDomain("http://Example.com/"))
	assert.Equal(t, "example.com", normalizeDomain("example.com/"))
	assert.Equal(t, "", normalizeDomain(""))

	logger := zap.NewNop()
	repo := NewRelationshipRepository(nil, "test-table", logger)

	repo.ensureDomainIndexes(nil) // no-op

	record := &models.RelationshipRecord{SK: "FOLLOWING#bob"}
	repo.ensureDomainIndexes(record)
	assert.NotEmpty(t, record.GSI2PK)
	assert.NotEmpty(t, record.GSI2SK)

	record2 := &models.RelationshipRecord{GSI1SK: "FOLLOWER#alice"}
	repo.ensureDomainIndexes(record2)
	assert.NotEmpty(t, record2.GSI3PK)
	assert.NotEmpty(t, record2.GSI3SK)

	rel := &models.RelationshipRecord{
		State:          models.RelationshipAccepted,
		Notifying:      false,
		ShowingReblogs: false,
		Languages:      []string{"en"},
		Note:           "hi",
	}

	assert.False(t, repo.applyFieldUpdate(rel, "unsupported", "x", "a", "b"))
	assert.False(t, repo.applyFieldUpdate(rel, "Notifying", "not-bool", "a", "b"))
	assert.True(t, repo.applyFieldUpdate(rel, "notifying", true, "a", "b"))
	assert.True(t, repo.applyFieldUpdate(rel, "showing_reblogs", true, "a", "b"))
	assert.True(t, repo.applyFieldUpdate(rel, "note", "updated", "a", "b"))
	assert.False(t, repo.applyFieldUpdate(rel, "languages", "not-a-slice", "a", "b"))
	assert.True(t, repo.applyFieldUpdate(rel, "languages", []string{"en", "fr"}, "a", "b"))
	assert.False(t, repo.applyFieldUpdate(rel, "languages", []string{"en", "fr"}, "a", "b"))
}

func TestRelationshipRepository_followers_following_and_counts(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// Followers with pagination cursor
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.RelationshipRecord)
		*out = []models.RelationshipRecord{
			{GSI1SK: "FOLLOWER#bob"},
			{GSI1SK: "FOLLOWER#carol"},
		}
	}).Once()

	// Following with cursor and PK mismatch warning path
	cursor := Utils.Pagination.EncodeCursor("FOLLOW#someoneelse", "FOLLOWING#0")
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.RelationshipRecord)
		*out = []models.RelationshipRecord{
			{SK: "FOLLOWING#bob"},
			{SK: "FOLLOWING#carol"},
		}
	}).Once()

	// Count followers/following (success and error paths)
	mockQuery.On("Count").Return(int64(5), nil).Once()
	mockQuery.On("Count").Return(int64(7), nil).Once()
	mockQuery.On("Count").Return(int64(0), fmt.Errorf("count boom")).Once()

	// Fallbacks (added after specific expectations)
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()

	followers, next, err := repo.GetFollowers(ctx, "alice", 1, "")
	assert.NoError(t, err)
	assert.Equal(t, []string{"bob"}, followers)
	assert.NotEmpty(t, next)

	// Cursor decode failure
	_, _, err = repo.GetFollowers(ctx, "alice", 10, "not-a-valid-cursor")
	assert.Error(t, err)

	following, next, err := repo.GetFollowing(ctx, "alice", 1, cursor)
	assert.NoError(t, err)
	assert.Equal(t, []string{"bob"}, following)
	assert.NotEmpty(t, next)

	count, err := repo.CountFollowers(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, 5, count)

	count, err = repo.CountFollowing(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, 7, count)

	_, err = repo.CountFollowing(ctx, "alice")
	assert.Error(t, err)
}

func TestRelationshipRepository_update_requests_and_relationship_checks(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// Keep UpdateRelationship simple: disable optional enhanced services
	repo.EnhancedBaseRepository.SetValidationService(nil)
	repo.EnhancedBaseRepository.SetPermissionService(nil)
	repo.EnhancedBaseRepository.SetCachingService(nil)
	repo.EnhancedBaseRepository.SetEventService(nil)

	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.GSI1SK = "FOLLOWER#alice"
		model.State = models.RelationshipAccepted
		model.Notifying = false
		model.ShowingReblogs = false
		model.Languages = []string{"en"}
		model.Note = ""
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	err := repo.UpdateRelationship(ctx, "alice", "bob", map[string]interface{}{
		"notifying": true,
		"languages": []string{"en", "fr"},
	})
	assert.NoError(t, err)

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipAccepted
		model.Notifying = true
	}).Once()

	// No-op update when there are no changes
	err = repo.UpdateRelationship(ctx, "alice", "bob", map[string]interface{}{"notifying": true})
	assert.NoError(t, err)

	// Accept + reject follow request use GetRelationship + Update
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.PK = "FOLLOW#alice"
		model.SK = "FOLLOWING#bob"
		model.State = models.RelationshipPending
	}).Twice()
	mockQuery.On("Update", mock.Anything).Return(nil).Twice()

	assert.NoError(t, repo.AcceptFollowRequest(ctx, "alice", "bob"))
	assert.NoError(t, repo.RejectFollowRequest(ctx, "alice", "bob"))

	// HasFollowRequest uses Exists (count)
	mockQuery.On("Count").Return(int64(1), nil).Once()
	exists, err := repo.HasFollowRequest(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, exists)

	// HasPendingFollowRequest reads and checks state
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.State = models.RelationshipPending
	}).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	pending, err := repo.HasPendingFollowRequest(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, pending)
}

func TestRelationshipRepository_domain_counts_and_collections(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	_, _, err := repo.CountRelationshipsByDomain(ctx, "")
	assert.Error(t, err)

	mockQuery.On("Count").Return(int64(3), nil).Once()
	mockQuery.On("Count").Return(int64(4), nil).Once()
	followers, following, err := repo.CountRelationshipsByDomain(ctx, "example.org")
	assert.NoError(t, err)
	assert.Equal(t, 3, followers)
	assert.Equal(t, 4, following)

	// GetCollectionItems and pagination cursor trimming
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.CollectionItem)
		*out = []models.CollectionItem{
			{Collection: "c1", ItemID: "i1", ItemType: "account", AddedBy: "alice", AddedAt: time.Now(), Position: 1, SK: "ITEM#i1"},
			{Collection: "c1", ItemID: "i2", ItemType: "account", AddedBy: "alice", AddedAt: time.Now(), Position: 2, SK: "ITEM#i2"},
		}
	}).Once()

	items, next, err := repo.GetCollectionItems(ctx, "c1", 1, "")
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "ITEM#i1", next)

	// IsInCollection not found vs success
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	in, err := repo.IsInCollection(ctx, "c1", "missing")
	assert.NoError(t, err)
	assert.False(t, in)

	mockQuery.On("First", mock.Anything).Return(nil).Once()
	in, err = repo.IsInCollection(ctx, "c1", "i1")
	assert.NoError(t, err)
	assert.True(t, in)

	// CountCollectionItems error handling
	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, err = repo.CountCollectionItems(ctx, "c1")
	assert.Error(t, err)
}
