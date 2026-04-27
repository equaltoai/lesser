package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityProcessor_AnnounceHelpers_Branches(t *testing.T) {
	ctx := context.Background()

	objectRepo := testmocks.NewMockObjectRepository()
	objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{ID: "https://example.com/objects/1", Content: "local"}, nil).Once()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

	ap := &ActivityProcessor{
		logger:     zap.NewNop(),
		objectRepo: objectRepo,
	}

	require.Equal(t, "https://example.com/objects/1", ap.extractAnnouncedID(&activitypub.Activity{Object: "https://example.com/objects/1"}))
	require.Equal(t, "https://example.com/objects/2", ap.extractAnnouncedID(&activitypub.Activity{Object: map[string]any{"id": "https://example.com/objects/2"}}))
	require.Empty(t, ap.extractAnnouncedID(&activitypub.Activity{Object: 123}))

	content, author := ap.getAnnouncedContent(ctx, &activitypub.Activity{Actor: "https://example.com/users/alice"}, "https://example.com/objects/1")
	require.Equal(t, "local", content)
	require.Empty(t, author)

	// Helper fallbacks.
	require.Equal(t, "Object", ap.extractObjectType(map[string]any{}))
	require.Equal(t, "https://example.com/users/alice", ap.extractObjectAuthor(map[string]any{"attributedTo": "https://example.com/users/alice"}))
	require.Empty(t, ap.extractObjectAuthor(map[string]any{}))

	ap.storeObjectWithLogging(ctx, &models.Object{ID: "https://example.com/objects/3"}, "https://example.com/objects/3", "Object")

	objectRepo.AssertExpectations(t)
}

func TestActivityProcessor_ExtractLanguage_SummaryHint(t *testing.T) {
	ap := &ActivityProcessor{logger: zap.NewNop()}

	require.Equal(t, "es", ap.extractLanguage(&activitypub.Note{BaseObject: activitypub.BaseObject{Summary: "[lang:es] hola"}}))
}

func TestActivityProcessor_GetFollowers_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("actor retrieval fails", func(t *testing.T) {
		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("GetActor", mock.Anything, "alice").Return((*activitypub.Actor)(nil), errors.New("boom")).Once()

		ap := &ActivityProcessor{
			logger:    zap.NewNop(),
			actorRepo: actorRepo,
		}

		_, err := ap.getFollowers(ctx, "alice")
		require.Error(t, err)
		actorRepo.AssertExpectations(t)
	})

	t.Run("followers query fails", func(t *testing.T) {
		actorRepo := testmocks.NewMockActorRepository()
		actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil).Once()

		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string(nil), "", errors.New("boom")).Once()

		ap := &ActivityProcessor{
			logger:           zap.NewNop(),
			actorRepo:        actorRepo,
			relationshipRepo: relationshipRepo,
		}

		_, err := ap.getFollowers(ctx, "alice")
		require.Error(t, err)

		actorRepo.AssertExpectations(t)
		relationshipRepo.AssertExpectations(t)
	})
}

func TestContainsFollowersAddress_FalseCase(t *testing.T) {
	// When no address contains /followers, the function returns false.
	require.False(t, containsFollowersAddress(
		[]string{"https://example.com/users/alice"},
		[]string{"https://www.w3.org/ns/activitystreams#Public"},
	))
	// Full true path already exercised by fanout tests; this ensures the false return is covered.
	require.True(t, containsFollowersAddress(
		[]string{"https://example.com/users/alice/followers"},
	))
}
