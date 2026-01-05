package threads

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestGetThreadContext_round27_error_paths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

	t.Run("missing_note_id", func(t *testing.T) {
		_, err := service.GetThreadContext(ctx, ThreadContextQuery{})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingRequiredParam)
	})

	t.Run("requested_note_not_found", func(t *testing.T) {
		objectRepo.On("GetObject", ctx, "missing").Return(nil, errors.New("boom")).Once()

		_, err := service.GetThreadContext(ctx, ThreadContextQuery{NoteID: "missing"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrThreadNotFound)
	})

	t.Run("thread_context_repo_error", func(t *testing.T) {
		note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}}
		objectRepo.On("GetObject", ctx, "note-1").Return(note, nil).Once()
		threadRepo.On("GetThreadContext", ctx, "note-1").Return(nil, errors.New("boom")).Once()

		_, err := service.GetThreadContext(ctx, ThreadContextQuery{NoteID: "note-1"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGetThreadContext)
	})

	t.Run("thread_context_nil_sets_sync_none_and_last_activity_now", func(t *testing.T) {
		note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-2", Type: "Note"}}
		objectRepo.On("GetObject", ctx, "note-2").Return(note, nil).Once()
		threadRepo.On("GetThreadContext", ctx, "note-2").Return(nil, nil).Once()

		start := time.Now()
		result, err := service.GetThreadContext(ctx, ThreadContextQuery{NoteID: "note-2"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, SyncStatusNone, result.SyncStatus)
		assert.True(t, result.LastActivity.After(start.Add(-250*time.Millisecond)))
	})
}

func TestService_calculateSyncStatus_round27_coverage(t *testing.T) {
	t.Parallel()

	service := &Service{}

	assert.Equal(t, SyncStatusNone, service.calculateSyncStatus(nil))
	assert.Equal(t, SyncStatusComplete, service.calculateSyncStatus(&repositories.ThreadContextResult{MissingCount: 0}))
	assert.Equal(t, SyncStatusPartial, service.calculateSyncStatus(&repositories.ThreadContextResult{
		MissingCount: 1,
		Nodes:        []*models.ThreadNode{{}},
	}))
	assert.Equal(t, SyncStatusNone, service.calculateSyncStatus(&repositories.ThreadContextResult{MissingCount: 1}))
}

func TestService_fetchRemoteNote_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

	signingActor := &activitypub.Actor{PreferredUsername: "system"}

	t.Run("non_map_object_is_not_a_note", func(t *testing.T) {
		federation.On("FetchObject", ctx, "https://remote.example/n1", signingActor).Return("not-a-map", nil).Once()

		_, err := service.fetchRemoteNote(ctx, "https://remote.example/n1", signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotANote)
	})

	t.Run("wrong_type_is_not_a_note", func(t *testing.T) {
		federation.On("FetchObject", ctx, "https://remote.example/n2", signingActor).Return(map[string]any{
			"id":   "https://remote.example/n2",
			"type": "Article",
		}, nil).Once()

		_, err := service.fetchRemoteNote(ctx, "https://remote.example/n2", signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotANote)
	})

	t.Run("stores_note_locally_even_if_storage_fails_and_defaults_visibility", func(t *testing.T) {
		federation.On("FetchObject", ctx, "https://remote.example/n3", signingActor).Return(map[string]any{
			"id":           "https://remote.example/n3",
			"type":         "Note",
			"content":      "hello",
			"attributedTo": "https://remote.example/users/alice",
			"inReplyTo":    "https://remote.example/n0",
			"published":    "not-a-time",
			"sensitive":    true,
		}, nil).Once()
		objectRepo.On("CreateObject", ctx, mock.Anything).Return(errors.New("boom")).Once()

		note, err := service.fetchRemoteNote(ctx, "https://remote.example/n3", signingActor)
		require.NoError(t, err)
		require.NotNil(t, note)
		assert.Equal(t, "https://remote.example/n3", note.ID)
		assert.Equal(t, "public", note.Visibility)
		assert.Equal(t, "https://remote.example/n0", note.InReplyTo)
		assert.True(t, note.Sensitive)
		assert.Nil(t, note.Published)
	})
}

func TestService_getSigningActor_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

	actorRepo.On("GetActorByUsername", ctx, "system").Return(&activitypub.Actor{PreferredUsername: "system"}, nil).Once()
	actorRepo.On("GetActorByUsername", ctx, "alice").Return(&activitypub.Actor{PreferredUsername: "alice"}, nil).Once()

	systemActor, err := service.getSigningActor(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "system", systemActor.PreferredUsername)

	alice, err := service.getSigningActor(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", alice.PreferredUsername)
}

func TestService_parseRepliesCollection_round27_coverage(t *testing.T) {
	t.Parallel()

	service := &Service{}

	_, err := service.parseRepliesCollection("not-a-map")
	require.Error(t, err)

	_, err = service.parseRepliesCollection(map[string]any{})
	require.Error(t, err)

	_, err = service.parseRepliesCollection(map[string]any{"type": "Unknown"})
	require.Error(t, err)

	urls, err := service.parseRepliesCollection(map[string]any{
		"type":         "Collection",
		"orderedItems": []any{"https://remote.example/r1", map[string]any{"id": "https://remote.example/r2"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://remote.example/r1", "https://remote.example/r2"}, urls)

	urls, err = service.parseRepliesCollection(map[string]any{
		"type":  "CollectionPage",
		"items": []any{"https://remote.example/r3"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://remote.example/r3"}, urls)

	_, err = service.parseRepliesCollection(map[string]any{"type": "Collection"})
	require.Error(t, err)
}

func TestSyncRemoteThread_round27_additional_paths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

	t.Run("missing_note_url", func(t *testing.T) {
		_, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingRequiredParam)
	})

	t.Run("signing_actor_error", func(t *testing.T) {
		actorRepo.On("GetActorByUsername", ctx, "system").Return(nil, errors.New("boom")).Once()

		_, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{NoteURL: "https://remote.example/n1"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteAuthFailed)
	})

	t.Run("fetch_root_error_returns_result_and_error", func(t *testing.T) {
		signingActor := &activitypub.Actor{PreferredUsername: "system"}
		actorRepo.On("GetActorByUsername", ctx, "system").Return(signingActor, nil).Once()
		federation.On("FetchObject", ctx, "https://remote.example/n2", signingActor).Return(nil, errors.New("boom")).Once()

		result, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{NoteURL: "https://remote.example/n2"})
		require.Error(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, ErrFetchRemoteNote)
	})

	t.Run("sync_in_progress_returns_specific_error", func(t *testing.T) {
		signingActor := &activitypub.Actor{PreferredUsername: "system"}
		actorRepo.On("GetActorByUsername", ctx, "system").Return(signingActor, nil).Once()
		federation.On("FetchObject", ctx, "https://remote.example/n3", signingActor).Return(map[string]any{
			"id":           "https://remote.example/n3",
			"type":         "Note",
			"content":      "hi",
			"attributedTo": "https://remote.example/users/alice",
		}, nil).Once()
		objectRepo.On("CreateObject", ctx, mock.Anything).Return(nil).Once()

		syncRecord := models.NewThreadSync("https://remote.example/n3")
		syncRecord.SyncStatus = "syncing"
		threadRepo.On("GetThreadSync", ctx, "https://remote.example/n3").Return(syncRecord, nil).Once()

		result, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{NoteURL: "https://remote.example/n3"})
		require.Error(t, err)
		require.NotNil(t, result)
		assert.Equal(t, SyncStatusSyncing, result.SyncStatus)
		assert.ErrorIs(t, err, ErrSyncInProgress)
	})

	t.Run("recently_completed_skips_when_not_forced", func(t *testing.T) {
		signingActor := &activitypub.Actor{PreferredUsername: "system"}
		actorRepo.On("GetActorByUsername", ctx, "system").Return(signingActor, nil).Once()
		federation.On("FetchObject", ctx, "https://remote.example/n4", signingActor).Return(map[string]any{
			"id":           "https://remote.example/n4",
			"type":         "Note",
			"content":      "hi",
			"attributedTo": "https://remote.example/users/alice",
		}, nil).Once()
		objectRepo.On("CreateObject", ctx, mock.Anything).Return(nil).Once()

		syncRecord := models.NewThreadSync("https://remote.example/n4")
		syncRecord.MarkCompleted()
		threadRepo.On("GetThreadSync", ctx, "https://remote.example/n4").Return(syncRecord, nil).Once()

		result, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{NoteURL: "https://remote.example/n4"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, SyncStatusComplete, result.SyncStatus)
	})

	t.Run("partial_when_reply_save_fails", func(t *testing.T) {
		threadRepo := &MockThreadRepository{}
		statusRepo := &MockStatusRepository{}
		objectRepo := &MockObjectRepository{}
		actorRepo := &MockActorRepository{}
		federation := &MockFederationClient{}
		publisher := &MockPublisher{}
		logger := zaptest.NewLogger(t)

		service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

		signingActor := &activitypub.Actor{PreferredUsername: "system"}
		actorRepo.On("GetActorByUsername", ctx, "system").Return(signingActor, nil).Once()
		federation.On("FetchObject", ctx, "https://remote.example/n5", signingActor).Return(map[string]any{
			"id":           "https://remote.example/n5",
			"type":         "Note",
			"content":      "hi",
			"attributedTo": "https://remote.example/users/alice",
		}, nil).Once()
		objectRepo.On("CreateObject", ctx, mock.Anything).Return(nil).Once()

		threadRepo.On("GetThreadSync", ctx, "https://remote.example/n5").Return(nil, nil).Once()
		threadRepo.On("SaveThreadSync", ctx, mock.Anything).Return(nil)

		replyStatus := &models.Status{
			StatusID:    "https://remote.example/reply-1",
			AuthorID:    "https://remote.example/users/bob",
			Content:     "Reply",
			InReplyToID: "https://remote.example/n5",
			PublishedAt: time.Now(),
			Visibility:  "public",
		}
		statusRepo.On("GetReplies", ctx, "https://remote.example/n5", interfaces.PaginationOptions{Limit: 100}).Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{replyStatus}}, nil).Once()
		statusRepo.On("GetReplies", ctx, replyStatus.StatusID, interfaces.PaginationOptions{Limit: 100}).Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{}}, nil).Once()

		threadRepo.On("SaveThreadNode", ctx, mock.MatchedBy(func(node *models.ThreadNode) bool {
			return node != nil && node.StatusID == replyStatus.StatusID
		})).Return(errors.New("boom")).Once()
		threadRepo.On("SaveThreadNode", ctx, mock.Anything).Return(nil).Maybe()

		result, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{NoteURL: "https://remote.example/n5", Depth: 1})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, SyncStatusPartial, result.SyncStatus)
		assert.Greater(t, result.SyncedPosts, 0)
		assert.Len(t, result.Errors, 1)
	})

	t.Run("failed_when_root_node_save_fails", func(t *testing.T) {
		threadRepo := &MockThreadRepository{}
		statusRepo := &MockStatusRepository{}
		objectRepo := &MockObjectRepository{}
		actorRepo := &MockActorRepository{}
		federation := &MockFederationClient{}
		publisher := &MockPublisher{}
		logger := zaptest.NewLogger(t)

		service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

		signingActor := &activitypub.Actor{PreferredUsername: "system"}
		actorRepo.On("GetActorByUsername", ctx, "system").Return(signingActor, nil).Once()
		federation.On("FetchObject", ctx, "https://remote.example/n6", signingActor).Return(map[string]any{
			"id":           "https://remote.example/n6",
			"type":         "Note",
			"content":      "hi",
			"attributedTo": "https://remote.example/users/alice",
		}, nil).Once()
		objectRepo.On("CreateObject", ctx, mock.Anything).Return(nil).Once()

		threadRepo.On("GetThreadSync", ctx, "https://remote.example/n6").Return(nil, nil).Once()
		threadRepo.On("SaveThreadSync", ctx, mock.Anything).Return(nil)
		threadRepo.On("SaveThreadNode", ctx, mock.Anything).Return(errors.New("boom")).Maybe()
		statusRepo.On("GetReplies", ctx, "https://remote.example/n6", interfaces.PaginationOptions{Limit: 100}).Return(nil, errors.New("boom")).Once()

		result, err := service.SyncRemoteThread(ctx, SyncRemoteThreadCommand{NoteURL: "https://remote.example/n6", Depth: 1})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, SyncStatusFailed, result.SyncStatus)
		assert.False(t, result.Success)
		assert.Equal(t, 0, result.SyncedPosts)
		assert.GreaterOrEqual(t, result.ErrorCount, 1)
	})
}

func TestSyncMissingReplies_round27_additional_paths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(threadRepo, statusRepo, objectRepo, actorRepo, federation, publisher, logger, "example.com")

	t.Run("note_not_found_returns_empty_result", func(t *testing.T) {
		objectRepo.On("GetObject", ctx, "missing").Return(nil, errors.New("boom")).Once()

		result, err := service.SyncMissingReplies(ctx, SyncMissingRepliesCommand{NoteID: "missing"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.SyncedReplies)
	})

	t.Run("failed_missing_reply_retry_updates_records_and_result", func(t *testing.T) {
		root := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "root", Type: "Note"}}
		objectRepo.On("GetObject", ctx, "root").Return(root, nil).Once()

		missing := models.NewMissingReply("root", "parent", "https://remote.example/reply-404")
		missing.Status = models.MissingReplyStatusFailed
		past := time.Now().Add(-time.Minute)
		missing.NextRetryAt = &past

		threadRepo.On("GetMissingReplies", ctx, "root").Return([]*models.MissingReply{missing}, nil).Once()
		actorRepo.On("GetActorByUsername", ctx, "system").Return(&activitypub.Actor{PreferredUsername: "system"}, nil).Once()
		threadRepo.On("SaveMissingReply", ctx, mock.Anything).Return(nil).Twice()

		federation.On("FetchObject", ctx, "https://remote.example/reply-404", mock.Anything).Return(nil, errors.New("404 not found")).Once()

		result, err := service.SyncMissingReplies(ctx, SyncMissingRepliesCommand{NoteID: "root", ViewerID: ""})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, 0, result.SyncedReplies)
		assert.Len(t, result.Errors, 1)
	})
}
