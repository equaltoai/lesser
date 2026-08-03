package federation

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

type stubRemoteNoteObjectRepo struct {
	created   []any
	createErr error
}

func (s *stubRemoteNoteObjectRepo) CreateObject(_ context.Context, object any) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, object)
	return nil
}

type stubRemoteNoteStatusRepo struct {
	created   []*models.Status
	createErr error
	existing  *models.Status
	getErr    error
}

func (s *stubRemoteNoteStatusRepo) CreateStatus(_ context.Context, status *models.Status) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, status)
	return nil
}

func (s *stubRemoteNoteStatusRepo) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.existing != nil && s.existing.StatusID == statusID {
		return s.existing, nil
	}
	return nil, storage.ErrNotFound
}

func TestMaterializeRemoteNote(t *testing.T) {
	ctx := context.Background()
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/steward/statuses/seed-1",
			Type: activitypub.NoteType,
			To:   []string{activitypub.PublicAddress},
			CC:   []string{"https://remote.example/users/steward/followers"},
		},
		AttributedTo: "https://remote.example/users/steward",
		Content:      "seed",
		Visibility:   models.VisibilityPublic,
	}

	t.Run("requires status repository", func(t *testing.T) {
		status, err := MaterializeRemoteNote(ctx, nil, nil, note, "example.com")
		require.Error(t, err)
		assert.Nil(t, status)
		assert.ErrorContains(t, err, "status repository not configured")
	})

	t.Run("requires note", func(t *testing.T) {
		status, err := MaterializeRemoteNote(ctx, nil, &stubRemoteNoteStatusRepo{}, nil, "example.com")
		require.Error(t, err)
		assert.Nil(t, status)
		assert.ErrorContains(t, err, "remote note is required")
	})

	t.Run("ignores object condition-failed when projecting remote note", func(t *testing.T) {
		objectRepo := &stubRemoteNoteObjectRepo{createErr: dynamormerrors.ErrConditionFailed}
		statusRepo := &stubRemoteNoteStatusRepo{}

		status, err := MaterializeRemoteNote(ctx, objectRepo, statusRepo, note, "example.com")
		require.NoError(t, err)
		require.NotNil(t, status)
		require.Len(t, statusRepo.created, 1)
		assert.Equal(t, models.CanonicalStatusIDForDomain(note.ID, "example.com"), status.StatusID)
		assert.Equal(t, note.ID, status.Note.ID)
		assert.Equal(t, models.VisibilityPublic, status.Visibility)
		assert.Equal(t, "steward@remote.example", status.AuthorUsername)
	})

	t.Run("returns object storage errors that are not condition-failed", func(t *testing.T) {
		status, err := MaterializeRemoteNote(ctx, &stubRemoteNoteObjectRepo{createErr: errors.New("object write failed")}, &stubRemoteNoteStatusRepo{}, note, "example.com")
		require.Error(t, err)
		assert.Nil(t, status)
		assert.ErrorContains(t, err, "object write failed")
	})

	t.Run("returns existing status when create is condition-failed", func(t *testing.T) {
		existing := BuildCanonicalRemoteStatus(note, "example.com")
		require.NotNil(t, existing)
		require.NoError(t, existing.UpdateKeys())

		statusRepo := &stubRemoteNoteStatusRepo{
			createErr: dynamormerrors.ErrConditionFailed,
			existing:  existing,
		}

		status, err := MaterializeRemoteNote(ctx, nil, statusRepo, note, "example.com")
		require.NoError(t, err)
		assert.Same(t, existing, status)
	})

	t.Run("returns lookup error when existing status cannot be loaded", func(t *testing.T) {
		statusRepo := &stubRemoteNoteStatusRepo{
			createErr: dynamormerrors.ErrConditionFailed,
			getErr:    errors.New("load failed"),
		}

		status, err := MaterializeRemoteNote(ctx, nil, statusRepo, note, "example.com")
		require.Error(t, err)
		assert.Nil(t, status)
		assert.ErrorContains(t, err, "load failed")
	})

	t.Run("returns status create errors that are not condition-failed", func(t *testing.T) {
		status, err := MaterializeRemoteNote(ctx, nil, &stubRemoteNoteStatusRepo{createErr: errors.New("status write failed")}, note, "example.com")
		require.Error(t, err)
		assert.Nil(t, status)
		assert.ErrorContains(t, err, "status write failed")
	})

	t.Run("rejects remote notes that cannot project to a canonical status", func(t *testing.T) {
		status, err := MaterializeRemoteNote(ctx, nil, &stubRemoteNoteStatusRepo{}, &activitypub.Note{
			BaseObject: activitypub.BaseObject{Type: activitypub.NoteType},
		}, "example.com")
		require.Error(t, err)
		assert.Nil(t, status)
		assert.ErrorContains(t, err, "canonical remote status payload is invalid")
	})
}
