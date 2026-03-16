package services

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recordingUserRepo struct {
	calls []*activitypub.Activity
	err   error
}

func (r *recordingUserRepo) FanOutPost(_ context.Context, activity *activitypub.Activity) error {
	r.calls = append(r.calls, activity)
	return r.err
}

type recordingTimelineRepo struct {
	calls []string
	err   error
}

func (r *recordingTimelineRepo) RemoveFromTimelines(_ context.Context, objectID string) error {
	r.calls = append(r.calls, objectID)
	return r.err
}

type capturingNotificationRepo struct {
	created []*models.Notification
	err     error
}

func (r *capturingNotificationRepo) CreateNotification(_ context.Context, notification *models.Notification) error {
	r.created = append(r.created, notification)
	return r.err
}

func (r *capturingNotificationRepo) DeleteNotificationsByObject(context.Context, string) error {
	return nil
}

type storageWithInstanceRelationships struct {
	StorageAdapter
	resp *model.InstanceRelations
	err  error
}

func (s storageWithInstanceRelationships) GetInstanceRelationships(context.Context, string) (*model.InstanceRelations, error) {
	return s.resp, s.err
}

type analyticsStorageStub struct {
	StorageAdapter

	recordActivityCalls []recordActivityCall
	recordActivityErr   error

	recordHashtagCalls []recordHashtagCall
	errByHashtag       map[string]error

	recordLinkCalls []recordLinkCall
	errByLink       map[string]error

	engagementCalls []recordEngagementCall
	engagementErr   error

	instanceActivityCalls []recordInstanceActivityCall
	instanceActivityErr   error

	infraResp *model.InfrastructureStatus
	infraErr  error
}

type recordActivityCall struct {
	activityType string
	actorID      string
	timestamp    time.Time
}

func (s *analyticsStorageStub) RecordActivity(_ context.Context, activityType, actorID string, timestamp time.Time) error {
	s.recordActivityCalls = append(s.recordActivityCalls, recordActivityCall{
		activityType: activityType,
		actorID:      actorID,
		timestamp:    timestamp,
	})
	return s.recordActivityErr
}

type recordHashtagCall struct {
	hashtag  string
	objectID string
	actorID  string
}

func (s *analyticsStorageStub) RecordHashtagUsage(_ context.Context, hashtag, objectID, actorID string) error {
	s.recordHashtagCalls = append(s.recordHashtagCalls, recordHashtagCall{hashtag: hashtag, objectID: objectID, actorID: actorID})
	if err := s.errByHashtag[hashtag]; err != nil {
		return err
	}
	return nil
}

type recordLinkCall struct {
	link     string
	objectID string
	actorID  string
}

func (s *analyticsStorageStub) RecordLinkShare(_ context.Context, link, objectID, actorID string) error {
	s.recordLinkCalls = append(s.recordLinkCalls, recordLinkCall{link: link, objectID: objectID, actorID: actorID})
	if err := s.errByLink[link]; err != nil {
		return err
	}
	return nil
}

type recordEngagementCall struct {
	objectID       string
	engagementType string
	actorID        string
}

func (s *analyticsStorageStub) RecordStatusEngagement(_ context.Context, objectID, engagementType, actorID string) error {
	s.engagementCalls = append(s.engagementCalls, recordEngagementCall{objectID: objectID, engagementType: engagementType, actorID: actorID})
	return s.engagementErr
}

type recordInstanceActivityCall struct {
	activityType string
	timestamp    time.Time
}

func (s *analyticsStorageStub) RecordInstanceActivity(_ context.Context, activityType string, timestamp time.Time) error {
	s.instanceActivityCalls = append(s.instanceActivityCalls, recordInstanceActivityCall{activityType: activityType, timestamp: timestamp})
	return s.instanceActivityErr
}

func (s *analyticsStorageStub) GetInfrastructureHealth(context.Context) (*model.InfrastructureStatus, error) {
	return s.infraResp, s.infraErr
}

type noopRepositoryStorage struct {
	storagecore.RepositoryStorage
}

type fakeAuthService struct {
	lastToken string
	user      *UserContext
	err       error
}

func (f *fakeAuthService) AuthenticateUser(_ context.Context, token string) (*UserContext, error) {
	f.lastToken = token
	return f.user, f.err
}

func (f *fakeAuthService) ValidateScope(*UserContext, string) error {
	return nil
}

func TestValidationService_round27_coverage(t *testing.T) {
	t.Parallel()

	svc := NewValidationService(&ServiceConfig{})

	t.Run("ValidateCreatePost_content_required", func(t *testing.T) {
		err := svc.ValidateCreatePost(&CreatePostInput{Content: ""})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 400, serr.Status)
		assert.Equal(t, "Post content is required and must not exceed 500 characters", serr.Message)
	})

	t.Run("ValidateCreatePost_visibility_invalid", func(t *testing.T) {
		err := svc.ValidateCreatePost(&CreatePostInput{Content: "hi", Visibility: "nope"})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Invalid visibility setting", serr.Message)
	})

	t.Run("ValidateCreatePost_media_invalid", func(t *testing.T) {
		err := svc.ValidateCreatePost(&CreatePostInput{Content: "hi", MediaIDs: []string{""}})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Invalid media attachments", serr.Message)
	})

	t.Run("ValidateCreatePost_valid", func(t *testing.T) {
		err := svc.ValidateCreatePost(&CreatePostInput{Content: "hi", Visibility: "public"})
		require.NoError(t, err)
	})

	t.Run("ValidateFollowInput_missing_target", func(t *testing.T) {
		err := svc.ValidateFollowInput(&FollowInput{TargetActorID: ""})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Target actor ID is required", serr.Message)
	})

	t.Run("ValidateFollowInput_trimmed_target_empty", func(t *testing.T) {
		err := svc.ValidateFollowInput(&FollowInput{TargetActorID: "   "})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Target actor ID cannot be empty", serr.Message)
	})

	t.Run("ValidateLikeInput_missing_object", func(t *testing.T) {
		err := svc.ValidateLikeInput(&LikeInput{ObjectID: ""})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Object ID is required", serr.Message)
	})

	t.Run("ValidateDeletePost_trimmed_object_empty", func(t *testing.T) {
		err := svc.ValidateDeletePost(&DeletePostInput{ObjectID: " \t "})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Object ID cannot be empty", serr.Message)
	})

	t.Run("ValidateUpdatePost_missing_object", func(t *testing.T) {
		err := svc.ValidateUpdatePost(&UpdatePostInput{ObjectID: ""})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Object ID is required", serr.Message)
	})

	t.Run("ValidateUpdatePost_content_too_long", func(t *testing.T) {
		long := make([]byte, 501)
		for i := range long {
			long[i] = 'a'
		}
		err := svc.ValidateUpdatePost(&UpdatePostInput{ObjectID: "o1", Content: string(long)})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Post content must not exceed 500 characters", serr.Message)
	})

	t.Run("ValidateUpdatePost_visibility_invalid", func(t *testing.T) {
		err := svc.ValidateUpdatePost(&UpdatePostInput{ObjectID: "o1", Visibility: "nope"})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, "Invalid visibility setting", serr.Message)
	})

	t.Run("ValidateUpdatePost_valid", func(t *testing.T) {
		err := svc.ValidateUpdatePost(&UpdatePostInput{ObjectID: "o1", Content: "hello", Visibility: "unlisted"})
		require.NoError(t, err)
	})
}

func TestTimelineService_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	userRepo := &recordingUserRepo{}
	timelineRepo := &recordingTimelineRepo{}
	storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
		user:     userRepo,
		timeline: timelineRepo,
		logger:   zap.NewNop(),
		table:    "tbl",
	}}

	svc := &timelineService{storage: storage, logger: zap.NewNop()}

	t.Run("FanOutToFollowers_ignores_non_create", func(t *testing.T) {
		err := svc.FanOutToFollowers(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.LikeType}}, nil)
		require.NoError(t, err)
		assert.Empty(t, userRepo.calls)
	})

	t.Run("FanOutToFollowers_uses_actor_username", func(t *testing.T) {
		userRepo.calls = nil
		err := svc.FanOutToFollowers(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}}, &activitypub.Actor{PreferredUsername: "alice"})
		require.NoError(t, err)
		require.Len(t, userRepo.calls, 1)
	})

	t.Run("FanOutToFollowers_parses_username_from_actor_id_users", func(t *testing.T) {
		userRepo.calls = nil
		err := svc.FanOutToFollowers(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}, Actor: "https://example.com/users/bob"}, nil)
		require.NoError(t, err)
		require.Len(t, userRepo.calls, 1)
	})

	t.Run("FanOutToFollowers_parses_username_from_actor_id_at", func(t *testing.T) {
		userRepo.calls = nil
		err := svc.FanOutToFollowers(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}, Actor: "https://example.com/@carol"}, nil)
		require.NoError(t, err)
		require.Len(t, userRepo.calls, 1)
	})

	t.Run("FanOutToFollowers_missing_username_is_noop", func(t *testing.T) {
		userRepo.calls = nil
		err := svc.FanOutToFollowers(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}, Actor: "https://example.com/actor/carol"}, nil)
		require.NoError(t, err)
		assert.Empty(t, userRepo.calls)
	})

	t.Run("UpdateTimelines_routes_by_activity_type", func(t *testing.T) {
		userRepo.calls = nil
		timelineRepo.calls = nil

		err := svc.UpdateTimelines(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.CreateType}, Actor: "https://example.com/users/alice"})
		require.NoError(t, err)
		require.Len(t, userRepo.calls, 1)

		err = svc.UpdateTimelines(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1"}},
		})
		require.NoError(t, err)

		err = svc.UpdateTimelines(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.LikeType}, Object: "note-1"})
		require.NoError(t, err)

		err = svc.UpdateTimelines(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType}, Object: "note-1"})
		require.NoError(t, err)
		assert.Len(t, timelineRepo.calls, 3)
	})

	t.Run("RemoveFromTimelines_attempts_all_standard_timelines", func(t *testing.T) {
		timelineRepo.calls = nil
		timelineRepo.err = stderrors.New("boom")

		err := svc.RemoveFromTimelines(ctx, "obj-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1", "obj-1", "obj-1"}, timelineRepo.calls)
	})
}

func TestNotificationService_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("CreateFollowNotification_invalid_object_is_noop", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateFollowNotification(ctx, &activitypub.Activity{Object: map[string]any{}, Actor: "https://example.com/users/bob"})
		require.NoError(t, err)
		assert.Empty(t, captureRepo.created)
	})

	t.Run("CreateFollowNotification_success", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateFollowNotification(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a1"},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://local.example/users/alice",
		})
		require.NoError(t, err)
		require.Len(t, captureRepo.created, 1)
		assert.Equal(t, "follow", captureRepo.created[0].Type)
		assert.Equal(t, "alice", captureRepo.created[0].UserID)
		assert.Equal(t, "bob", captureRepo.created[0].ActorID)
	})

	t.Run("CreateLikeNotification_swallows_storage_errors", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			object:       fakeObjectRepo{getErr: stderrors.New("boom")},
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateLikeNotification(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "like-1"},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://local.example/objects/n1",
		})
		require.NoError(t, err)
		assert.Empty(t, captureRepo.created)
	})

	t.Run("CreateLikeNotification_ignores_self_like", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			object:       fakeObjectRepo{getValue: &activitypub.Note{AttributedTo: "https://remote.example/users/bob"}},
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateLikeNotification(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "like-2"},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://local.example/objects/n1",
		})
		require.NoError(t, err)
		assert.Empty(t, captureRepo.created)
	})

	t.Run("CreateLikeNotification_success", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			object:       fakeObjectRepo{getValue: map[string]any{"attributedTo": "https://local.example/users/alice"}},
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateLikeNotification(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "like-3"},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://local.example/objects/n1",
		})
		require.NoError(t, err)
		require.Len(t, captureRepo.created, 1)
		assert.Equal(t, "favourite", captureRepo.created[0].Type)
		assert.Equal(t, "alice", captureRepo.created[0].UserID)
	})

	t.Run("CreateReblogNotification_success", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			object:       fakeObjectRepo{getValue: map[string]any{"attributedTo": "https://local.example/users/alice"}},
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateReblogNotification(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "announce-1"},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://local.example/objects/n1",
		})
		require.NoError(t, err)
		require.Len(t, captureRepo.created, 1)
		assert.Equal(t, "reblog", captureRepo.created[0].Type)
		assert.Equal(t, "alice", captureRepo.created[0].UserID)
	})

	t.Run("CreateReplyNotification_requires_in_reply_to", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateReplyNotification(ctx, &activitypub.Activity{Object: &activitypub.Note{}})
		require.NoError(t, err)
		assert.Empty(t, captureRepo.created)
	})

	t.Run("CreateReplyNotification_success", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			object:       fakeObjectRepo{getValue: &activitypub.Note{AttributedTo: "https://local.example/users/alice"}},
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateReplyNotification(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "reply-1"},
			Actor:      "https://remote.example/users/bob",
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{InReplyTo: "https://local.example/objects/n1"}},
		})
		require.NoError(t, err)
		require.Len(t, captureRepo.created, 1)
		assert.Equal(t, "reply", captureRepo.created[0].Type)
		assert.Equal(t, "alice", captureRepo.created[0].UserID)
	})

	t.Run("CreateMentionNotification_skips_self_and_continues_on_error", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{err: stderrors.New("boom")}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}
		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		err := svc.CreateMentionNotification(ctx, []string{"https://local.example/users/alice", "https://remote.example/users/bob"}, &activitypub.Activity{Actor: "https://local.example/users/alice"})
		require.NoError(t, err)
		require.Len(t, captureRepo.created, 1)
		assert.Equal(t, "bob", captureRepo.created[0].UserID)
	})

	t.Run("createNotification_handles_reblog_and_custom_types", func(t *testing.T) {
		captureRepo := &capturingNotificationRepo{}
		storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
			notification: captureRepo,
			logger:       zap.NewNop(),
			table:        "tbl",
		}}

		svc := &notificationService{storage: storage, logger: zap.NewNop()}

		require.NoError(t, svc.createNotification(ctx, "https://local.example/users/alice", "https://remote.example/users/bob", "reblog", "obj-1"))
		require.NoError(t, svc.createNotification(ctx, "https://local.example/users/alice", "https://remote.example/users/bob", "custom.type", "obj-1"))
		require.Len(t, captureRepo.created, 2)
		assert.Equal(t, "reblog", captureRepo.created[0].Type)
		assert.Equal(t, "custom.type", captureRepo.created[1].Type)
	})
}

func TestFederationService_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	actor := &activitypub.Actor{PreferredUsername: "alice"}
	var followerCalls []struct {
		username string
		limit    int
		cursor   string
	}

	relationshipRepo := fakeRelationshipRepo{
		getFollowersFn: func(_ context.Context, username string, limit int, cursor string) ([]string, string, error) {
			followerCalls = append(followerCalls, struct {
				username string
				limit    int
				cursor   string
			}{username: username, limit: limit, cursor: cursor})
			return []string{"https://remote/users/bob", "https://remote/users/cat"}, "", nil
		},
	}

	storage := &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{
		actor:        fakeActorRepo{actor: actor},
		relationship: relationshipRepo,
		logger:       zap.NewNop(),
		table:        "tbl",
		rawDB:        nil,
	}}

	svc := &federationService{
		deps: &ServiceDependencies{
			Config: &ServiceConfig{Config: &config.Config{}},
			Logger: zap.NewNop(),
		},
		storage: storage,
		logger:  zap.NewNop(),
	}

	t.Run("DeliverToFollowers_no_db", func(t *testing.T) {
		err := svc.DeliverToFollowers(ctx, &activitypub.Activity{}, actor)
		require.ErrorIs(t, err, ErrNoDatabaseAvailable)
	})

	t.Run("DeliverToRecipients_unsupported_db", func(t *testing.T) {
		withDB := &federationService{
			deps:    svc.deps,
			storage: &repositoryStorageAdapter{repos: fakeStorageAdapterRepos{rawDB: struct{}{}, table: "tbl", logger: zap.NewNop()}},
			logger:  zap.NewNop(),
		}

		err := withDB.DeliverToRecipients(ctx, &activitypub.Activity{}, actor)
		require.ErrorIs(t, err, ErrUnsupportedDatabaseType)
	})

	t.Run("DetermineRecipients_missing_actor_returns_error", func(t *testing.T) {
		_, err := svc.DetermineRecipients(ctx, &activitypub.Activity{}, "public")
		require.ErrorIs(t, err, ErrActivityMissingActor)
	})

	t.Run("DetermineRecipients_invalid_actor_id_is_noop", func(t *testing.T) {
		recipients, err := svc.DetermineRecipients(ctx, &activitypub.Activity{Actor: "https://example.com/actor/unknown"}, "public")
		require.NoError(t, err)
		assert.Empty(t, recipients)
	})

	t.Run("DetermineRecipients_public_includes_followers_mentions_and_to_cc", func(t *testing.T) {
		followerCalls = nil

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{activitypub.PublicAddress, "https://remote/users/eve"},
				CC: []string{"https://remote/users/frank"},
			},
			Actor:  "https://example.com/users/alice",
			Object: &activitypub.Note{Tag: []activitypub.Tag{{Type: "Mention", Href: "https://remote/users/dan", Name: "@dan"}}},
		}
		recipients, err := svc.DetermineRecipients(ctx, activity, "public")
		require.NoError(t, err)
		require.Len(t, followerCalls, 1)
		assert.Equal(t, "alice", followerCalls[0].username)
		assert.Equal(t, 1000, followerCalls[0].limit)
		assert.Equal(t, "", followerCalls[0].cursor)
		assert.ElementsMatch(t, []string{
			"https://remote/users/bob",
			"https://remote/users/cat",
			"https://remote/users/dan",
			"https://remote/users/eve",
			"https://remote/users/frank",
		}, recipients)
	})

	t.Run("DetermineRecipients_direct_uses_mentions_and_to_cc_not_followers", func(t *testing.T) {
		followerCalls = nil

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{"https://remote/users/eve"},
			},
			Actor:  "https://example.com/@alice",
			Object: &activitypub.Article{Note: activitypub.Note{Tag: []activitypub.Tag{{Type: "Mention", Href: "https://remote/users/dan", Name: "@dan"}}}},
		}
		recipients, err := svc.DetermineRecipients(ctx, activity, "direct")
		require.NoError(t, err)
		assert.Empty(t, followerCalls)
		assert.ElementsMatch(t, []string{"https://remote/users/dan", "https://remote/users/eve"}, recipients)
	})

	t.Run("GetInstanceRelationships_propagates_error", func(t *testing.T) {
		errBoom := stderrors.New("boom")
		service := &federationService{
			storage: storageWithInstanceRelationships{StorageAdapter: storage, err: errBoom},
			logger:  zap.NewNop(),
		}
		relationships, err := service.GetInstanceRelationships(ctx, "example.com")
		assert.Nil(t, relationships)
		assert.Equal(t, errBoom, err)
	})
}

func TestAnalyticsService_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stub := &analyticsStorageStub{}
	svc := &analyticsService{storage: stub, logger: zap.NewNop()}

	t.Run("RecordStatusCreation_calls_storage", func(t *testing.T) {
		stub.recordActivityCalls = nil
		ts := time.Date(2025, 1, 1, 2, 3, 4, 0, time.UTC)

		err := svc.RecordStatusCreation(ctx, "https://example.com/users/alice", ts)
		require.NoError(t, err)
		require.Len(t, stub.recordActivityCalls, 1)
		assert.Equal(t, "status", stub.recordActivityCalls[0].activityType)
		assert.Equal(t, "https://example.com/users/alice", stub.recordActivityCalls[0].actorID)
		assert.True(t, stub.recordActivityCalls[0].timestamp.Equal(ts))
	})

	t.Run("RecordHashtagUsage_continues_on_error", func(t *testing.T) {
		stub.recordHashtagCalls = nil
		stub.errByHashtag = map[string]error{"bad": stderrors.New("boom")}

		err := svc.RecordHashtagUsage(ctx, []string{"good", "bad"}, "obj-1", "actor-1")
		require.NoError(t, err)
		assert.Len(t, stub.recordHashtagCalls, 2)
	})

	t.Run("RecordLinkShare_continues_on_error", func(t *testing.T) {
		stub.recordLinkCalls = nil
		stub.errByLink = map[string]error{"https://bad.example": stderrors.New("boom")}

		err := svc.RecordLinkShare(ctx, []string{"https://ok.example", "https://bad.example"}, "obj-1", "actor-1")
		require.NoError(t, err)
		assert.Len(t, stub.recordLinkCalls, 2)
	})

	t.Run("RecordEngagement_propagates_error", func(t *testing.T) {
		stub.engagementErr = stderrors.New("boom")
		err := svc.RecordEngagement(ctx, "obj-1", "like", "actor-1")
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("RecordInstanceActivity_propagates_error", func(t *testing.T) {
		stub.instanceActivityErr = stderrors.New("boom")
		err := svc.RecordInstanceActivity(ctx, "signup", time.Now())
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("GetInfrastructureHealth_propagates_error", func(t *testing.T) {
		stub.infraErr = stderrors.New("boom")
		resp, err := svc.GetInfrastructureHealth(ctx)
		assert.Nil(t, resp)
		assert.Equal(t, "boom", err.Error())
	})
}

func TestAuthenticationService_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("AuthenticateUser_missing_token", func(t *testing.T) {
		svc := &authenticationService{}
		_, err := svc.AuthenticateUser(ctx, "")
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 401, serr.Status)
		assert.Equal(t, "Missing authentication token", serr.Message)
	})

	t.Run("AuthenticateUser_legacy_storage_not_supported", func(t *testing.T) {
		svc := &authenticationService{jwtSecret: "secret", config: &config.Config{}, repos: struct{}{}}
		_, err := svc.AuthenticateUser(ctx, "token")
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 500, serr.Status)
		assert.Equal(t, "OAuth validation not supported for legacy storage", serr.Message)
	})

	t.Run("ValidateScope_requires_claims", func(t *testing.T) {
		svc := &authenticationService{}
		err := svc.ValidateScope(nil, auth.ScopeRead)
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 401, serr.Status)
	})

	t.Run("ValidateScope_forbidden_when_missing_scope", func(t *testing.T) {
		svc := &authenticationService{}
		err := svc.ValidateScope(&UserContext{Claims: &auth.Claims{Scopes: []string{auth.ScopeRead}}}, auth.ScopeWrite)
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 403, serr.Status)
	})

	t.Run("ValidateScope_ok_when_scope_present", func(t *testing.T) {
		svc := &authenticationService{}
		err := svc.ValidateScope(&UserContext{Claims: &auth.Claims{Scopes: []string{auth.ScopeWrite}}}, auth.ScopeWrite)
		require.NoError(t, err)
	})

	t.Run("AuthenticateUserFromHeader_missing_header", func(t *testing.T) {
		_, err := AuthenticateUserFromHeader("", &fakeAuthService{})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 401, serr.Status)
		assert.Equal(t, "Missing authorization header", serr.Message)
	})

	t.Run("AuthenticateUserFromHeader_invalid_header_format", func(t *testing.T) {
		_, err := AuthenticateUserFromHeader("Token abc", &fakeAuthService{})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 401, serr.Status)
		assert.Equal(t, "Invalid authorization header format", serr.Message)
	})

	t.Run("AuthenticateUserFromHeader_success", func(t *testing.T) {
		f := &fakeAuthService{user: &UserContext{Username: "alice"}}
		user, err := AuthenticateUserFromHeader("Bearer abc", f)
		require.NoError(t, err)
		assert.Equal(t, "abc", f.lastToken)
		assert.Equal(t, "alice", user.Username)
	})

	t.Run("ValidateWriteScope_and_follow_scope_helpers", func(t *testing.T) {
		noClaims := &UserContext{}
		require.Error(t, ValidateWriteScope(noClaims))

		writeUser := &UserContext{Claims: &auth.Claims{Scopes: []string{auth.ScopeWrite}}}
		require.NoError(t, ValidateWriteScope(writeUser))
		require.NoError(t, ValidateReadScope(&UserContext{Claims: &auth.Claims{Scopes: []string{auth.ScopeRead}}}))

		require.NoError(t, ValidateFollowScope(&UserContext{Claims: &auth.Claims{Scopes: []string{"write:follows"}}}))
		require.NoError(t, ValidateFollowScope(writeUser))

		err := ValidateFollowScope(&UserContext{Claims: &auth.Claims{Scopes: []string{auth.ScopeRead}}})
		serr := new(ServiceError)
		require.ErrorAs(t, err, &serr)
		assert.Equal(t, 403, serr.Status)
	})
}

func TestServiceFactory_round27_coverage(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	repos := &noopRepositoryStorage{}
	svcCfg := &ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: "secret",
		Config:    &config.Config{},
	}
	factory := NewServiceFactory(repos, svcCfg, zap.NewNop())
	require.NotNil(t, factory.GetServiceDependencies())

	require.NotNil(t, factory.CreateValidationService())
	require.NotNil(t, factory.CreateAuthenticationService())
	require.NotNil(t, factory.CreateFederationService())
	require.NotNil(t, factory.CreateTimelineService())
	require.NotNil(t, factory.CreateAnalyticsService())
	require.NotNil(t, factory.CreateNotificationService())
	require.NotNil(t, factory.CreateBusinessLogicService())
}

func TestAWSS3StorageClient_round27_coverage(t *testing.T) {
	t.Parallel()

	s := &AWSS3StorageClient{bucketName: "bucket", logger: zap.NewNop()}

	assert.Equal(t, "application/zip", s.getContentType("archive.zip"))
	assert.Equal(t, "application/x-tar", s.getContentType("archive.tar"))
	assert.Equal(t, "application/gzip", s.getContentType("archive.tar.gz"))
	assert.Equal(t, "text/csv", s.getContentType("data.csv"))
	assert.Equal(t, "application/json", s.getContentType("data.json"))
	assert.Equal(t, "application/octet-stream", s.getContentType("data.bin"))

	err := s.UploadFile(context.Background(), "", []byte{1})
	require.Error(t, err)

	err = s.UploadFile(context.Background(), "file.json", nil)
	require.ErrorIs(t, err, ErrCannotUploadEmptyData)

	_, err = s.GetFile(context.Background(), "")
	require.Error(t, err)
}
