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
	dynamock "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityProcessor_SendToDeadLetterQueue(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	err := ap.sendToDeadLetterQueue(ctx, events.DynamoDBEventRecord{EventID: "evt-1", EventName: "INSERT"}, "boom")
	require.NoError(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_IsRetryableStreamError(t *testing.T) {
	ap := &ActivityProcessor{}

	require.False(t, ap.isRetryableStreamError(nil))
	require.True(t, ap.isRetryableStreamError(errors.New("Timeout while connecting to DynamoDB")))
	require.False(t, ap.isRetryableStreamError(errors.New("validation failed 422")))
	require.True(t, ap.isRetryableStreamError(errors.New("some other unknown error")))
}

func TestActivityProcessor_ValidateAndProcessRemoteObject(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	_, err := ap.validateAndProcessRemoteObject("nope", "https://remote.example/objects/1")
	require.Error(t, err)

	_, err = ap.validateAndProcessRemoteObject(map[string]any{}, "https://remote.example/objects/1")
	require.Error(t, err)

	_, err = ap.validateAndProcessRemoteObject(map[string]any{"id": "https://remote.example/objects/WRONG"}, "https://remote.example/objects/1")
	require.Error(t, err)

	_, err = ap.validateAndProcessRemoteObject(map[string]any{"id": "https://remote.example/objects/1"}, "https://remote.example/objects/1")
	require.Error(t, err)

	obj, err := ap.validateAndProcessRemoteObject(map[string]any{
		"id":           "https://remote.example/objects/1",
		"type":         "Note",
		"attributedTo": "https://remote.example/users/bob",
		"content":      "<p>hi</p>",
	}, "https://remote.example/objects/1")
	require.NoError(t, err)
	note, ok := obj.(*activitypub.Note)
	require.True(t, ok)
	require.Equal(t, "https://remote.example/objects/1", note.ID)

	_, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":   "https://remote.example/objects/2",
		"type": "Video",
	}, "https://remote.example/objects/2")
	require.Error(t, err)

	obj, err = ap.validateAndProcessRemoteObject(map[string]any{
		"id":   "https://remote.example/objects/3",
		"type": "SomethingCustom",
	}, "https://remote.example/objects/3")
	require.NoError(t, err)
	_, ok = obj.(map[string]any)
	require.True(t, ok)
}

func TestActivityProcessor_FetchRemoteObjectWithRetry_RetriesAndSucceeds(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	orig := fetchAuthorizedObjectFn
	t.Cleanup(func() { fetchAuthorizedObjectFn = orig })

	var calls int
	fetchAuthorizedObjectFn = func(_ context.Context, _ *federation.AuthorizedFetchService, objectURL string, _ *activitypub.Actor) (any, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("timeout")
		}
		return map[string]any{
			"id":           objectURL,
			"type":         "Note",
			"attributedTo": "https://remote.example/users/bob",
			"content":      "hi",
		}, nil
	}

	ap := &ActivityProcessor{
		db:            mockDB,
		logger:        zap.NewNop(),
		retryAttempts: 2,
		retryDelay:    4 * time.Nanosecond,
	}

	signingActor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}
	obj, err := ap.fetchRemoteObjectWithRetry(ctx, "https://remote.example/objects/1", signingActor)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.IsType(t, &activitypub.Note{}, obj)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_StoreGenericRemoteObject(t *testing.T) {
	ctx := context.Background()

	objectRepo := testmocks.NewMockObjectRepository()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil)

	ap := &ActivityProcessor{
		logger:     zap.NewNop(),
		objectRepo: objectRepo,
	}

	ap.storeGenericRemoteObject(ctx, map[string]interface{}{})
	ap.storeGenericRemoteObject(ctx, map[string]interface{}{
		"id":           "https://remote.example/objects/1",
		"type":         "Article",
		"attributedTo": "https://remote.example/users/bob",
		"content":      "hello",
	})

	objectRepo.AssertExpectations(t)
}
