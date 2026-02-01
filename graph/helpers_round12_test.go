package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/services/hashtags"
	"github.com/equaltoai/lesser/pkg/services/severance"
	"github.com/equaltoai/lesser/pkg/services/threads"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12Helpers_UtilityFunctions(t *testing.T) {
	now := time.Now().UTC()
	zero := time.Time{}

	require.Nil(t, firstNonZeroTime(nil))
	require.Nil(t, firstNonZeroTime(&zero))
	require.NotNil(t, firstNonZeroTime(&zero, &now))

	require.Equal(t, "", deriveUsernameFromIRI(""))
	require.Equal(t, "alice", deriveUsernameFromIRI("https://example.com/users/alice"))
	require.Equal(t, "alice", deriveUsernameFromIRI("https://example.com/users/alice/"))
	require.Equal(t, "alice", deriveUsernameFromIRI("https://example.com/users/alice.json"))
	require.Equal(t, "alice", deriveUsernameFromIRI("@alice"))
	require.Equal(t, "alice", deriveUsernameFromIRI("https://example.com/users/alice?x=1"))

	id := generateID()
	require.Len(t, id, 32)

	require.True(t, notificationMatchesTypes(&model.Notification{Type: "mention"}, nil))
	require.True(t, notificationMatchesTypes(&model.Notification{Type: "mention"}, []string{}))
	require.True(t, notificationMatchesTypes(&model.Notification{Type: "mention"}, []string{"follow", "mention"}))
	require.False(t, notificationMatchesTypes(&model.Notification{Type: "mention"}, []string{"follow"}))

	require.Equal(t, "", getUsernameFromContext(context.Background()))

	ctxClaims := context.WithValue(context.Background(), common.ContextKeyClaims, round12TestClaims{username: "alice"})
	require.Equal(t, "alice", getUsernameFromContext(ctxClaims))

	ctxAuthClaims := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{Username: "bob"})
	require.Equal(t, "bob", getUsernameFromContext(ctxAuthClaims))
	require.Equal(t, "bob", GetUserID(ctxAuthClaims))
}

func TestRound12Helpers_SocialAndDrivers(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	// executeSocialAction: auth required.
	_, err := mut.executeSocialAction(context.Background(), "o1", "Like", "like", func(context.Context, string, string) error {
		return nil
	})
	require.Error(t, err)

	// executeSocialAction: service error path.
	_, err = mut.executeSocialAction(round12AuthContext("alice"), "o1", "Like", "like", func(context.Context, string, string) error {
		return assertError("boom")
	})
	require.Error(t, err)

	// executeSocialAction: success path.
	act, err := mut.executeSocialAction(round12AuthContext("alice"), "o1", "Like", "like", func(context.Context, string, string) error {
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, act)

	// executeSocialUndo: auth required and success path.
	_, err = mut.executeSocialUndo(context.Background(), "o1", "unlike", func(context.Context, string, string) error {
		return nil
	})
	require.Error(t, err)

	ok, err := mut.executeSocialUndo(round12AuthContext("alice"), "o1", "unlike", func(context.Context, string, string) error {
		return nil
	})
	require.NoError(t, err)
	require.True(t, ok)

	// buildAndSortDrivers and createReadWriteDrivers.
	q := &queryResolver{resolver}
	drivers := q.buildAndSortDrivers([]*cost.Driver{
		{Service: "A", PercentageOfTotal: 10},
		{Service: "B", PercentageOfTotal: 50},
		{Service: "C", PercentageOfTotal: 20},
	})
	require.Len(t, drivers, 3)
	require.Equal(t, "B", drivers[0].Service)

	rw := q.createReadWriteDrivers(10, 5, 1.0)
	require.NotEmpty(t, rw)

	require.Equal(t, "STABLE", q.calculateTrend(10, 0))
	require.Equal(t, "INCREASING", q.calculateTrend(12, 10))
	require.Equal(t, "DECREASING", q.calculateTrend(8, 10))
	require.Equal(t, "STABLE", q.calculateTrend(10.5, 10))
}

func TestRound12Helpers_ThreadAndHashtagConversions(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)

	// Thread context conversion covers sync-status mapping and minimal-note fallback.
	root := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://localhost/statuses/1",
			Type:      activitypub.NoteType,
			Sensitive: true,
		},
		AttributedTo: "https://localhost/users/alice",
		Content:      "hello",
	}
	threadCtx := resolver.convertThreadContextResultToModel(context.Background(), &threads.ThreadContextResult{
		RootNote:         root,
		ReplyCount:       2,
		ParticipantCount: 1,
		MissingCount:     0,
		LastActivity:     time.Now(),
		SyncStatus:       threads.SyncStatusComplete,
	})
	require.NotNil(t, threadCtx)
	require.NotNil(t, threadCtx.RootNote)

	minimal := resolver.buildMinimalThreadNote(root)
	require.NotNil(t, minimal)
	require.NotNil(t, minimal.Actor)
	require.Equal(t, model.VisibilityPublic, minimal.Visibility)

	// Hashtag conversion helpers.
	settings := &storage.HashtagNotificationSettings{
		Level: "mutuals",
		Muted: true,
		Filters: []*storage.NotificationFilter{
			{Types: []string{"mention"}, ExcludeTypes: []string{"follow"}},
		},
	}
	h := &hashtags.Hashtag{
		Name:                 "tag",
		PostCount:            1,
		FollowerCount:        2,
		TrendingScore:        3.0,
		IsFollowing:          true,
		Related:              []string{"a", "", "b"},
		FollowedAt:           ptrTime(time.Now().Add(-time.Minute)),
		NotificationSettings: settings,
	}
	hashtagModel := resolver.convertHashtagToModel(context.Background(), h, "alice")
	require.NotNil(t, hashtagModel)
	require.NotNil(t, hashtagModel.NotificationSettings)
	require.NotNil(t, hashtagModel.FollowedAt)
	require.Len(t, hashtagModel.RelatedHashtags, 2)

	// Storage-backed fetch falls back to default on missing settings.
	require.NotNil(t, resolver.fetchHashtagNotificationSettings(context.Background(), "", ""))
	require.NotNil(t, resolver.fetchHashtagNotificationSettings(context.Background(), "tag", "alice"))

	// Actor lookup used by affected relationship conversion.
	actorRepo := storageRepo.Actor()
	require.NoError(t, actorRepo.CreateActor(context.Background(), &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://localhost/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}, ""))

	aff := resolver.convertAffectedRelationshipToModel(context.Background(), &severance.AffectedRelationship{
		ActorID:          "alice",
		ActorHandle:      "@alice@localhost",
		ActorDomain:      "localhost",
		RelationshipType: "follow",
		EstablishedAt:    time.Now().Add(-time.Hour),
	})
	require.NotNil(t, aff)
	require.NotNil(t, aff.Actor)
}

func TestRound12Helpers_SeveranceAndActorConstruction(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	sev := resolver.convertSeveredRelationshipToModel(context.Background(), &severance.SeveredRelationship{
		ID:                "sev-1",
		LocalInstance:     "localhost",
		RemoteInstance:    "example.com",
		Reason:            storageModels.SeveranceReasonDomainBlock,
		AffectedFollowers: 1,
		AffectedFollowing: 2,
		DetectedAt:        time.Now(),
		Reversible:        true,
		Details:           "details",
		AdminNotes:        "notes",
		AutoDetected:      true,
	})
	require.NotNil(t, sev)
	require.NotNil(t, sev.Details)
	require.NotNil(t, sev.Details.AdminNotes)

	actor := resolver.constructMinimalActor("", "@bob@example.com", "example.com")
	require.NotNil(t, actor)
	require.Equal(t, "bob", actor.PreferredUsername)

	actorUnknown := resolver.constructMinimalActor("carol", "", "")
	require.NotNil(t, actorUnknown)
	require.NotEmpty(t, actorUnknown.Inbox)
}

type assertError string

func (e assertError) Error() string { return string(e) }

func ptrTime(t time.Time) *time.Time { return &t }
