package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityProcessor_DetermineVisibility(t *testing.T) {
	ap := &ActivityProcessor{}

	public := "https://www.w3.org/ns/activitystreams#Public"
	require.Equal(t, "direct", ap.determineVisibility(nil, nil))
	require.Equal(t, "public", ap.determineVisibility([]string{public}, nil))
	require.Equal(t, "unlisted", ap.determineVisibility(nil, []string{public}))
	require.Equal(t, "direct", ap.determineVisibility([]string{"https://example.com/followers"}, nil))
}

func TestActivityProcessor_LanguageDetection(t *testing.T) {
	ap := &ActivityProcessor{}

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{Summary: "[lang:fr] bonjour"},
		Content:    "<p>Hello</p>",
	}
	require.Equal(t, "fr", ap.extractLanguage(note))

	note = &activitypub.Note{Content: "<p>こんにちは世界</p>"}
	require.Equal(t, "ja", ap.extractLanguage(note))

	require.Equal(t, "en", ap.detectLanguageFromContent(""))
	require.Equal(t, "es", ap.detectLanguageFromContent(" el la de "))
}

func TestActivityProcessor_FanOutToTimelines_PublicLocalWithFollowers(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
	actorRepo := testmocks.NewMockActorRepository()
	relationshipRepo := testmocks.NewMockRelationshipRepository()

	baseURL := "https://example.com"
	username := "alice"
	actorID := baseURL + "/users/" + username

	actorRepo.On("GetActor", mock.Anything, username).Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: actorID}}, nil)
	relationshipRepo.On("GetFollowers", mock.Anything, username, 1000, "").Return([]string{"bob", "carol"}, "", nil)

	var gotEntries int
	timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		entries := args.Get(1).([]*models.Timeline)
		gotEntries = len(entries)
	}).Return(nil)

	ap := &ActivityProcessor{
		db:               mockDB,
		logger:           zap.NewNop(),
		timelineRepo:     timelineRepo,
		actorRepo:        actorRepo,
		relationshipRepo: relationshipRepo,
		baseURL:          baseURL,
	}

	content := strings.Repeat("x", 600)
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   baseURL + "/activities/1",
			Type: activitypub.CreateType,
		},
		Actor: actorID,
		Object: map[string]any{
			"id":      baseURL + "/objects/1",
			"type":    "Note",
			"content": content,
			"to":      []any{"https://www.w3.org/ns/activitystreams#Public"},
		},
	}

	err := ap.fanOutToTimelines(ctx, activity, username)
	require.NoError(t, err)
	require.Equal(t, 5, gotEntries)
}

func TestActivityProcessor_ProcessMapObject_RemoteFallbackWhenActorLookupFails(t *testing.T) {
	ctx := context.Background()

	actorRepo := testmocks.NewMockActorRepository()
	actorRepo.On("GetActor", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	ap := &ActivityProcessor{
		logger:    zap.NewNop(),
		actorRepo: actorRepo,
		baseURL:   "https://example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "id", Type: activitypub.CreateType},
		Actor:      "https://example.com/users/alice",
		Object: map[string]any{
			"id":      "https://remote.example/objects/1",
			"type":    "Note",
			"content": "remote",
		},
	}

	obj, err := ap.processActivityObject(ctx, activity)
	require.NoError(t, err)
	require.NotNil(t, obj)
	require.Equal(t, "https://remote.example/objects/1", obj.ObjectID)
	require.NotEmpty(t, obj.Content)
}

func TestActivityProcessor_RecordMetric_IgnoresCreateErrors(t *testing.T) {
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

	require.NotPanics(t, func() {
		ap.recordMetric(ctx, "TEST", "TestMetric", "key", time.Minute, map[string]any{"x": "y"}, nil)
	})
}
