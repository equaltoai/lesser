package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type recordingInboxProcessingRecorder struct {
	seen     map[string]string
	released []string
}

func newRecordingInboxProcessingRecorder() *recordingInboxProcessingRecorder {
	return &recordingInboxProcessingRecorder{seen: make(map[string]string)}
}

func (r *recordingInboxProcessingRecorder) TryRecordTarget(_ context.Context, activityID, targetActorID, activityType string) (bool, error) {
	key := activityID + "\x00" + targetActorID
	if _, exists := r.seen[key]; exists {
		return false, nil
	}
	r.seen[key] = activityType
	return true, nil
}

func (r *recordingInboxProcessingRecorder) ForgetTarget(_ context.Context, activityID, targetActorID string) error {
	r.released = append(r.released, activityID+"\x00"+targetActorID)
	return nil
}

func TestInboxHandler_Project20_TargetReceiptIdempotencyIncludesCreateAndUndoInteractions(t *testing.T) {
	env := newInboxTestEnv(t)
	recorder := newRecordingInboxProcessingRecorder()
	env.handler.inboxProcessingRepository = recorder

	activities := []*activitypub.Activity{
		{
			BaseObject: activitypub.BaseObject{ID: "https://remote.example/activities/create-1", Type: activitypub.CreateType},
			Actor:      env.remoteActorID,
			Object:     "https://remote.example/objects/note-1",
		},
		{
			BaseObject: activitypub.BaseObject{ID: "https://remote.example/activities/undo-like-1", Type: activitypub.UndoType},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":     "https://remote.example/activities/like-1",
				"type":   activitypub.LikeType,
				"actor":  env.remoteActorID,
				"object": env.cfg.BaseURL() + "/objects/1",
			},
		},
		{
			BaseObject: activitypub.BaseObject{ID: "https://remote.example/activities/undo-announce-1", Type: activitypub.UndoType},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":     "https://remote.example/activities/announce-1",
				"type":   activitypub.AnnounceType,
				"actor":  env.remoteActorID,
				"object": env.cfg.BaseURL() + "/objects/1",
			},
		},
	}

	for _, activity := range activities {
		first, err := env.handler.shouldProcessInboundActivityTarget(context.Background(), activity, env.local)
		require.NoError(t, err)
		require.True(t, first, activity.ID)

		second, err := env.handler.shouldProcessInboundActivityTarget(context.Background(), activity, env.local)
		require.NoError(t, err)
		require.False(t, second, activity.ID)
	}

	require.Equal(t, activitypub.CreateType, recorder.seen["https://remote.example/activities/create-1\x00"+env.local.ID])
	require.Equal(t, activitypub.UndoType, recorder.seen["https://remote.example/activities/undo-like-1\x00"+env.local.ID])
	require.Equal(t, activitypub.UndoType, recorder.seen["https://remote.example/activities/undo-announce-1\x00"+env.local.ID])
}

func TestInboxHandler_Project20_RemoteCreateMentionNotificationIsDeterministic(t *testing.T) {
	env := newInboxTestEnv(t)
	notifications := inmemory.NewNotificationRepository()
	env.handler.notificationRepository = notifications
	env.handler.statusRepository = newProject20StatusRepo()

	published := time.Date(2026, 4, 24, 15, 30, 0, 0, time.UTC)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        "https://remote.example/users/bob/statuses/mention-1",
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{env.local.ID},
			CC:        []string{activitypub.PublicAddress},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>hello @alice</p>",
		Tag: []activitypub.Tag{{
			Type: "Mention",
			Href: env.local.ID,
			Name: "@alice@localhost",
		}},
	}
	activity := remoteCreateActivityForNote(t, env.remoteActorID, note, "https://remote.example/activities/create-mention-1")

	require.NoError(t, env.handler.processRemoteCreateActivity(context.Background(), activity, env.local))
	require.NoError(t, env.handler.processRemoteCreateActivity(context.Background(), activity, env.local))

	result, err := notifications.GetUserNotifications(context.Background(), "alice", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, common.NotificationTypeMention, result.Items[0].Type)
	require.Equal(t, env.remoteActorID, result.Items[0].ActorID)
	require.Equal(t, "remote_actor", result.Items[0].ActorType)
	require.Equal(t, "status", result.Items[0].TargetType)
	require.NotEmpty(t, result.Items[0].TargetID)
	require.Contains(t, result.Items[0].Data, "postSnapshot")
}

func TestInboxHandler_Project20_RemoteCreateReplyNotificationTargetsLocalParentAuthor(t *testing.T) {
	env := newInboxTestEnv(t)
	notifications := inmemory.NewNotificationRepository()
	parentURL := env.cfg.BaseURL() + "/users/alice/statuses/parent-1"
	env.handler.notificationRepository = notifications
	env.handler.statusRepository = newProject20StatusRepo(&models.Status{
		StatusID:       "parent-1",
		AuthorID:       env.local.ID,
		AuthorUsername: "alice",
	})

	published := time.Date(2026, 4, 24, 15, 45, 0, 0, time.UTC)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        "https://remote.example/users/bob/statuses/reply-1",
			Type:      activitypub.NoteType,
			Published: &published,
			InReplyTo: parentURL,
			To:        []string{env.local.ID},
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>replying from remote</p>",
	}
	activity := remoteCreateActivityForNote(t, env.remoteActorID, note, "https://remote.example/activities/create-reply-1")

	require.NoError(t, env.handler.processRemoteCreateActivity(context.Background(), activity, env.local))

	result, err := notifications.GetUserNotifications(context.Background(), "alice", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, common.NotificationTypeReply, result.Items[0].Type)
	require.Equal(t, "parent-1", result.Items[0].Data["parent_status_id"])
	require.Contains(t, result.Items[0].Data, "postSnapshot")
}

type project20StatusRepo struct {
	interfaces.StatusRepository
	statuses map[string]*models.Status
}

func newProject20StatusRepo(statuses ...*models.Status) *project20StatusRepo {
	repo := &project20StatusRepo{statuses: make(map[string]*models.Status)}
	for _, status := range statuses {
		if status != nil {
			repo.statuses[status.StatusID] = status
		}
	}
	return repo
}

func (r *project20StatusRepo) CreateStatus(_ context.Context, status *models.Status) error {
	if status == nil {
		return fmt.Errorf("status is required")
	}
	if err := status.BeforeCreate(); err != nil {
		return err
	}
	r.statuses[status.StatusID] = status
	return nil
}

func (r *project20StatusRepo) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if status, ok := r.statuses[statusID]; ok {
		return status, nil
	}
	return nil, storage.ErrNotFound
}

func remoteCreateActivityForNote(t *testing.T, actorID string, note *activitypub.Note, activityID string) *activitypub.Activity {
	t.Helper()

	body, err := json.Marshal(note)
	require.NoError(t, err)

	var object map[string]any
	require.NoError(t, json.Unmarshal(body, &object))

	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   activityID,
			Type: activitypub.CreateType,
			To:   note.To,
			CC:   note.CC,
		},
		Actor:  actorID,
		Object: object,
	}
}

func TestInboxHandler_Project20_RemoteNotificationHelperBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	oldDomain := env.cfg.Domain
	oldBaseURL := env.handler.baseURL
	defer func() {
		env.cfg.Domain = oldDomain
		env.handler.baseURL = oldBaseURL
	}()
	published := time.Date(2026, 4, 24, 16, 30, 0, 0, time.UTC)
	activityPublished := published.Add(time.Minute)
	created := published.Add(2 * time.Minute)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/users/bob/statuses/helper-1",
			Type:      activitypub.NoteType,
			Published: &published,
			InReplyTo: env.cfg.BaseURL() + "/users/alice/statuses/parent-1",
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>helper</p>",
		Visibility:   models.VisibilityPublic,
	}
	status := &models.Status{
		StatusID:    "remote-helper-1",
		Visibility:  models.VisibilityPublic,
		PublishedAt: published.Add(3 * time.Minute),
		CreatedAt:   created,
	}

	require.Equal(t, published, remoteCreateNotificationCreatedAt(&activitypub.Activity{}, note, status))
	require.Equal(t, activityPublished, remoteCreateNotificationCreatedAt(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{Published: &activityPublished},
	}, &activitypub.Note{}, status))
	require.Equal(t, status.PublishedAt, remoteCreateNotificationCreatedAt(&activitypub.Activity{}, &activitypub.Note{}, status))
	statusWithoutPublished := &models.Status{CreatedAt: created}
	require.Equal(t, created, remoteCreateNotificationCreatedAt(&activitypub.Activity{}, &activitypub.Note{}, statusWithoutPublished))
	require.False(t, remoteCreateNotificationCreatedAt(nil, nil, nil).IsZero())

	firstID := deterministicRemoteCreateNotificationID(common.NotificationTypeMention, "alice", env.remoteActorID, note.ID)
	secondID := deterministicRemoteCreateNotificationID(common.NotificationTypeMention, "alice", env.remoteActorID, note.ID)
	require.Equal(t, firstID, secondID)
	require.NotEqual(t, firstID, deterministicRemoteCreateNotificationID(common.NotificationTypeReply, "alice", env.remoteActorID, note.ID))

	snapshot := remoteCreatePostSnapshot(note, status)
	require.Equal(t, note.ID, snapshot["id"])
	require.Equal(t, note.Content, snapshot["content"])
	require.Equal(t, note.AttributedTo, snapshot["attributedTo"])
	require.Equal(t, note.InReplyTo, snapshot["inReplyToId"])
	require.Equal(t, models.VisibilityPublic, snapshot["visibility"])
	require.Equal(t, status.StatusID, snapshot["statusId"])
	require.NotEmpty(t, snapshot["createdAt"])
	require.Empty(t, remoteCreatePostSnapshot(nil, nil))

	require.True(t, env.handler.remoteNoteMentionsTargetActor(&activitypub.Note{Tag: []activitypub.Tag{{
		Type: "Mention",
		Href: env.local.ID,
	}}}, env.local))
	require.True(t, env.handler.remoteNoteMentionsTargetActor(&activitypub.Note{Tag: []activitypub.Tag{{
		Type: "Mention",
		Href: env.cfg.BaseURL() + "/users/alice",
	}}}, env.local))
	require.True(t, env.handler.remoteNoteMentionsTargetActor(&activitypub.Note{Tag: []activitypub.Tag{{
		Type: "Mention",
		Name: "@alice@localhost",
	}}}, env.local))
	require.True(t, env.handler.remoteNoteMentionsTargetActor(&activitypub.Note{Tag: []activitypub.Tag{{
		Type: "Mention",
		Name: "@alice",
	}}}, env.local))
	require.False(t, env.handler.remoteNoteMentionsTargetActor(nil, env.local))
	require.False(t, env.handler.remoteNoteMentionsTargetActor(&activitypub.Note{}, nil))
	require.False(t, env.handler.remoteNoteMentionsTargetActor(&activitypub.Note{Tag: []activitypub.Tag{{
		Type: "Hashtag",
		Name: "#alice",
	}}}, env.local))

	targetIDs := env.handler.localTargetActorIDs(env.local)
	_, hasPrimary := targetIDs[env.local.ID]
	require.True(t, hasPrimary)
	require.Empty(t, env.handler.localTargetActorIDs(nil))

	parent := &models.Status{StatusID: "parent-1", AuthorID: env.local.ID}
	require.True(t, env.handler.replyParentBelongsToTarget(parent, env.local))
	parent = &models.Status{StatusID: "parent-1", Note: &activitypub.Note{AttributedTo: env.local.ID}}
	require.True(t, env.handler.replyParentBelongsToTarget(parent, env.local))
	parent = &models.Status{StatusID: "parent-1", AuthorUsername: "alice"}
	require.True(t, env.handler.replyParentBelongsToTarget(parent, env.local))
	require.False(t, env.handler.replyParentBelongsToTarget(nil, env.local))
	require.False(t, env.handler.replyParentBelongsToTarget(parent, nil))
	require.False(t, env.handler.replyParentBelongsToTarget(&models.Status{AuthorUsername: "mallory"}, env.local))

	env.cfg.Domain = ""
	env.handler.baseURL = "https://fallback.example"
	require.Equal(t, "fallback.example", env.handler.localDomain())

	username, domain := actorURLUsernameDomain("https://remote.example/users/@bob")
	require.Equal(t, "bob", username)
	require.Equal(t, "remote.example", domain)
	username, domain = actorURLUsernameDomain("not a url")
	require.Empty(t, username)
	require.Empty(t, domain)

	require.True(t, isRemoteCreateNotificationDuplicate(storage.ErrAlreadyExists))
	require.True(t, isRemoteCreateNotificationDuplicate(dynamormerrors.ErrConditionFailed))
	require.False(t, isRemoteCreateNotificationDuplicate(nil))
}

func TestInboxHandler_Project20_IdempotencyHelperFallbackBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	env.handler.inboxProcessingRepository = nil
	shouldProcess, err := env.handler.shouldProcessInboundActivityTarget(context.Background(), nil, nil)
	require.NoError(t, err)
	require.True(t, shouldProcess)

	shouldProcess, err = env.handler.shouldProcessInboundActivityTarget(context.Background(), &activitypub.Activity{}, env.local)
	require.NoError(t, err)
	require.True(t, shouldProcess)

	env.handler.releaseInboundActivityTargetClaim(context.Background(), nil, nil)
}

func TestInboxHandler_Project20_RemoteCreateNotificationCreateAndReleaseBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	notifications := inmemory.NewNotificationRepository()
	env.handler.notificationRepository = notifications

	published := time.Date(2026, 4, 24, 17, 0, 0, 0, time.UTC)
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/activities/create-direct-1",
			Type:      activitypub.CreateType,
			Published: &published,
		},
		Actor: env.remoteActorID,
	}
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/users/bob/statuses/direct-1",
			Type:      activitypub.NoteType,
			Published: &published,
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>direct helper</p>",
	}
	status := &models.Status{
		StatusID:    "remote-direct-1",
		PublishedAt: published,
		CreatedAt:   published,
	}

	input := remoteCreateNotificationInput{
		kind:      common.NotificationTypeMention,
		recipient: "alice",
		actorID:   env.remoteActorID,
		activity:  activity,
		note:      note,
		status:    status,
		title:     "bob mentioned you",
		body:      "bob mentioned you",
		groupKey:  "remote-mention:alice:remote-direct-1",
		extraData: map[string]interface{}{
			"extra": "value",
		},
		stableParts: []string{"alice", env.remoteActorID, activity.ID, note.ID, status.StatusID},
	}

	env.handler.createRemoteCreateNotification(context.Background(), input)
	env.handler.createRemoteCreateNotification(context.Background(), input)

	result, err := notifications.GetUserNotifications(context.Background(), "alice", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "value", result.Items[0].Data["extra"])
	require.Equal(t, activity.ID, result.Items[0].Data["activity_id"])

	failingNotifications := &project20FailingNotificationRepo{err: fmt.Errorf("write failed")}
	env.handler.notificationRepository = failingNotifications
	env.handler.createRemoteCreateNotification(context.Background(), input)
	require.Len(t, failingNotifications.created, 1)

	recorder := newRecordingInboxProcessingRecorder()
	env.handler.inboxProcessingRepository = recorder
	claimed, err := env.handler.shouldProcessInboundActivityTarget(context.Background(), activity, env.local)
	require.NoError(t, err)
	require.True(t, claimed)
	env.handler.releaseInboundActivityTargetClaim(context.Background(), activity, env.local)
	require.Equal(t, []string{activity.ID + "\x00" + env.local.ID}, recorder.released)
}

func TestInboxHandler_Project20_RemoteCreateNotificationGuardBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	env.handler.notificationRepository = inmemory.NewNotificationRepository()
	status := &models.Status{StatusID: "remote-guard-1", CreatedAt: time.Date(2026, 4, 24, 17, 15, 0, 0, time.UTC)}
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/bob/statuses/guard-1",
			Type: activitypub.NoteType,
		},
		AttributedTo: env.remoteActorID,
		Content:      "<p>guard</p>",
	}
	activity := remoteCreateActivityForNote(t, env.remoteActorID, note, "https://remote.example/activities/create-guard-1")

	env.handler.createRemoteCreateNotifications(context.Background(), nil, env.local, note, status)
	env.handler.createRemoteCreateNotifications(context.Background(), activity, nil, note, status)
	env.handler.createRemoteCreateNotifications(context.Background(), activity, env.local, nil, status)
	env.handler.createRemoteCreateNotifications(context.Background(), activity, env.local, note, nil)

	namelessTarget := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "not-an-actor-url", Type: activitypub.PersonType},
	}
	env.handler.createRemoteCreateNotifications(context.Background(), activity, namelessTarget, note, status)

	selfActivity := remoteCreateActivityForNote(t, env.local.ID, note, "https://remote.example/activities/create-self-guard-1")
	env.handler.createRemoteCreateNotifications(context.Background(), selfActivity, env.local, note, status)

	result, err := env.handler.notificationRepository.GetUserNotifications(
		context.Background(),
		"alice",
		interfaces.PaginationOptions{Limit: 10},
	)
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestInboxHandler_Project20_RemoteReplyParentFailureBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/users/bob/statuses/reply-missing-1",
			Type:      activitypub.NoteType,
			InReplyTo: env.cfg.BaseURL() + "/users/alice/statuses/missing-parent",
		},
		AttributedTo: env.remoteActorID,
	}

	env.handler.statusRepository = nil
	parent, parentID := env.handler.remoteCreateReplyParent(context.Background(), note, env.local)
	require.Nil(t, parent)
	require.Empty(t, parentID)

	env.handler.statusRepository = &project20ErroringStatusRepo{err: fmt.Errorf("backend unavailable")}
	parent, parentID = env.handler.remoteCreateReplyParent(context.Background(), note, env.local)
	require.Nil(t, parent)
	require.Empty(t, parentID)

	env.handler.statusRepository = newProject20StatusRepo(&models.Status{
		StatusID:       "missing-parent",
		AuthorID:       "https://other.localhost/users/mallory",
		AuthorUsername: "mallory",
	})
	parent, parentID = env.handler.remoteCreateReplyParent(context.Background(), note, env.local)
	require.Nil(t, parent)
	require.Empty(t, parentID)
}

func TestInboxHandler_Project20_IdempotencyRepositoryErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	recorder := &project20ErroringInboxProcessingRecorder{err: fmt.Errorf("receipt unavailable")}
	env.handler.inboxProcessingRepository = recorder

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/activities/create-error-branch-1",
			Type: activitypub.CreateType,
		},
		Actor:  env.remoteActorID,
		Object: "https://remote.example/objects/error-branch-1",
	}

	shouldProcess, err := env.handler.shouldProcessInboundActivityTarget(context.Background(), activity, env.local)
	require.Error(t, err)
	require.False(t, shouldProcess)

	env.handler.releaseInboundActivityTargetClaim(context.Background(), activity, env.local)
	require.Equal(t, 1, recorder.forgetCalls)
}

type project20FailingNotificationRepo struct {
	interfaces.NotificationRepository
	err     error
	created []*models.Notification
}

func (r *project20FailingNotificationRepo) CreateNotification(_ context.Context, notification *models.Notification) error {
	r.created = append(r.created, notification)
	return r.err
}

type project20ErroringStatusRepo struct {
	interfaces.StatusRepository
	err error
}

func (r *project20ErroringStatusRepo) GetStatus(_ context.Context, _ string) (*models.Status, error) {
	return nil, r.err
}

type project20ErroringInboxProcessingRecorder struct {
	err         error
	forgetCalls int
}

func (r *project20ErroringInboxProcessingRecorder) TryRecordTarget(_ context.Context, _, _, _ string) (bool, error) {
	return false, r.err
}

func (r *project20ErroringInboxProcessingRecorder) ForgetTarget(_ context.Context, _, _ string) error {
	r.forgetCalls++
	return r.err
}
