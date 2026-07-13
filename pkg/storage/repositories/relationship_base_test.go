package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRelationshipBase_generateKeys_and_helpers(t *testing.T) {
	logger := zap.NewNop()
	base := NewRelationshipBase(nil, "test-table", logger, RelationshipTypeFollow)

	pk, sk := base.generateKeys("alice", "bob")
	assert.Equal(t, "USER#alice", pk)
	assert.Equal(t, "FOLLOWING#bob", sk)

	base.relType = RelationshipTypeLike
	pk, sk = base.generateKeys("alice", "note-1")
	assert.Equal(t, "object#note-1#likes", pk)
	assert.Equal(t, "actor#alice", sk)

	base.relType = RelationshipTypeBlock
	pk, sk = base.generateKeys("https://example.com/users/alice", "https://example.com/users/bob")
	assert.Equal(t, "ACTOR#alice#BLOCKS", pk)
	assert.Equal(t, "BLOCKED#bob", sk)

	base.relType = RelationshipTypeMute
	pk, sk = base.generateKeys("https://example.com/users/alice", "@bob@example.org")
	assert.Equal(t, "ACTOR#alice#MUTES", pk)
	assert.Equal(t, "MUTED#@bob@example.org", sk)

	base.relType = RelationshipTypeBookmark
	pk, sk = base.generateKeys("alice", "status-1")
	assert.Equal(t, "USER#alice#BOOKMARKS", pk)
	assert.Equal(t, "STATUS#status-1", sk)

	base.relType = RelationshipTypeFavorite
	pk, sk = base.generateKeys("alice", "status-2")
	assert.Equal(t, "USER#alice#FAVORITES", pk)
	assert.Equal(t, "STATUS#status-2", sk)

	base.relType = RelationshipType("custom")
	pk, sk = base.generateKeys("alice", "obj")
	assert.Equal(t, "custom#alice", pk)
	assert.Equal(t, "object#obj", sk)

	base.relType = RelationshipTypeFollow
	assert.Equal(t, "actor#alice#follows", base.generateGSI1PK("alice"))
	assert.Equal(t, "object#bob", base.generateGSI1SK("bob"))

	base.relType = RelationshipTypeLike
	assert.Equal(t, "object#s1#likes", base.generateObjectPK("s1"))
	base.relType = RelationshipTypeBlock
	assert.Equal(t, "object#s1#blocks", base.generateObjectPK("s1"))
	base.relType = RelationshipTypeMute
	assert.Equal(t, "object#s1#mutes", base.generateObjectPK("s1"))
	base.relType = RelationshipTypeFavorite
	assert.Equal(t, "object#s1#favorites", base.generateObjectPK("s1"))

	base.relType = RelationshipTypeFollow
	assert.Equal(t, EntityFollow, base.getEntityType())
	base.relType = RelationshipTypeBlock
	assert.Equal(t, EntityBlock, base.getEntityType())
	base.relType = RelationshipTypeMute
	assert.Equal(t, EntityMute, base.getEntityType())
	base.relType = RelationshipTypeBookmark
	assert.Equal(t, EntityBookmark, base.getEntityType())
	base.relType = RelationshipTypeLike
	assert.Equal(t, "favorite", base.getEntityType())
	base.relType = RelationshipType("unknown")
	assert.Equal(t, "unknown", base.getEntityType())
}

func TestRelationshipBase_mapToStruct(t *testing.T) {
	now := time.Now().UTC()
	input := map[string]interface{}{
		"PK":        "PK1",
		"SK":        "SK1",
		"Actor":     "alice",
		"Object":    "bob",
		"Type":      "follow",
		"CreatedAt": now,
		"ID":        "id-1",
	}

	var model RelationshipModel
	err := mapToStruct(input, &model)
	assert.NoError(t, err)
	assert.Equal(t, "PK1", model.PK)
	assert.Equal(t, "SK1", model.SK)
	assert.Equal(t, "alice", model.Actor)
	assert.Equal(t, "bob", model.Object)
	assert.Equal(t, "follow", model.Type)
	assert.Equal(t, "id-1", model.ID)
	assert.Equal(t, now, model.CreatedAt)

	// No-op for non-*RelationshipModel targets
	assert.NoError(t, mapToStruct(map[string]interface{}{"PK": "x"}, &struct{}{}))
}

func TestRelationshipBase_CreateRelationship_idempotent_and_error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	base := NewRelationshipBase(mockDB, "test-table", logger, RelationshipTypeFollow)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	mockQuery.On("Create").Return(errors.ErrConditionFailed).Once()
	assert.NoError(t, base.CreateRelationship(ctx, "alice", "bob", "id-1"))

	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	assert.Error(t, base.CreateRelationship(ctx, "alice", "bob", "id-2"))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestRelationshipBase_GetRelationship_and_queries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	base := NewRelationshipBase(mockDB, "test-table", logger, RelationshipTypeFollow)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
	got, err := base.GetRelationship(ctx, "alice", "bob")
	assert.Error(t, err)
	assert.Nil(t, got)

	mockQuery.On("First", mock.Anything).Return(nil).Once()
	got, err = base.GetRelationship(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.NotNil(t, got)

	// ExistsRelationship + counts go through QueryUtils and are mostly DB count wrappers
	mockQuery.On("Count").Return(int64(2), nil).Maybe()
	exists, err := base.ExistsRelationship(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, exists)

	count, err := base.CountRelationshipsByActor(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = base.CountRelationshipsByObject(ctx, "bob")
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	// Relationship list methods convert map results into RelationshipModel
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]map[string]interface{})
		*out = []map[string]interface{}{
			{"PK": "FOLLOW#alice", "SK": "FOLLOWING#bob", "Actor": "alice", "Object": "bob"},
			{"PK": "FOLLOW#alice", "SK": "FOLLOWING#carol", "Actor": "alice", "Object": "carol"},
		}
	}).Once()

	models, cursor, err := base.GetRelationshipsByActor(ctx, "alice", 1, "")
	assert.NoError(t, err)
	assert.Len(t, models, 1)
	assert.NotEmpty(t, cursor)

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]map[string]interface{})
		*out = []map[string]interface{}{
			{"PK": "USER#alice#BOOKMARKS", "SK": "STATUS#1", "Actor": "alice", "Object": "1"},
		}
	}).Once()
	base.relType = RelationshipTypeBookmark
	models, cursor, err = base.GetRelationshipsByObject(ctx, "alice", 10, "")
	assert.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Empty(t, cursor)
}

func TestRelationshipBase_DeleteRelationship_and_query_errors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	base := NewRelationshipBase(mockDB, "test-table", logger, RelationshipTypeFollow)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Delete").Return(nil).Once()

	assert.NoError(t, base.DeleteRelationship(ctx, "alice", "bob"))

	mockQuery.On("Delete").Return(fmt.Errorf("delete failed")).Once()
	assert.Error(t, base.DeleteRelationship(ctx, "alice", "bob"))

	// QueryByGSI error path for GetRelationshipsByActor
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("query failed")).Once()
	_, _, err := base.GetRelationshipsByActor(ctx, "alice", 10, "")
	assert.Error(t, err)

	// Prefix query error path for GetRelationshipsByObject
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("query failed")).Once()
	_, _, err = base.GetRelationshipsByObject(ctx, "bob", 10, "")
	assert.Error(t, err)
}
