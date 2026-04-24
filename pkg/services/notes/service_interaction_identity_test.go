package notes

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestServiceInteractionIdentity_RemoteLikeQueuesCanonicalObject(t *testing.T) {
	ctx := context.Background()
	service, _, federation, _, _ := newNotesServiceHarness(t)
	remoteURL := "https://remote.example/users/steward/statuses/interaction-like"
	remoteStatus := remoteInteractionStatus(remoteURL)
	originalRepo := service.noteRepo
	service.noteRepo = &delegatingStatusRepo{
		StatusRepository: originalRepo,
		getStatus: func(ctx context.Context, statusID string) (*models.Status, error) {
			if statusID == remoteStatus.StatusID {
				return remoteStatus, nil
			}
			return originalRepo.GetStatus(ctx, statusID)
		},
	}

	result, err := service.LikeNote(ctx, &LikeNoteCommand{
		StatusID: remoteStatus.StatusID,
		LikerID:  "alice",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, federation.activities, 1)
	activity := federation.activities[0]
	assert.Equal(t, activitypub.LikeType, activity.Type)
	assert.Equal(t, "https://example.com/users/alice", activity.Actor)
	assert.Equal(t, remoteURL, activity.Object)
	assert.Contains(t, activity.To, remoteStatus.AuthorID)
}

func TestServiceInteractionIdentity_RemoteAnnounceStoresAndQueuesCanonicalObject(t *testing.T) {
	ctx := context.Background()
	service, _, federation, _, _ := newNotesServiceHarness(t)
	remoteURL := "https://remote.example/users/steward/statuses/interaction-announce"
	remoteStatus := remoteInteractionStatus(remoteURL)
	originalRepo := service.noteRepo
	service.noteRepo = &delegatingStatusRepo{
		StatusRepository: originalRepo,
		getStatus: func(ctx context.Context, statusID string) (*models.Status, error) {
			if statusID == remoteStatus.StatusID {
				return remoteStatus, nil
			}
			return originalRepo.GetStatus(ctx, statusID)
		},
	}

	result, err := service.ReblogNote(ctx, &ReblogNoteCommand{
		StatusID:    remoteStatus.StatusID,
		RebloggerID: "bob",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Announce)
	assert.Equal(t, remoteURL, result.Announce.Object)
	require.Len(t, federation.activities, 1)
	activity := federation.activities[0]
	assert.Equal(t, activitypub.AnnounceType, activity.Type)
	assert.Equal(t, "https://example.com/users/bob", activity.Actor)
	assert.Equal(t, remoteURL, activity.Object)
	assert.Contains(t, activity.To, remoteStatus.AuthorID)
}

func TestServiceInteractionIdentity_QueuesUndoLikeAndUndoAnnounceWithCanonicalObject(t *testing.T) {
	now := time.Now().UTC()
	remoteURL := "https://remote.example/users/steward/statuses/interaction-undo"
	remoteStatus := remoteInteractionStatus(remoteURL)
	identity := statusInteractionIdentity{
		localStatusID:    remoteStatus.StatusID,
		activityObjectID: remoteURL,
		actorURL:         "https://example.com/users/alice",
		targetActorID:    remoteStatus.AuthorID,
		statusAuthorID:   remoteStatus.AuthorUsername,
	}

	t.Run("undo like embeds stored like activity", func(t *testing.T) {
		federation := &stubFederation{}
		service := &Service{federation: federation, logger: zap.NewNop()}
		like := &models.Like{
			ID:        "https://example.com/users/alice/activities/like-1",
			Actor:     identity.actorURL,
			Object:    remoteURL,
			Published: now,
		}

		service.queueUndoLikeActivity(context.Background(), remoteStatus, like, identity)

		require.Len(t, federation.activities, 1)
		undo := federation.activities[0]
		assert.Equal(t, activitypub.UndoType, undo.Type)
		assert.Equal(t, identity.actorURL, undo.Actor)
		assert.Contains(t, undo.To, remoteStatus.AuthorID)
		original, ok := undo.Object.(*activitypub.Activity)
		require.True(t, ok)
		assert.Equal(t, activitypub.LikeType, original.Type)
		assert.Equal(t, like.ID, original.ID)
		assert.Equal(t, remoteURL, original.Object)
	})

	t.Run("undo announce embeds stored announce activity", func(t *testing.T) {
		federation := &stubFederation{}
		service := &Service{federation: federation, logger: zap.NewNop()}
		announce := &storage.Announce{
			ID:        "https://example.com/users/alice/activities/announce-1",
			Actor:     identity.actorURL,
			Object:    remoteURL,
			Published: now,
		}

		service.queueUndoAnnounceActivity(context.Background(), remoteStatus, announce, identity)

		require.Len(t, federation.activities, 1)
		undo := federation.activities[0]
		assert.Equal(t, activitypub.UndoType, undo.Type)
		assert.Equal(t, identity.actorURL, undo.Actor)
		assert.Contains(t, undo.To, remoteStatus.AuthorID)
		original, ok := undo.Object.(*activitypub.Activity)
		require.True(t, ok)
		assert.Equal(t, activitypub.AnnounceType, original.Type)
		assert.Equal(t, announce.ID, original.ID)
		assert.Equal(t, remoteURL, original.Object)
	})
}

func TestServiceInteractionIdentity_RemoteUnreblogQueuesUndoAnnounce(t *testing.T) {
	ctx := context.Background()
	service, _, federation, _, _ := newNotesServiceHarness(t)
	remoteURL := "https://remote.example/users/steward/statuses/interaction-unreblog"
	remoteStatus := remoteInteractionStatus(remoteURL)
	originalRepo := service.noteRepo
	service.noteRepo = &delegatingStatusRepo{
		StatusRepository: originalRepo,
		getStatus: func(ctx context.Context, statusID string) (*models.Status, error) {
			if statusID == remoteStatus.StatusID {
				return remoteStatus, nil
			}
			return originalRepo.GetStatus(ctx, statusID)
		},
	}

	actorURL := "https://example.com/users/bob"
	announce := &storage.Announce{
		ID:        "https://example.com/users/bob/activities/announce-1",
		Actor:     actorURL,
		Object:    remoteURL,
		Published: time.Now().UTC(),
	}
	socialRepo, ok := service.socialRepo.(*stubSocialRepo)
	require.True(t, ok)
	socialRepo.announces = map[string]*storage.Announce{
		actorURL + "|" + remoteURL: announce,
	}

	result, err := service.UnreblogNote(ctx, &UnreblogNoteCommand{
		StatusID:      remoteStatus.StatusID,
		UnrebloggerID: "bob",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, federation.activities, 1)
	undo := federation.activities[0]
	assert.Equal(t, activitypub.UndoType, undo.Type)
	original, ok := undo.Object.(*activitypub.Activity)
	require.True(t, ok)
	assert.Equal(t, activitypub.AnnounceType, original.Type)
	assert.Equal(t, announce.ID, original.ID)
	assert.Equal(t, remoteURL, original.Object)
}

func TestServiceInteractionIdentity_HelperFallbackBranches(t *testing.T) {
	service := &Service{domainName: "example.com", logger: zap.NewNop()}

	assert.Empty(t, service.accountActorURL(nil))
	assert.Empty(t, service.accountActorURL(&storage.Account{}))
	assert.Empty(t, service.accountActorURL(&storage.Account{User: &storage.User{Username: "   "}}))
	assert.Equal(t,
		"https://example.com/users/carol",
		service.accountActorURL(&storage.Account{User: &storage.User{Username: " carol "}}),
	)

	assert.Empty(t, service.statusAuthorActorID(nil))
	assert.Equal(t,
		"https://example.com/users/dana",
		service.statusAuthorActorID(&models.Status{AuthorID: " https://example.com/users/dana "}),
	)
	assert.Equal(t,
		"https://example.com/users/erin",
		service.statusAuthorActorID(&models.Status{AuthorUsername: " erin "}),
	)
	assert.Empty(t, service.statusAuthorActorID(&models.Status{AuthorUsername: "erin@remote.example"}))

	identity, err := service.statusInteractionIdentity(
		&models.Status{
			StatusID:       "local-1",
			AuthorID:       "https://example.com/users/frank",
			AuthorUsername: "",
		},
		&storage.Account{User: &storage.User{Username: "grace"}},
	)
	require.NoError(t, err)
	assert.Equal(t, "local-1", identity.localStatusID)
	assert.Equal(t, "https://example.com/users/grace", identity.actorURL)
	assert.Equal(t, "https://example.com/users/frank", identity.targetActorID)
	assert.Equal(t, "https://example.com/users/frank", identity.statusAuthorID)
}

func TestServiceInteractionIdentity_UndoActivityFallbackBranches(t *testing.T) {
	service := &Service{logger: zap.NewNop()}
	status := remoteInteractionStatus("https://remote.example/users/steward/statuses/fallback")
	identity := statusInteractionIdentity{
		activityObjectID: status.Note.ID,
		actorURL:         "https://example.com/users/alice",
		targetActorID:    status.AuthorID,
	}

	likeActivity := service.likeActivityForUndo(&models.Like{ID: "like-1"}, identity)
	require.NotNil(t, likeActivity)
	assert.Equal(t, activitypub.LikeType, likeActivity.Type)
	assert.Equal(t, identity.actorURL, likeActivity.Actor)
	assert.Equal(t, identity.activityObjectID, likeActivity.Object)
	assert.Contains(t, likeActivity.To, status.AuthorID)
	assert.NotNil(t, likeActivity.Published)
	assert.Nil(t, service.likeActivityForUndo(&models.Like{ID: "like-without-object"}, statusInteractionIdentity{}))

	announceActivity := service.announceActivityForUndo(
		status,
		&storage.Announce{ID: "announce-1", Object: identity.activityObjectID},
		identity,
	)
	require.NotNil(t, announceActivity)
	assert.Equal(t, activitypub.AnnounceType, announceActivity.Type)
	assert.Equal(t, identity.actorURL, announceActivity.Actor)
	assert.Equal(t, identity.activityObjectID, announceActivity.Object)
	assert.Contains(t, announceActivity.To, status.AuthorID)
	assert.Contains(t, announceActivity.CC, "https://remote.example/users/steward/followers")
	assert.NotNil(t, announceActivity.Published)
	assert.Nil(t, service.announceActivityForUndo(status, &storage.Announce{ID: "announce-without-object"}, identity))
}

func remoteInteractionStatus(remoteURL string) *models.Status {
	now := time.Now().UTC()
	return &models.Status{
		StatusID:       models.CanonicalStatusIDForDomain(remoteURL, "example.com"),
		AuthorID:       "https://remote.example/users/steward",
		AuthorUsername: "steward@remote.example",
		Visibility:     models.VisibilityPublic,
		ToRecipients:   []string{activitypub.PublicAddress},
		CcRecipients:   []string{"https://remote.example/users/steward/followers"},
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   remoteURL,
				Type: activitypub.NoteType,
				To:   []string{activitypub.PublicAddress},
				CC:   []string{"https://remote.example/users/steward/followers"},
			},
			AttributedTo: "https://remote.example/users/steward",
			Content:      "remote seed",
			Visibility:   models.VisibilityPublic,
		},
	}
}
