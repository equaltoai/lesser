package scheduled

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeScheduledRepo struct {
	nextID string

	createCalls []*storage.ScheduledStatus
	createErr   error

	getByID map[string]*storage.ScheduledStatus
	getErr  error

	listCalls []struct {
		username string
		limit    int
		cursor   string
	}
	list       []*storage.ScheduledStatus
	nextCursor string
	listErr    error

	updateCalls []*storage.ScheduledStatus
	updateErr   error

	deleteCalls []string
	deleteErr   error

	mediaByScheduledID map[string][]*models.Media
	getMediaErr        error
}

func (r *fakeScheduledRepo) CreateScheduledStatus(_ context.Context, scheduled *storage.ScheduledStatus) error {
	r.createCalls = append(r.createCalls, scheduled)
	if r.createErr != nil {
		return r.createErr
	}
	if scheduled.ID == "" {
		id := r.nextID
		if id == "" {
			id = "sched1"
		}
		scheduled.ID = id
	}
	if r.getByID != nil {
		r.getByID[scheduled.ID] = scheduled
	}
	return nil
}

func (r *fakeScheduledRepo) GetScheduledStatus(_ context.Context, id string) (*storage.ScheduledStatus, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.getByID == nil {
		return nil, stderrors.New("not found")
	}
	status, ok := r.getByID[id]
	if !ok {
		return nil, stderrors.New("not found")
	}
	return status, nil
}

func (r *fakeScheduledRepo) GetScheduledStatuses(_ context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	r.listCalls = append(r.listCalls, struct {
		username string
		limit    int
		cursor   string
	}{username: username, limit: limit, cursor: cursor})
	if r.listErr != nil {
		return nil, "", r.listErr
	}
	return r.list, r.nextCursor, nil
}

func (r *fakeScheduledRepo) UpdateScheduledStatus(_ context.Context, scheduled *storage.ScheduledStatus) error {
	r.updateCalls = append(r.updateCalls, scheduled)
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.getByID != nil {
		r.getByID[scheduled.ID] = scheduled
	}
	return nil
}

func (r *fakeScheduledRepo) DeleteScheduledStatus(_ context.Context, id string) error {
	r.deleteCalls = append(r.deleteCalls, id)
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if r.getByID != nil {
		delete(r.getByID, id)
	}
	return nil
}

func (r *fakeScheduledRepo) GetScheduledStatusMedia(_ context.Context, scheduledStatusID string) ([]*models.Media, error) {
	if r.getMediaErr != nil {
		return nil, r.getMediaErr
	}
	if r.mediaByScheduledID == nil {
		return nil, nil
	}
	return r.mediaByScheduledID[scheduledStatusID], nil
}

type fakeMediaRepo struct {
	items   map[string]*models.Media
	errByID map[string]error
	calls   []string
}

func (r *fakeMediaRepo) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	r.calls = append(r.calls, mediaID)
	if err := r.errByID[mediaID]; err != nil {
		return nil, err
	}
	media, ok := r.items[mediaID]
	if !ok {
		return nil, stderrors.New("not found")
	}
	return media, nil
}

func TestService_CreateScheduledStatus_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("scheduled_time_too_soon", func(t *testing.T) {
		repo := &fakeScheduledRepo{}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			ScheduledAt: time.Now().Add(1 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrScheduledTimeInPast)
		require.Empty(t, repo.createCalls)
	})

	t.Run("too_many_media_ids", func(t *testing.T) {
		repo := &fakeScheduledRepo{}
		mediaRepo := &fakeMediaRepo{}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			MediaIDs:    []string{"m1", "m2", "m3", "m4", "m5"},
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrValidationFailed)
		require.Empty(t, repo.createCalls)
	})

	t.Run("invalid_media_id_format", func(t *testing.T) {
		repo := &fakeScheduledRepo{}
		mediaRepo := &fakeMediaRepo{}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			MediaIDs:    []string{"", "m2"},
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrValidationFailed)
		require.Empty(t, repo.createCalls)
	})

	t.Run("media_lookup_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{},
			errByID: map[string]error{
				"m1": stderrors.New("boom"),
			},
		}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			MediaIDs:    []string{"m1"},
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrMediaAttachmentNotFound)
		require.Empty(t, repo.createCalls)
	})

	t.Run("media_not_ready", func(t *testing.T) {
		repo := &fakeScheduledRepo{}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{
				"m1": {MediaID: "m1", Status: "processing"},
			},
		}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			MediaIDs:    []string{"m1"},
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrMediaAttachmentNotReady)
	})

	t.Run("media_expired", func(t *testing.T) {
		repo := &fakeScheduledRepo{}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{
				"m1": {MediaID: "m1", Status: "ready", ExpiresAt: time.Now().Unix() - 1},
			},
		}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			MediaIDs:    []string{"m1"},
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrMediaAttachmentExpired)
	})

	t.Run("repository_create_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{createErr: stderrors.New("db down")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.ErrorIs(t, err, svcErrors.ErrCreateScheduledStatus)
	})

	t.Run("success_defaults_visibility_and_emits_event", func(t *testing.T) {
		repo := &fakeScheduledRepo{getByID: map[string]*storage.ScheduledStatus{}}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{
				"m1": {MediaID: "m1", Status: "ready", ContentType: "image/png"},
				"m2": {MediaID: "m2", Status: "completed", ContentType: "image/jpeg"},
			},
		}
		pub := &fakePublisher{}
		svc := NewService(repo, nil, mediaRepo, pub, zap.NewNop(), "example.com")

		result, err := svc.CreateScheduledStatus(ctx, &CreateScheduledStatusCommand{
			Username:    "alice",
			Status:      "hi",
			MediaIDs:    []string{"m1", "m2"},
			Visibility:  "",
			ScheduledAt: time.Now().Add(10 * time.Minute),
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.ScheduledStatus)
		assert.Equal(t, "alice", result.ScheduledStatus.Username)
		assert.Equal(t, "public", result.ScheduledStatus.Visibility)
		assert.False(t, result.ScheduledStatus.Published)
		assert.NotEmpty(t, result.ScheduledStatus.ID)
		require.Len(t, result.MediaAttachments, 2)
		require.Len(t, result.Events, 1)
		require.Len(t, pub.userCalls, 1)
	})
}

func TestService_GetScheduledStatus_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("repository_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{getErr: stderrors.New("db down")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.GetScheduledStatus(ctx, &GetScheduledStatusQuery{ID: "sched1"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("ownership_mismatch_is_hidden", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{
				"sched1": {ID: "sched1", Username: "bob"},
			},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.GetScheduledStatus(ctx, &GetScheduledStatusQuery{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("already_published_is_hidden", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{
				"sched1": {ID: "sched1", Username: "alice", Published: true},
			},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.GetScheduledStatus(ctx, &GetScheduledStatusQuery{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("media_retrieval_error_is_non_fatal", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{
				"sched1": {ID: "sched1", Username: "alice", MediaIDs: []string{"m1"}},
			},
		}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{},
			errByID: map[string]error{
				"m1": stderrors.New("boom"),
			},
		}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		result, err := svc.GetScheduledStatus(ctx, &GetScheduledStatusQuery{ID: "sched1", Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Nil(t, result.MediaAttachments)
		assert.Equal(t, []string{"m1"}, mediaRepo.calls)
	})

	t.Run("filters_media_to_ready_or_completed", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{
				"sched1": {ID: "sched1", Username: "alice", MediaIDs: []string{"m1", "m2"}},
			},
		}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{
				"m1": {MediaID: "m1", Status: "ready"},
				"m2": {MediaID: "m2", Status: "processing"},
			},
		}
		svc := NewService(repo, nil, mediaRepo, nil, zap.NewNop(), "example.com")

		result, err := svc.GetScheduledStatus(ctx, &GetScheduledStatusQuery{ID: "sched1", Username: "alice"})
		require.NoError(t, err)
		require.Len(t, result.MediaAttachments, 1)
		assert.Equal(t, "m1", result.MediaAttachments[0].MediaID)
	})
}

func TestService_ListScheduledStatuses_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("repository_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{listErr: stderrors.New("boom")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.ListScheduledStatuses(ctx, &ListScheduledStatusesQuery{Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatuses)
	})

	t.Run("filters_published_and_sets_pagination", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			list: []*storage.ScheduledStatus{
				{ID: "s1", Username: "alice", Published: false},
				{ID: "s2", Username: "alice", Published: true},
			},
			nextCursor: "next",
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		result, err := svc.ListScheduledStatuses(ctx, &ListScheduledStatusesQuery{Username: "alice"})
		require.NoError(t, err)
		require.Len(t, result.ScheduledStatuses, 1)
		assert.Equal(t, "s1", result.ScheduledStatuses[0].ID)
		require.NotNil(t, result.Pagination)
		assert.Equal(t, "next", result.Pagination.NextCursor)
		assert.True(t, result.Pagination.HasMore)
		assert.Equal(t, []string{"s1"}, result.Pagination.Items)

		require.Len(t, repo.listCalls, 1)
		assert.Equal(t, "alice", repo.listCalls[0].username)
		assert.Equal(t, 20, repo.listCalls[0].limit)
	})
}

func TestService_UpdateScheduledStatus_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("get_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{getErr: stderrors.New("boom")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("ownership_mismatch_hidden", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{
				"sched1": {ID: "sched1", Username: "bob"},
			},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("published_cannot_update", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{
				"sched1": {ID: "sched1", Username: "alice", Published: true},
			},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrUpdateStatus)
	})

	t.Run("no_changes_returns_existing_without_update", func(t *testing.T) {
		existing := &storage.ScheduledStatus{ID: "sched1", Username: "alice", Published: false}
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": existing},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		result, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.NoError(t, err)
		require.Same(t, existing, result.ScheduledStatus)
		assert.Empty(t, repo.updateCalls)
	})

	t.Run("invalid_scheduled_time", func(t *testing.T) {
		existing := &storage.ScheduledStatus{ID: "sched1", Username: "alice", Published: false}
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": existing},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		badTime := time.Now().Add(1 * time.Minute)
		_, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{
			ID:          "sched1",
			Username:    "alice",
			ScheduledAt: &badTime,
		})
		require.ErrorIs(t, err, svcErrors.ErrScheduledTimeInPast)
		assert.Empty(t, repo.updateCalls)
	})

	t.Run("update_error", func(t *testing.T) {
		existing := &storage.ScheduledStatus{ID: "sched1", Username: "alice", Published: false}
		repo := &fakeScheduledRepo{
			getByID:    map[string]*storage.ScheduledStatus{"sched1": existing},
			updateErr:  stderrors.New("boom"),
			nextCursor: "",
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		nextTime := time.Now().Add(20 * time.Minute)
		_, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{ID: "sched1", Username: "alice", ScheduledAt: &nextTime})
		require.ErrorIs(t, err, svcErrors.ErrUpdateStatus)
	})

	t.Run("success_updates_and_emits_event", func(t *testing.T) {
		existing := &storage.ScheduledStatus{ID: "sched1", Username: "alice", Published: false, MediaIDs: []string{"m1", "m2"}}
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": existing},
		}
		mediaRepo := &fakeMediaRepo{
			items: map[string]*models.Media{
				"m1": {MediaID: "m1", Status: "ready"},
				"m2": {MediaID: "m2", Status: "processing"},
			},
		}
		pub := &fakePublisher{}
		svc := NewService(repo, nil, mediaRepo, pub, zap.NewNop(), "example.com")

		nextTime := time.Now().Add(20 * time.Minute)
		result, err := svc.UpdateScheduledStatus(ctx, &UpdateScheduledStatusCommand{ID: "sched1", Username: "alice", ScheduledAt: &nextTime})
		require.NoError(t, err)
		require.Len(t, repo.updateCalls, 1)
		require.NotNil(t, result)
		require.NotNil(t, result.ScheduledStatus)
		require.Len(t, result.Events, 1)
		require.Len(t, result.MediaAttachments, 1)
		assert.Equal(t, "m1", result.MediaAttachments[0].MediaID)
	})
}

func TestService_DeleteScheduledStatus_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("get_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{getErr: stderrors.New("boom")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.DeleteScheduledStatus(ctx, &DeleteScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("published_cannot_delete", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": {ID: "sched1", Username: "alice", Published: true}},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.DeleteScheduledStatus(ctx, &DeleteScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrDeleteStatus)
	})

	t.Run("delete_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID:   map[string]*storage.ScheduledStatus{"sched1": {ID: "sched1", Username: "alice", Published: false}},
			deleteErr: stderrors.New("boom"),
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.DeleteScheduledStatus(ctx, &DeleteScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.ErrorIs(t, err, svcErrors.ErrDeleteStatus)
	})

	t.Run("success_deletes_and_emits_event", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": {ID: "sched1", Username: "alice", Published: false}},
		}
		pub := &fakePublisher{}
		svc := NewService(repo, nil, nil, pub, zap.NewNop(), "example.com")

		err := svc.DeleteScheduledStatus(ctx, &DeleteScheduledStatusCommand{ID: "sched1", Username: "alice"})
		require.NoError(t, err)
		assert.Equal(t, []string{"sched1"}, repo.deleteCalls)
		require.Len(t, pub.userCalls, 1)
		assert.Equal(t, "scheduled_status.deleted", pub.userCalls[0].event.Type)
	})
}

func TestService_PublishScheduledStatus_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("get_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{getErr: stderrors.New("boom")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.PublishScheduledStatus(ctx, &PublishScheduledStatusCommand{ID: "sched1"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("already_published_is_hidden", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": {ID: "sched1", Username: "alice", Published: true}},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.PublishScheduledStatus(ctx, &PublishScheduledStatusCommand{ID: "sched1"})
		require.ErrorIs(t, err, svcErrors.ErrGetStatus)
	})

	t.Run("update_error", func(t *testing.T) {
		existing := &storage.ScheduledStatus{ID: "sched1", Username: "alice", Published: false}
		repo := &fakeScheduledRepo{
			getByID:   map[string]*storage.ScheduledStatus{"sched1": existing},
			updateErr: stderrors.New("boom"),
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		err := svc.PublishScheduledStatus(ctx, &PublishScheduledStatusCommand{ID: "sched1"})
		require.ErrorIs(t, err, svcErrors.ErrUpdateStatus)
	})

	t.Run("success_marks_published_and_emits_event", func(t *testing.T) {
		existing := &storage.ScheduledStatus{ID: "sched1", Username: "alice", Published: false}
		repo := &fakeScheduledRepo{
			getByID: map[string]*storage.ScheduledStatus{"sched1": existing},
		}
		pub := &fakePublisher{}
		svc := NewService(repo, nil, nil, pub, zap.NewNop(), "example.com")

		err := svc.PublishScheduledStatus(ctx, &PublishScheduledStatusCommand{ID: "sched1"})
		require.NoError(t, err)
		assert.True(t, existing.Published)
		require.NotNil(t, existing.PublishedAt)
		require.Len(t, repo.updateCalls, 1)
		require.Len(t, pub.userCalls, 1)
		assert.Equal(t, "scheduled_status.published", pub.userCalls[0].event.Type)
	})
}

func TestService_GetScheduledMediaAttachments_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("repository_error", func(t *testing.T) {
		repo := &fakeScheduledRepo{getMediaErr: stderrors.New("boom")}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		_, err := svc.GetScheduledMediaAttachments(ctx, "sched1")
		require.ErrorIs(t, err, svcErrors.ErrGetStatuses)
	})

	t.Run("success_returns_media", func(t *testing.T) {
		repo := &fakeScheduledRepo{
			mediaByScheduledID: map[string][]*models.Media{
				"sched1": {
					{MediaID: "m1", Status: "ready"},
				},
			},
		}
		svc := NewService(repo, nil, nil, nil, zap.NewNop(), "example.com")

		items, err := svc.GetScheduledMediaAttachments(ctx, "sched1")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "m1", items[0].MediaID)
	})
}

