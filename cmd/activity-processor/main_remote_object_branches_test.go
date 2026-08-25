package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityProcessor_ProcessActivityObject_NoteAndDefaultBranches(t *testing.T) {
	ctx := context.Background()
	ap := &ActivityProcessor{logger: zap.NewNop()}

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/objects/1", Type: "Note"},
		Content:    "hello",
	}

	obj, err := ap.processActivityObject(ctx, &activitypub.Activity{Object: note})
	require.NoError(t, err)
	require.NotNil(t, obj)
	require.Equal(t, "hello", obj.Content)

	obj, err = ap.processActivityObject(ctx, &activitypub.Activity{Object: 123})
	require.NoError(t, err)
	require.Nil(t, obj)
}

func TestActivityProcessor_ProcessRemoteObject_Branches(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	obj, err := ap.processRemoteObject(&activitypub.Note{
		BaseObject: activitypub.BaseObject{ID: "https://remote.example/objects/1", Type: "Note"},
		Content:    "note content",
	}, "https://remote.example/objects/1")
	require.NoError(t, err)
	require.True(t, obj.IsRemote)
	require.Equal(t, "note content", obj.Content)

	obj, err = ap.processRemoteObject(map[string]any{"content": "map content"}, "https://remote.example/objects/2")
	require.NoError(t, err)
	require.Equal(t, "map content", obj.Content)

	obj, err = ap.processRemoteObject(map[string]any{"id": "https://remote.example/objects/3"}, "https://remote.example/objects/3")
	require.NoError(t, err)
	require.Contains(t, obj.Content, "Remote object:")

	obj, err = ap.processRemoteObject("nope", "https://remote.example/objects/4")
	require.NoError(t, err)
	require.Contains(t, obj.Content, "Remote object:")
}

func TestActivityProcessor_FetchRemoteMapAndStringObject_Branches(t *testing.T) {
	ctx := context.Background()

	orig := fetchAuthorizedObjectFn
	t.Cleanup(func() { fetchAuthorizedObjectFn = orig })

	t.Run("fetchRemoteMapObject actor lookup fails", func(t *testing.T) {
		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return((*activitypub.Actor)(nil), errors.New("boom")).Once()

		ap := &ActivityProcessor{
			logger:    zap.NewNop(),
			actorRepo: actorRepo,
			baseURL:   "https://example.com",
		}

		obj, err := ap.fetchRemoteMapObject(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, map[string]any{}, "https://remote.example/objects/1")
		require.NoError(t, err)
		require.Equal(t, "https://remote.example/objects/1", obj.ObjectID)
		require.Contains(t, obj.Content, "Remote object:")
		actorRepo.AssertExpectations(t)
	})

	t.Run("fetchRemoteMapObject fetch fails", func(t *testing.T) {
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
		mockQuery.On("Create").Return(nil).Maybe()

		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil).Once()

		fetchAuthorizedObjectFn = func(_ context.Context, _ *federation.AuthorizedFetchService, _ string, _ *activitypub.Actor) (any, error) {
			return nil, errors.New("status 400")
		}

		ap := &ActivityProcessor{
			db:            mockDB,
			logger:        zap.NewNop(),
			actorRepo:     actorRepo,
			baseURL:       "https://example.com",
			retryAttempts: 1,
			retryDelay:    1 * time.Nanosecond,
		}

		obj, err := ap.fetchRemoteMapObject(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, map[string]any{}, "https://remote.example/objects/1")
		require.NoError(t, err)
		require.Contains(t, obj.Content, "Remote object:")
		actorRepo.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("fetchRemoteMapObject succeeds with note", func(t *testing.T) {
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
		mockQuery.On("Create").Return(nil).Maybe()

		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil).Once()

		fetchAuthorizedObjectFn = func(_ context.Context, _ *federation.AuthorizedFetchService, objectURL string, _ *activitypub.Actor) (any, error) {
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
			actorRepo:     actorRepo,
			baseURL:       "https://example.com",
			retryAttempts: 1,
			retryDelay:    1 * time.Nanosecond,
		}

		obj, err := ap.fetchRemoteMapObject(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, map[string]any{}, "https://remote.example/objects/1")
		require.NoError(t, err)
		require.True(t, obj.IsRemote)
		require.Equal(t, "hi", obj.Content)

		actorRepo.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("fetchStringRemoteObject actor lookup fails", func(t *testing.T) {
		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return((*activitypub.Actor)(nil), errors.New("boom")).Once()

		ap := &ActivityProcessor{
			logger:    zap.NewNop(),
			actorRepo: actorRepo,
		}

		obj, err := ap.fetchStringRemoteObject(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, "https://remote.example/objects/2")
		require.NoError(t, err)
		require.Contains(t, obj.Content, "Referenced object:")
		actorRepo.AssertExpectations(t)
	})
}

func TestActivityProcessor_HandleCreateActivityDeletion_ObjectExtractionBranches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	// Map object path.
	err := ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object:     map[string]any{"id": "https://example.com/objects/1"},
	}, "alice")
	require.NoError(t, err)

	// Note object path.
	err = ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-2", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/objects/2", Type: "Note"}},
	}, "alice")
	require.NoError(t, err)

	// Unsupported object type should be ignored.
	err = ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-3", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object:     123,
	}, "alice")
	require.NoError(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_HandleCreateActivityDeletion_TombstoneError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Create").Return(errors.New("boom"))

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	err := ap.handleCreateActivityDeletion(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-1", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object:     map[string]any{"id": "https://example.com/objects/1"},
	}, "alice")
	require.Error(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestActivityProcessor_FanOutToTimelines_NilAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	ap := &ActivityProcessor{logger: zap.NewNop(), baseURL: "https://example.com"}

	// Unsupported object type -> nil processed object.
	err := ap.fanOutToTimelines(ctx, &activitypub.Activity{Object: 123}, "alice")
	require.NoError(t, err)

	// Embedded map that can't be marshaled -> error.
	err = ap.fanOutToTimelines(ctx, &activitypub.Activity{
		Object: map[string]any{
			"id":      "https://example.com/objects/1",
			"type":    "Note",
			"content": make(chan int),
		},
	}, "alice")
	require.Error(t, err)
}

func TestActivityProcessor_FetchAndProcessRemoteObject_TypeAssertFailure(t *testing.T) {
	ctx := context.Background()
	ap := &ActivityProcessor{logger: zap.NewNop()}

	content, author := ap.fetchAndProcessRemoteObject(ctx, "https://remote.example/objects/1", "not-an-actor")
	require.Contains(t, content, "Boosted:")
	require.Empty(t, author)
}

func TestActivityProcessor_GetRemoteAnnouncedContent_ActorLookupFailure(t *testing.T) {
	ctx := context.Background()

	actorRepo := testmocks.NewMockActorRepository()
	actorRepo.On("GetActor", mock.Anything, "https://example.com/users/alice").Return((*activitypub.Actor)(nil), errors.New("boom")).Once()

	ap := &ActivityProcessor{
		logger:    zap.NewNop(),
		actorRepo: actorRepo,
	}

	content, author := ap.getRemoteAnnouncedContent(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, "https://remote.example/objects/1")
	require.Contains(t, content, "Boosted:")
	require.Empty(t, author)
	actorRepo.AssertExpectations(t)
}

func TestActivityProcessor_ExtractPublishedTime_Branches(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	got := ap.extractPublishedTime(&activitypub.Activity{BaseObject: activitypub.BaseObject{Published: &now}})
	require.Equal(t, now, got)
}

func TestActivityProcessor_DetectLanguageFromContent_Branches(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	require.Equal(t, "ja", ap.detectLanguageFromContent("こんにちは"))
	require.Equal(t, "zh", ap.detectLanguageFromContent("\u9FFF\u9FFF\u9FFF\u9FFF\u9FFF\u9FFF"))
	require.Equal(t, "ko", ap.detectLanguageFromContent("안녕하세요"))
	require.Equal(t, "ar", ap.detectLanguageFromContent("مرحبا"))
	require.Equal(t, "ru", ap.detectLanguageFromContent("Привет"))

	require.Equal(t, "es", ap.detectLanguageFromContent(" el la de "))
	require.Equal(t, "fr", ap.detectLanguageFromContent(" le la de "))
	require.Equal(t, "de", ap.detectLanguageFromContent(" der die und "))
	require.Equal(t, "pt", ap.detectLanguageFromContent(" o a de "))
	require.Equal(t, "it", ap.detectLanguageFromContent(" il la di "))

	require.Equal(t, "en", ap.detectLanguageFromContent("plain english content"))
}
