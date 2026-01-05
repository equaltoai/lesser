package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round18ValidationService struct {
	validateCreatePostErr error
	validateDeletePostErr error
	validateFollowErr     error
	validateLikeErr       error
	validateUpdateErr     error
}

func (v *round18ValidationService) ValidateCreatePost(input *CreatePostInput) error {
	return v.validateCreatePostErr
}
func (v *round18ValidationService) ValidateFollowInput(input *FollowInput) error {
	return v.validateFollowErr
}
func (v *round18ValidationService) ValidateLikeInput(input *LikeInput) error {
	return v.validateLikeErr
}
func (v *round18ValidationService) ValidateDeletePost(input *DeletePostInput) error {
	return v.validateDeletePostErr
}
func (v *round18ValidationService) ValidateUpdatePost(input *UpdatePostInput) error {
	return v.validateUpdateErr
}

type round18ScheduledStatusRepo struct {
	mu     sync.Mutex
	byID   map[string]*storage.ScheduledStatus
	create error
}

func (r *round18ScheduledStatusRepo) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.create != nil {
		return r.create
	}
	if r.byID == nil {
		r.byID = make(map[string]*storage.ScheduledStatus)
	}
	r.byID[scheduled.ID] = scheduled
	return nil
}

func (r *round18ScheduledStatusRepo) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byID[id], nil
}

func (r *round18ScheduledStatusRepo) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	_ = ctx
	_ = username
	_ = limit
	_ = cursor
	return nil, "", nil
}

func (r *round18ScheduledStatusRepo) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	_ = ctx
	_ = scheduled
	return nil
}

func (r *round18ScheduledStatusRepo) DeleteScheduledStatus(ctx context.Context, id string) error {
	_ = ctx
	_ = id
	return nil
}

func (r *round18ScheduledStatusRepo) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	_ = ctx
	_ = before
	_ = limit
	return nil, nil
}

func (r *round18ScheduledStatusRepo) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	_ = ctx
	_ = id
	return nil
}

type round18StorageAdapter struct {
	actorsByKey map[string]*activitypub.Actor
	objectsByID map[string]any

	getActorErr           error
	createObjectErr       error
	createActivityErr     error
	tombstoneObjectErr    error
	incrementReplyErr     error
	getObjectErr          error
	createRelationshipErr error
	removeRelationshipErr error
	isFollowing           bool
	isFollowingErr        error
	hasLiked              bool
	hasLikedErr           error
	createLikeErr         error
	removeLikeErr         error
	removeFromTimelineErr error
	recordHashtagUsageErr error

	scheduledRepo *round18ScheduledStatusRepo
	db            any
}

func (s *round18StorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	_ = ctx
	if s.getActorErr != nil {
		return nil, s.getActorErr
	}
	if actor, ok := s.actorsByKey[username]; ok {
		return actor, nil
	}
	return nil, errors.New("not found")
}

func (s *round18StorageAdapter) CreateObject(ctx context.Context, object interface{}) error {
	_ = ctx
	if s.createObjectErr != nil {
		return s.createObjectErr
	}
	if note, ok := object.(*activitypub.Note); ok {
		if s.objectsByID == nil {
			s.objectsByID = make(map[string]any)
		}
		s.objectsByID[note.ID] = note
	}
	return nil
}

func (s *round18StorageAdapter) GetObject(ctx context.Context, objectID string) (interface{}, error) {
	_ = ctx
	if s.getObjectErr != nil {
		return nil, s.getObjectErr
	}
	if s.objectsByID == nil {
		return nil, errors.New("not found")
	}
	obj, ok := s.objectsByID[objectID]
	if !ok {
		return nil, errors.New("not found")
	}
	return obj, nil
}

func (s *round18StorageAdapter) TombstoneObject(ctx context.Context, objectID, actorID string) error {
	_ = ctx
	_ = objectID
	_ = actorID
	return s.tombstoneObjectErr
}

func (s *round18StorageAdapter) IncrementReplyCount(ctx context.Context, objectID string) error {
	_ = ctx
	_ = objectID
	return s.incrementReplyErr
}

func (s *round18StorageAdapter) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	_ = ctx
	_ = activity
	return s.createActivityErr
}

func (s *round18StorageAdapter) CreateRelationship(ctx context.Context, followerUsername, followingID, activityID string) error {
	_ = ctx
	_ = followerUsername
	_ = followingID
	_ = activityID
	return s.createRelationshipErr
}

func (s *round18StorageAdapter) RemoveRelationship(ctx context.Context, followerUsername, followingID string) error {
	_ = ctx
	_ = followerUsername
	_ = followingID
	return s.removeRelationshipErr
}

func (s *round18StorageAdapter) IsFollowing(ctx context.Context, followerUsername, followingID string) (bool, error) {
	_ = ctx
	_ = followerUsername
	_ = followingID
	return s.isFollowing, s.isFollowingErr
}

func (s *round18StorageAdapter) CreateLike(ctx context.Context, actorID, objectID, activityID string) error {
	_ = ctx
	_ = actorID
	_ = objectID
	_ = activityID
	return s.createLikeErr
}

func (s *round18StorageAdapter) RemoveLike(ctx context.Context, actorID, objectID string) error {
	_ = ctx
	_ = actorID
	_ = objectID
	return s.removeLikeErr
}

func (s *round18StorageAdapter) HasLiked(ctx context.Context, actorID, objectID string) (bool, error) {
	_ = ctx
	_ = actorID
	_ = objectID
	return s.hasLiked, s.hasLikedErr
}

func (s *round18StorageAdapter) RecordActivity(ctx context.Context, activityType, actorID string, timestamp time.Time) error {
	_ = ctx
	_ = activityType
	_ = actorID
	_ = timestamp
	return nil
}

func (s *round18StorageAdapter) RecordHashtagUsage(ctx context.Context, hashtag, objectID, actorID string) error {
	_ = ctx
	_ = hashtag
	_ = objectID
	_ = actorID
	return s.recordHashtagUsageErr
}

func (s *round18StorageAdapter) RecordLinkShare(ctx context.Context, link, objectID, actorID string) error {
	_ = ctx
	_ = link
	_ = objectID
	_ = actorID
	return nil
}

func (s *round18StorageAdapter) RecordStatusEngagement(ctx context.Context, objectID, engagementType, actorID string) error {
	_ = ctx
	_ = objectID
	_ = engagementType
	_ = actorID
	return nil
}

func (s *round18StorageAdapter) RecordInstanceActivity(ctx context.Context, activityType string, timestamp time.Time) error {
	_ = ctx
	_ = activityType
	_ = timestamp
	return nil
}

func (s *round18StorageAdapter) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	_ = ctx
	_ = activity
	return nil
}

func (s *round18StorageAdapter) RemoveFromTimelines(ctx context.Context, objectID string) error {
	_ = ctx
	_ = objectID
	return s.removeFromTimelineErr
}

func (s *round18StorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	_ = ctx
	_ = username
	_ = limit
	_ = cursor
	return nil, "", nil
}

func (s *round18StorageAdapter) CreateNotification(ctx context.Context, notification interface{}) error {
	_ = ctx
	_ = notification
	return nil
}

func (s *round18StorageAdapter) DeleteNotificationsByObject(ctx context.Context, objectID string) error {
	_ = ctx
	_ = objectID
	return nil
}

func (s *round18StorageAdapter) ScheduledStatus() interface {
	CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	DeleteScheduledStatus(ctx context.Context, id string) error
	GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error)
	MarkScheduledStatusPublished(ctx context.Context, id string) error
} {
	if s.scheduledRepo == nil {
		s.scheduledRepo = &round18ScheduledStatusRepo{}
	}
	return s.scheduledRepo
}

func (s *round18StorageAdapter) GetInfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error) {
	_ = ctx
	return nil, nil
}

func (s *round18StorageAdapter) GetInstanceBudgets(ctx context.Context, exceeded *bool) ([]*model.InstanceBudget, error) {
	_ = ctx
	_ = exceeded
	return nil, nil
}

func (s *round18StorageAdapter) GetInstanceHealthReport(ctx context.Context, domain string) (*model.InstanceHealthReport, error) {
	_ = ctx
	_ = domain
	return nil, nil
}

func (s *round18StorageAdapter) GetInstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error) {
	_ = ctx
	_ = domain
	return nil, nil
}

func (s *round18StorageAdapter) GetDB() interface{} { return s.db }
func (s *round18StorageAdapter) GetTableName() string {
	return "test-table"
}

type round18TimelineService struct {
	fanOutErr error
}

func (t *round18TimelineService) FanOutToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	_ = ctx
	_ = activity
	_ = actor
	return t.fanOutErr
}
func (t *round18TimelineService) UpdateTimelines(ctx context.Context, activity *activitypub.Activity) error {
	_ = ctx
	_ = activity
	return nil
}
func (t *round18TimelineService) RemoveFromTimelines(ctx context.Context, objectID string) error {
	_ = ctx
	_ = objectID
	return nil
}

type round18FederationService struct {
	deliverToFollowersErr   error
	deliverToRecipientsErr  error
	determineRecipientsErr  error
	determineRecipientsList []string
}

func (f *round18FederationService) DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	_ = ctx
	_ = activity
	_ = actor
	return f.deliverToFollowersErr
}
func (f *round18FederationService) DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	_ = ctx
	_ = activity
	_ = actor
	return f.deliverToRecipientsErr
}
func (f *round18FederationService) DetermineRecipients(ctx context.Context, activity *activitypub.Activity, visibility string) ([]string, error) {
	_ = ctx
	_ = activity
	_ = visibility
	return f.determineRecipientsList, f.determineRecipientsErr
}
func (f *round18FederationService) GetInstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error) {
	_ = ctx
	_ = domain
	return nil, nil
}

type round18DeliverSignalFederation struct {
	round18FederationService
	ch chan struct{}
}

func (f *round18DeliverSignalFederation) DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	select {
	case f.ch <- struct{}{}:
	default:
	}
	return f.round18FederationService.DeliverToRecipients(ctx, activity, actor)
}

type round18AnalyticsService struct {
	recordStatusCreationErr error
	recordHashtagUsageErr   error
	recordLinkShareErr      error
	recordEngagementErr     error
}

func (a *round18AnalyticsService) RecordStatusCreation(ctx context.Context, actorID string, timestamp time.Time) error {
	_ = ctx
	_ = actorID
	_ = timestamp
	return a.recordStatusCreationErr
}
func (a *round18AnalyticsService) RecordHashtagUsage(ctx context.Context, hashtags []string, objectID, actorID string) error {
	_ = ctx
	_ = hashtags
	_ = objectID
	_ = actorID
	return a.recordHashtagUsageErr
}
func (a *round18AnalyticsService) RecordLinkShare(ctx context.Context, links []string, objectID, actorID string) error {
	_ = ctx
	_ = links
	_ = objectID
	_ = actorID
	return a.recordLinkShareErr
}
func (a *round18AnalyticsService) RecordEngagement(ctx context.Context, objectID, engagementType, actorID string) error {
	_ = ctx
	_ = objectID
	_ = engagementType
	_ = actorID
	return a.recordEngagementErr
}
func (a *round18AnalyticsService) RecordInstanceActivity(ctx context.Context, activityType string, timestamp time.Time) error {
	_ = ctx
	_ = activityType
	_ = timestamp
	return nil
}
func (a *round18AnalyticsService) GetInfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error) {
	_ = ctx
	return nil, nil
}
func (a *round18AnalyticsService) GetInstanceBudgets(ctx context.Context, exceeded *bool) ([]*model.InstanceBudget, error) {
	_ = ctx
	_ = exceeded
	return nil, nil
}
func (a *round18AnalyticsService) GetInstanceHealthReport(ctx context.Context, domain string) (*model.InstanceHealthReport, error) {
	_ = ctx
	_ = domain
	return nil, nil
}

type round18Publisher struct {
	publishUserErr   error
	publishStreamErr error
}

func (p *round18Publisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	_ = ctx
	_ = userID
	_ = event
	return p.publishUserErr
}
func (p *round18Publisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	_ = ctx
	_ = streamName
	_ = event
	return p.publishStreamErr
}
func (p *round18Publisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	_ = ctx
	_ = conversationID
	_ = event
	return nil
}
func (p *round18Publisher) Close() error { return nil }

type round18JobQueue struct {
	queueScheduledErr error
}

func (q *round18JobQueue) QueueImportJob(ctx context.Context, msg ImportJobMessage) error {
	_ = ctx
	_ = msg
	return nil
}
func (q *round18JobQueue) QueueExportJob(ctx context.Context, msg ExportJobMessage) error {
	_ = ctx
	_ = msg
	return nil
}
func (q *round18JobQueue) QueueMediaJob(ctx context.Context, msg MediaJobMessage) error {
	_ = ctx
	_ = msg
	return nil
}
func (q *round18JobQueue) QueueScheduledJob(ctx context.Context, msg ScheduledJobMessage) error {
	_ = ctx
	_ = msg
	return q.queueScheduledErr
}
func (q *round18JobQueue) QueueActivityJob(ctx context.Context, msg ActivityJobMessage) error {
	_ = ctx
	_ = msg
	return nil
}
func (q *round18JobQueue) QueueDelayedJob(ctx context.Context, queueName string, messageBody interface{}, delaySeconds int32) error {
	_ = ctx
	_ = queueName
	_ = messageBody
	_ = delaySeconds
	return nil
}

type round18LikeCascadeRepo struct {
	err error
}

func (r *round18LikeCascadeRepo) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	_ = ctx
	_ = objectID
	return r.err
}

type round18CascadeDB struct {
	likeRepo *round18LikeCascadeRepo
}

func (db *round18CascadeDB) Like() interface {
	CascadeDeleteLikes(ctx context.Context, objectID string) error
} {
	return db.likeRepo
}

func newBusinessLogicServiceForRound18Test(t *testing.T) (*businessLogicService, *round18StorageAdapter) {
	t.Helper()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
		PreferredUsername: "alice",
		Followers:         "https://example.com/users/alice/followers",
	}
	target := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/bob"},
		PreferredUsername: "bob",
		Followers:         "https://example.com/users/bob/followers",
	}

	storageAdapter := &round18StorageAdapter{
		actorsByKey: map[string]*activitypub.Actor{
			"alice": actor,
			"bob":   target,
		},
	}

	svc := &businessLogicService{
		deps: &ServiceDependencies{
			Config: &ServiceConfig{BaseURL: "https://example.com"},
			Repos:  mocks.NewMockRepositoryStorage(),
			Logger: zap.NewNop(),
		},
		storage:    storageAdapter,
		validation: &round18ValidationService{},
		federation: &round18FederationService{},
		timeline:   &round18TimelineService{},
		analytics:  &round18AnalyticsService{},
		publisher:  &round18Publisher{},
		jobQueue:   &round18JobQueue{},
		logger:     zap.NewNop(),
	}

	return svc, storageAdapter
}

func TestBusinessLogicService_Round18_MainFlows(t *testing.T) {
	svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
	user := &UserContext{Username: "alice"}

	t.Run("CreatePost_scheduled_and_immediate", func(t *testing.T) {
		soon := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second).Format(time.RFC3339)

		// Scheduled path.
		result, err := svc.CreatePost(context.Background(), user, &CreatePostInput{
			Content:     "hello world",
			Visibility:  VisibilityPublic,
			ScheduledAt: &soon,
			MediaIDs:    []string{"m1"},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		// Immediate path.
		storageAdapter.incrementReplyErr = errors.New("increment failed")
		result, err = svc.CreatePost(context.Background(), user, &CreatePostInput{
			Content:     "hello #tag http://a.example",
			Visibility:  VisibilityPublic,
			InReplyToID: "https://example.com/objects/reply1",
		})
		require.NoError(t, err)
		require.NotNil(t, result.Activity)
		require.NotNil(t, result.Note)
	})

	t.Run("performPostCreationTasks_and_reply_processing_errors_do_not_panic", func(t *testing.T) {
		analytics := &round18AnalyticsService{
			recordStatusCreationErr: errors.New("status"),
			recordHashtagUsageErr:   errors.New("hashtag"),
			recordLinkShareErr:      errors.New("link"),
		}
		federation := &round18FederationService{deliverToFollowersErr: errors.New("federation")}
		timeline := &round18TimelineService{fanOutErr: errors.New("fanout")}
		svc2, _ := newBusinessLogicServiceForRound18Test(t)
		svc2.analytics = analytics
		svc2.federation = federation
		svc2.timeline = timeline

		now := time.Now()
		actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}
		note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/objects/1"}, Content: "x"}
		activity := &activitypub.Activity{Object: note}
		svc2.performPostCreationTasks(context.Background(), activity, &CreatePostInput{Content: "x"}, []string{"tag"}, actor, user, now)

		svc2.analytics = &round18AnalyticsService{recordEngagementErr: errors.New("engagement")}
		svc2.storage = &round18StorageAdapter{incrementReplyErr: errors.New("increment")}
		_ = svc2.handleReplyProcessing(context.Background(), "https://example.com/objects/reply1", actor.ID)
	})

	t.Run("DeletePost_follow_like_update_unfollow_unlike", func(t *testing.T) {
		actor, err := storageAdapter.GetActor(context.Background(), "alice")
		require.NoError(t, err)

		existing := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/objects/obj1",
				Published: round18PtrTime(time.Now()),
			},
			AttributedTo: actor.ID,
			Content:      "hello",
			Visibility:   VisibilityPublic,
		}
		storageAdapter.objectsByID = map[string]any{existing.ID: existing}
		storageAdapter.db = &round18CascadeDB{likeRepo: &round18LikeCascadeRepo{}}

		// Delete
		_, err = svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: existing.ID})
		require.NoError(t, err)

		// Follow (requested)
		targetActor, err := storageAdapter.GetActor(context.Background(), "bob")
		require.NoError(t, err)
		targetActor.ManuallyApprovesFollowers = true
		storageAdapter.actorsByKey["bob"] = targetActor

		_, err = svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "bob"})
		require.NoError(t, err)

		// Like
		storageAdapter.hasLiked = false
		storageAdapter.objectsByID[existing.ID] = &activitypub.Note{AttributedTo: "https://example.com/users/bob"}
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: existing.ID})
		require.NoError(t, err)

		// Update
		storageAdapter.objectsByID[existing.ID] = existing

		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{
			ObjectID:    existing.ID,
			Content:     "updated #tag",
			Visibility:  VisibilityPublic,
			SpoilerText: "cw",
			Sensitive:   true,
		})
		require.NoError(t, err)

		// Unfollow
		storageAdapter.isFollowing = true
		_, err = svc.UnfollowActor(context.Background(), user, "bob")
		require.NoError(t, err)

		// Unlike
		storageAdapter.hasLiked = true
		_, err = svc.UnlikeObject(context.Background(), user, existing.ID)
		require.NoError(t, err)
	})

	t.Run("cascade_and_event_helpers", func(t *testing.T) {
		svc3, storage3 := newBusinessLogicServiceForRound18Test(t)
		storage3.db = &round18CascadeDB{likeRepo: &round18LikeCascadeRepo{err: errors.New("cascade")}}
		_ = svc3.performCascadeDeletion(context.Background(), "https://example.com/objects/1", "https://example.com/users/alice")

		// Fallback branch when DB doesn't support Like().
		storage3.db = struct{}{}
		require.NoError(t, svc3.cascadeDeleteLikes(context.Background(), "https://example.com/objects/1"))

		// Simple helpers.
		require.NoError(t, svc3.cascadeDeleteAnnounces(context.Background(), "https://example.com/objects/1"))
		require.NoError(t, svc3.handleReplyChainUpdates(context.Background(), "https://example.com/objects/1"))
		require.NoError(t, svc3.removeFromUserCollections(context.Background(), "https://example.com/objects/1"))

		actor := &activitypub.Actor{PreferredUsername: "alice"}
		note := &activitypub.Note{Visibility: VisibilityPublic}
		act := &activitypub.Activity{Object: &activitypub.Activity{Object: "bob"}}
		svc3.publisher = &round18Publisher{publishUserErr: errors.New("publish"), publishStreamErr: errors.New("publish")}
		svc3.emitUnfollowEvents(context.Background(), act, actor)
		svc3.emitUnlikeEvents(context.Background(), act, actor)
		svc3.emitPostUpdateEvents(context.Background(), &activitypub.Activity{}, actor, note)

		// Cover federation / analytics helper methods.
		svc3.federation = &round18FederationService{deliverToRecipientsErr: errors.New("deliver")}
		svc3.handleFollowFederation(context.Background(), &activitypub.Activity{}, actor, actor, false)
		svc3.analytics = &round18AnalyticsService{recordEngagementErr: errors.New("engagement")}
		svc3.handleLikePostProcessing(context.Background(), &activitypub.Activity{Object: "https://example.com/objects/1"}, actor, nil)
	})

	t.Run("extractMediaIDs_branches", func(t *testing.T) {
		require.Nil(t, extractMediaIDs(nil))
		require.Equal(t, []string{"a"}, extractMediaIDs([]string{"a"}))
		require.Equal(t, []string{"a"}, extractMediaIDs([]interface{}{"a", 1}))
		require.Nil(t, extractMediaIDs(123))
	})
}

func round18PtrTime(t time.Time) *time.Time { return &t }

func TestBusinessLogicService_Round18_ErrorBranches(t *testing.T) {
	t.Run("CreatePost_validation_and_storage_errors", func(t *testing.T) {
		svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
		user := &UserContext{Username: "alice"}

		svc.validation = &round18ValidationService{validateCreatePostErr: errors.New("bad")}
		_, err := svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		svc.deps.Repos = struct{}{} // forces emoji repo unavailable branch
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "hi :smile:"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.createObjectErr = errors.New("create")
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.createActivityErr = errors.New("create activity")
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x"})
		require.Error(t, err)
	})

	t.Run("DeletePost_validation_and_notfound_forbidden_errors", func(t *testing.T) {
		svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
		user := &UserContext{Username: "alice"}

		svc.validation = &round18ValidationService{validateDeletePostErr: errors.New("bad")}
		_, err := svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "x"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "x"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getObjectErr = errors.New("not found")
		_, err = svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "x"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		actor, _ := storageAdapter.GetActor(context.Background(), "alice")
		storageAdapter.objectsByID = map[string]any{
			"https://example.com/objects/obj1": &activitypub.Note{AttributedTo: "https://example.com/users/other"},
		}
		_, err = svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "https://example.com/objects/obj1"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{
			"https://example.com/objects/obj1": &activitypub.Note{AttributedTo: actor.ID},
		}
		storageAdapter.createActivityErr = errors.New("create activity")
		_, err = svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "https://example.com/objects/obj1"})
		require.Error(t, err)

		// Tombstone error: stops deletion before any federation delivery goroutine.
		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{
			"https://example.com/objects/obj1": &activitypub.Note{AttributedTo: actor.ID},
		}
		storageAdapter.tombstoneObjectErr = errors.New("tombstone")
		_, err = svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "https://example.com/objects/obj1"})
		require.Error(t, err)

		// Federation delivery error branch runs asynchronously after tombstone succeeds.
		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{
			"https://example.com/objects/obj1": &activitypub.Note{AttributedTo: actor.ID},
		}
		delivered := make(chan struct{}, 1)
		svc.federation = &round18DeliverSignalFederation{
			round18FederationService: round18FederationService{deliverToRecipientsErr: errors.New("deliver")},
			ch:                       delivered,
		}

		res, err := svc.DeletePost(context.Background(), user, &DeletePostInput{ObjectID: "https://example.com/objects/obj1"})
		require.NoError(t, err)
		require.True(t, res.Deleted)

		select {
		case <-delivered:
		case <-time.After(1 * time.Second):
			t.Fatal("expected DeliverToRecipients to be called")
		}
	})

	t.Run("FollowActor_and_LikeObject_error_branches", func(t *testing.T) {
		svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
		user := &UserContext{Username: "alice"}

		svc.validation = &round18ValidationService{validateFollowErr: errors.New("bad")}
		_, err := svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "bob"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "bob"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		_, err = svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "missing"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.isFollowing = true
		_, err = svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "bob"})
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.createRelationshipErr = errors.New("rel")
		_, err = svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "bob"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.createActivityErr = errors.New("activity")
		_, err = svc.FollowActor(context.Background(), user, &FollowInput{TargetActorID: "bob"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		svc.validation = &round18ValidationService{validateLikeErr: errors.New("bad")}
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.hasLiked = true
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: "obj"})
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getObjectErr = errors.New("missing")
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": &activitypub.Note{AttributedTo: "https://example.com/users/bob"}}
		storageAdapter.createLikeErr = errors.New("like")
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": &activitypub.Note{AttributedTo: "https://example.com/users/bob"}}
		storageAdapter.createActivityErr = errors.New("activity")
		_, err = svc.LikeObject(context.Background(), user, &LikeInput{ObjectID: "obj"})
		require.Error(t, err)
	})

	t.Run("handleScheduledPost_validation_and_storage_branches", func(t *testing.T) {
		svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
		user := &UserContext{Username: "alice"}

		bad := "not-a-time"
		_, err := svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &bad})
		require.Error(t, err)

		tooSoon := time.Now().Add(1 * time.Minute).UTC().Truncate(time.Second).Format(time.RFC3339)
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &tooSoon})
		require.Error(t, err)

		tooFar := time.Now().AddDate(2, 0, 0).UTC().Truncate(time.Second).Format(time.RFC3339)
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &tooFar})
		require.Error(t, err)

		okTime := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second).Format(time.RFC3339)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &okTime})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.scheduledRepo = &round18ScheduledStatusRepo{create: errors.New("create")}
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &okTime})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		svc.jobQueue = nil
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &okTime})
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		svc.jobQueue = &round18JobQueue{queueScheduledErr: errors.New("queue")}
		_, err = svc.CreatePost(context.Background(), user, &CreatePostInput{Content: "x", ScheduledAt: &okTime})
		require.NoError(t, err)
	})

	t.Run("UpdatePost_and_UnfollowUnlike_error_branches", func(t *testing.T) {
		svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
		user := &UserContext{Username: "alice"}

		storageAdapter.getActorErr = errors.New("db")
		_, err := svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getObjectErr = errors.New("missing")
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": "not-a-note"}
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj"})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": &activitypub.Note{AttributedTo: "someone"}}
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj"})
		require.Error(t, err)

		// Cover remaining visibility switch cases.
		actor, _ := storageAdapter.GetActor(context.Background(), "alice")
		note := &activitypub.Note{AttributedTo: actor.ID, Visibility: VisibilityPublic}
		storageAdapter.objectsByID = map[string]any{"obj": note}
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj", Content: "x", Visibility: VisibilityUnlisted})
		require.NoError(t, err)
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj", Content: "x", Visibility: VisibilityPrivate})
		require.NoError(t, err)
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj", Content: "x", Visibility: VisibilityDirect})
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": &activitypub.Note{AttributedTo: actor.ID}}
		storageAdapter.createObjectErr = errors.New("create")
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj", Content: "x", Visibility: VisibilityPublic})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": &activitypub.Note{AttributedTo: actor.ID}}
		storageAdapter.createActivityErr = errors.New("activity")
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj", Content: "x", Visibility: VisibilityPublic})
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.objectsByID = map[string]any{"obj": &activitypub.Note{AttributedTo: actor.ID}}
		storageAdapter.recordHashtagUsageErr = errors.New("hashtag")
		_, err = svc.UpdatePost(context.Background(), user, &UpdatePostInput{ObjectID: "obj", Content: "#tag", Visibility: VisibilityPublic})
		require.NoError(t, err)

		// Unfollow/Unlike alternate branches and errors.
		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.UnfollowActor(context.Background(), user, "bob")
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.isFollowingErr = errors.New("db")
		_, err = svc.UnfollowActor(context.Background(), user, "bob")
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.isFollowing = false
		_, err = svc.UnfollowActor(context.Background(), user, "bob")
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.isFollowing = true
		storageAdapter.createActivityErr = errors.New("activity")
		_, err = svc.UnfollowActor(context.Background(), user, "bob")
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.isFollowing = true
		storageAdapter.removeRelationshipErr = errors.New("remove")
		_, err = svc.UnfollowActor(context.Background(), user, "bob")
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.getActorErr = errors.New("db")
		_, err = svc.UnlikeObject(context.Background(), user, "obj")
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.hasLikedErr = errors.New("db")
		_, err = svc.UnlikeObject(context.Background(), user, "obj")
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.hasLiked = false
		_, err = svc.UnlikeObject(context.Background(), user, "obj")
		require.NoError(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.hasLiked = true
		storageAdapter.createActivityErr = errors.New("activity")
		_, err = svc.UnlikeObject(context.Background(), user, "obj")
		require.Error(t, err)

		svc, storageAdapter = newBusinessLogicServiceForRound18Test(t)
		storageAdapter.hasLiked = true
		storageAdapter.removeLikeErr = errors.New("remove")
		_, err = svc.UnlikeObject(context.Background(), user, "obj")
		require.NoError(t, err)
	})

	t.Run("performCascadeDeletion_branches", func(t *testing.T) {
		svc, storageAdapter := newBusinessLogicServiceForRound18Test(t)
		storageAdapter.removeFromTimelineErr = errors.New("remove")
		storageAdapter.db = &round18CascadeDB{likeRepo: &round18LikeCascadeRepo{}}
		require.NoError(t, svc.performCascadeDeletion(context.Background(), "obj", "actor"))

		svc.publisher = nil
		svc.emitUnfollowEvents(context.Background(), &activitypub.Activity{Object: &activitypub.Activity{Object: "x"}}, &activitypub.Actor{PreferredUsername: "alice"})
	})
}
