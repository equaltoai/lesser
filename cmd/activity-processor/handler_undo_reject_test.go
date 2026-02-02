package main

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestProcessUndoRejectRecreatesFollowRelationship(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	tableName := "test-table"

	mockDB := new(dynamock.MockDB)
	mockQueryFetch := new(dynamock.MockQuery)
	mockQueryCreate := new(dynamock.MockQuery)

	followActivityID := "https://remote.example/activities/follow123"
	rejectActor := "https://example.com/users/alice"
	followerActor := "https://remote.example/users/bob"

	var createdRelationship *models.RelationshipRecord

	// Allow WithContext to be chained for both fetch and create operations.
	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	// Mock fetching the original follow activity
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		_, ok := model.(*models.Activity)
		return ok
	})).Return(mockQueryFetch)
	mockQueryFetch.On("Where", "SK", "CONTAINS", followActivityID).Return(mockQueryFetch)
	mockQueryFetch.On("Limit", 50).Return(mockQueryFetch)
	mockQueryFetch.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Activity)
		followActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   followActivityID,
				Type: "Follow",
			},
			Actor: followerActor,
			Object: map[string]interface{}{
				"id": followerActor,
			},
		}
		*dest = append(*dest, &models.Activity{Activity: followActivity})
	}).Return(nil)

	// Mock creating the restored follow relationship
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		rel, ok := model.(*models.RelationshipRecord)
		if ok {
			createdRelationship = rel
		}
		return ok
	})).Run(func(args mock.Arguments) {
		if rel, ok := args.Get(0).(*models.RelationshipRecord); ok {
			createdRelationship = rel
		}
	}).Return(mockQueryCreate)
	mockQueryCreate.On("Create").Return(nil)

	handler := &ActivityHandler{
		DB:               mockDB,
		TableName:        tableName,
		Logger:           logger,
		ActivityRepo:     repositories.NewActivityRepository(mockDB, tableName, logger, nil),
		RelationshipRepo: repositories.NewRelationshipRepository(mockDB, tableName, logger),
	}

	undoActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "undo-1",
			Type: "Undo",
		},
		Actor: rejectActor,
		Object: map[string]interface{}{
			"type":   "Reject",
			"actor":  rejectActor,
			"object": followActivityID,
		},
	}

	err := handler.processUndoActivity(ctx, undoActivity, "alice")
	require.NoError(t, err)

	require.NotNil(t, createdRelationship)
	assert.Equal(t, "FOLLOW#bob", createdRelationship.PK)
	assert.Equal(t, "FOLLOWING#alice", createdRelationship.SK)
	assert.Equal(t, followActivityID, createdRelationship.ActivityID)
	assert.Equal(t, models.RelationshipPending, createdRelationship.State)

	mockQueryCreate.AssertExpectations(t)
	mockQueryFetch.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}
