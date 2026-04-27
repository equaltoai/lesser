package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityProcessor_GetAnnouncedContent_RemotePath(t *testing.T) {
	ctx := context.Background()

	orig := fetchAuthorizedObjectFn
	t.Cleanup(func() { fetchAuthorizedObjectFn = orig })

	fetchAuthorizedObjectFn = func(_ context.Context, _ *federation.AuthorizedFetchService, objectURL string, _ *activitypub.Actor) (any, error) {
		return map[string]any{
			"id":           objectURL,
			"type":         "Note",
			"attributedTo": "https://remote.example/users/bob",
			"content":      "remote content",
		}, nil
	}

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	actorRepo := testmocks.NewMockActorRepository()
	actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil).Once()

	objectRepo := testmocks.NewMockObjectRepository()
	objectRepo.On("GetObject", mock.Anything, "https://remote.example/objects/1").Return(nil, errors.New("not found")).Once()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Once()

	ap := &ActivityProcessor{
		db:            mockDB,
		logger:        zap.NewNop(),
		actorRepo:     actorRepo,
		objectRepo:    objectRepo,
		baseURL:       "https://example.com",
		retryAttempts: 1,
		retryDelay:    1 * time.Nanosecond,
	}

	content, author := ap.getAnnouncedContent(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, "https://remote.example/objects/1")
	require.Equal(t, "remote content", content)
	require.Equal(t, "https://remote.example/users/bob", author)

	actorRepo.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_ProcessActivityDeleted_CreateErrorBranch(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("boom"))

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	err := ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-remove-create-error",
		EventName: activityRemove,
		Change: events.DynamoDBStreamRecord{
			OldImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#create"),
				"SK":        events.NewStringAttribute("SK#create"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"type":      events.NewStringAttribute("Create"),
				"activity":  events.NewStringAttribute(`{"id":"https://example.com/activities/act-1","type":"Create","actor":"https://example.com/users/alice","object":"https://example.com/objects/1"}`),
			},
		},
	})
	require.Error(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}
