package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityHandler_NewAndHelpers(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	handler := NewActivityHandler(new(dynamock.MockDB), "test-table")
	require.NotNil(t, handler)
	require.NotNil(t, handler.RouteManager)

	manual := &ActivityHandler{
		DB:        new(dynamock.MockDB),
		TableName: "test-table",
		Logger:    zap.NewNop(),
	}
	require.NotNil(t, manual.createNotificationRepo())

	out := manual.interfaceSliceToStringSlice([]any{"a", 1, "b"})
	require.Equal(t, []string{"a", "", "b"}, out)
}

func TestActivityHandler_ProcessActivityByTypeAndDelivery(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	handler := &ActivityHandler{Logger: zap.NewNop()}

	// Unsupported inbox types should be ignored.
	require.NoError(t, handler.processInboxActivity(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "1", Type: "SomethingElse"},
		Actor:      "https://example.com/users/alice",
	}, "alice"))

	// Outbox types should route to deliverActivity.
	require.NoError(t, handler.processOutboxActivity(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "2",
			Type: ActivityTypeCreate,
			To:   []string{"https://remote.example/users/bob"},
		},
		Actor: "https://example.com/users/alice",
	}, "alice"))
}

func TestActivityHandler_ProcessRejectUpdateAndStreamRecordNoop(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	relationshipRepo := testmocks.NewMockRelationshipRepository()
	activityRepo := testmocks.NewMockActivityRepository()
	notificationRepo := testmocks.NewMockNotificationRepository()

	follow := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "follow-1", Type: ActivityTypeFollow},
		Actor:      "https://remote.example/users/bob",
		Object:     "https://example.com/users/alice",
	}
	activityRepo.On("GetActivity", mock.Anything, "follow-1").Return(follow, nil).Once()
	relationshipRepo.On("GetRelationship", mock.Anything, "bob", "alice").Return(&models.RelationshipRecord{
		State:      models.RelationshipPending,
		ActivityID: "follow-1",
	}, nil).Once()
	relationshipRepo.On("RejectFollowRequest", mock.Anything, "bob", "alice").Return(nil).Once()
	notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	handler := &ActivityHandler{
		ActivityRepo:     activityRepo,
		Logger:           zap.NewNop(),
		RelationshipRepo: relationshipRepo,
		NotificationRepo: notificationRepo,
	}

	reject := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "reject-1", Type: ActivityTypeReject},
		Actor:      "https://example.com/users/alice",
		Object:     "follow-1",
	}
	require.NoError(t, handler.processRejectActivity(ctx, reject, "bob"))

	require.NoError(t, handler.processUpdateActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "update-1", Type: ActivityTypeUpdate}}, "alice"))

	// Handler.processRecord only processes INSERT events.
	require.NoError(t, handler.processRecord(ctx, events.DynamoDBEventRecord{EventName: "MODIFY"}))

	relationshipRepo.AssertExpectations(t)
	activityRepo.AssertExpectations(t)
	notificationRepo.AssertExpectations(t)
}
