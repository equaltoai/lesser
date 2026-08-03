package routing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type recordingStatusRepository struct {
	interfaces.StatusRepository
	statuses   map[string]*models.Status
	created    []*models.Status
	updated    []*models.Status
	deletedIDs []string
	createErr  error
	getErr     error
	updateErr  error
	deleteErr  error
}

func (r *recordingStatusRepository) CreateStatus(_ context.Context, status *models.Status) error {
	cloned := cloneRecordedStatus(status)
	if cloned != nil {
		r.created = append(r.created, cloned)
		if r.statuses == nil {
			r.statuses = map[string]*models.Status{}
		}
		r.statuses[cloned.StatusID] = cloneRecordedStatus(cloned)
	}

	return r.createErr
}

func (r *recordingStatusRepository) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.statuses == nil {
		return nil, storage.ErrNotFound
	}

	status, ok := r.statuses[statusID]
	if !ok {
		return nil, storage.ErrNotFound
	}

	return cloneRecordedStatus(status), nil
}

func (r *recordingStatusRepository) UpdateStatus(_ context.Context, status *models.Status) error {
	if r.updateErr != nil {
		return r.updateErr
	}

	cloned := cloneRecordedStatus(status)
	if cloned != nil {
		_ = cloned.UpdateKeys()
		r.updated = append(r.updated, cloned)
		if r.statuses == nil {
			r.statuses = map[string]*models.Status{}
		}
		r.statuses[cloned.StatusID] = cloneRecordedStatus(cloned)
	}

	return nil
}

func (r *recordingStatusRepository) DeleteStatus(_ context.Context, statusID string) error {
	r.deletedIDs = append(r.deletedIDs, statusID)
	if r.deleteErr != nil {
		return r.deleteErr
	}

	if existing, ok := r.statuses[statusID]; ok && existing != nil {
		now := time.Now().UTC()
		existing.Deleted = true
		existing.DeletedAt = &now
	}

	return nil
}

func cloneRecordedStatus(status *models.Status) *models.Status {
	if status == nil {
		return nil
	}

	cloned := *status
	cloned.Hashtags = append([]string(nil), status.Hashtags...)
	cloned.Mentions = append([]string(nil), status.Mentions...)
	cloned.URLs = append([]string(nil), status.URLs...)
	cloned.ToRecipients = append([]string(nil), status.ToRecipients...)
	cloned.CcRecipients = append([]string(nil), status.CcRecipients...)
	cloned.BtoRecipients = append([]string(nil), status.BtoRecipients...)
	cloned.BccRecipients = append([]string(nil), status.BccRecipients...)

	if status.Note != nil {
		noteCopy := *status.Note
		noteCopy.To = append([]string(nil), status.Note.To...)
		noteCopy.CC = append([]string(nil), status.Note.CC...)
		noteCopy.BTo = append([]string(nil), status.Note.BTo...)
		noteCopy.BCC = append([]string(nil), status.Note.BCC...)
		noteCopy.Attachment = append([]activitypub.Attachment(nil), status.Note.Attachment...)
		noteCopy.Tag = append([]activitypub.Tag(nil), status.Note.Tag...)
		cloned.Note = &noteCopy
	}

	return &cloned
}

func TestInboxHandler_Round10_ProcessAddRemove_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()

	target := env.local.ID + "/featured"
	objectID := env.cfg.BaseURL() + "/objects/1"

	t.Run("add missing target", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-missing-target"},
			Actor:      env.local.ID,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add missing object", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-missing-object"},
			Actor:      env.local.ID,
			Target:     target,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add object missing id", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-object-no-id"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     map[string]any{"type": activitypub.NoteType},
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add invalid target", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-invalid-target"},
			Actor:      env.local.ID,
			Target:     "bad",
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add unauthorized", func(t *testing.T) {
		err := env.handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-unauthorized"},
			Actor:      env.remoteActorID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("add item persistence fails", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Create").Return(errors.New("boom")).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		err := handler.processAddActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AddType, ID: env.cfg.BaseURL() + "/activities/add-create-fail"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove missing target", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-missing-target"},
			Actor:      env.local.ID,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove missing object", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-missing-object"},
			Actor:      env.local.ID,
			Target:     target,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove object missing id", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-object-no-id"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     map[string]any{"type": activitypub.NoteType},
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove invalid target", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-invalid-target"},
			Actor:      env.local.ID,
			Target:     "bad",
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove unauthorized", func(t *testing.T) {
		err := env.handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-unauthorized"},
			Actor:      env.remoteActorID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})

	t.Run("remove idempotent not found", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Delete").Return(errors.New("not found")).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		err := handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-not-found"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.NoError(t, err)
	})

	t.Run("remove delete failure", func(t *testing.T) {
		innerDB := new(dynamormMocks.MockDB)
		query := new(dynamormMocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Delete").Return(errors.New("boom")).Maybe()

		handler := *env.handler
		handler.objectRepository = repositories.NewObjectRepository(db, env.cfg.DynamoTableName, env.cfg.Domain, zap.NewNop())

		err := handler.processRemoveActivity(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.RemoveType, ID: env.cfg.BaseURL() + "/activities/remove-boom"},
			Actor:      env.local.ID,
			Target:     target,
			Object:     objectID,
		}, env.local)
		require.Error(t, err)
	})
}

func TestInboxHandler_Round10_RemoteCreateUpdateDelete_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	ctx := context.Background()

	t.Run("create invalid object returns success", func(t *testing.T) {
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-invalid-object"},
			Actor:      env.remoteActorID,
			Object:     "not-a-map",
		}
		require.NoError(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
	})

	t.Run("create invalid note returns error", func(t *testing.T) {
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-invalid-note"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"type":    activitypub.NoteType,
				"content": "missing id attributedTo etc",
			},
		}
		require.Error(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
	})

	t.Run("create non-note object returns success", func(t *testing.T) {
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-non-note"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": "Article"},
		}
		require.NoError(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
	})

	t.Run("create remote article is explicit unsupported no-op", func(t *testing.T) {
		objectRepo := inmemory.NewObjectRepository()
		statusRepo := &recordingStatusRepository{}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		articleID := "https://remote.example/articles/protocol-article"
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-remote-article"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           articleID,
				"type":         activitypub.ArticleType,
				"name":         "Remote Article",
				"summary":      "not ingested during M2",
				"content":      "<p>remote long form</p>",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
			},
		}

		require.NoError(t, handler.processRemoteCreateActivity(ctx, create, env.local))
		_, err := objectRepo.GetObject(ctx, articleID)
		require.ErrorIs(t, err, storage.ErrNotFound)
		require.Empty(t, statusRepo.created)
		require.Empty(t, statusRepo.updated)
	})

	t.Run("create note materializes canonical status", func(t *testing.T) {
		statusRepo := &recordingStatusRepository{}
		env.handler.statusRepository = statusRepo

		noteID := "https://remote.example/users/bob/statuses/create-123"
		parentID := "https://another.remote/users/alice/statuses/create-123"
		published := time.Date(2025, 12, 28, 13, 0, 0, 0, time.UTC)

		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: env.cfg.BaseURL() + "/activities/create-status-materialization"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "hello from a federated note",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
				"inReplyTo":    parentID,
				"published":    published.Format(time.RFC3339),
			},
		}

		require.NoError(t, env.handler.processRemoteCreateActivity(ctx, create, env.local))
		require.Len(t, statusRepo.created, 1)

		status := statusRepo.created[0]
		require.NotNil(t, status)
		assert.Equal(t, models.CanonicalStatusID(noteID), status.StatusID)
		assert.Equal(t, env.remoteActorID, status.AuthorID)
		assert.Equal(t, "bob@remote.example", status.AuthorUsername)
		assert.Equal(t, []string{noteID}, status.URLs)
		require.NotNil(t, status.Note)
		assert.Equal(t, noteID, status.Note.ID)
		assert.Equal(t, env.remoteActorID, status.Note.AttributedTo)
		assert.Equal(t, parentID, status.Note.InReplyTo)
		assert.Equal(t, published, status.PublishedAt)

		require.NoError(t, status.BeforeCreate())
		assert.Equal(t, "bob@remote.example", status.AuthorUsername)
		assert.Equal(t, models.CanonicalStatusID(parentID), status.InReplyToID)
		assert.Equal(t, models.VisibilityPublic, status.Visibility)
	})

	t.Run("update invalid object returns success", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-invalid-object"},
			Actor:      env.remoteActorID,
			Object:     "not-a-map",
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update missing id returns success", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-missing-id"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": activitypub.NoteType},
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update unauthorized returns error", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-unauthorized"},
			Actor:      "https://remote.example/users/other",
			Object: map[string]any{
				"id":           env.cfg.BaseURL() + "/objects/1",
				"type":         activitypub.NoteType,
				"attributedTo": env.remoteActorID,
				"content":      "updated content",
			},
		}
		require.Error(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update non-note object returns success", func(t *testing.T) {
		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-non-note"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":           env.cfg.BaseURL() + "/objects/1",
				"type":         "Article",
				"attributedTo": env.remoteActorID,
				"content":      "updated content",
			},
		}
		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
	})

	t.Run("update remote article is explicit unsupported no-op", func(t *testing.T) {
		objectRepo := inmemory.NewObjectRepository()
		statusRepo := &recordingStatusRepository{}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		published := time.Date(2026, time.May, 19, 13, 30, 0, 0, time.UTC)
		articleID := "https://remote.example/articles/update-protocol-article"
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        articleID,
					Type:      activitypub.ArticleType,
					Published: &published,
					To:        []string{activitypub.PublicAddress},
				},
				AttributedTo: env.remoteActorID,
				Content:      "<p>before</p>",
			},
			Name: "Remote Article",
		}
		require.NoError(t, objectRepo.CreateObject(ctx, article))

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-remote-article"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":           articleID,
				"type":         activitypub.ArticleType,
				"name":         "Remote Article Updated",
				"content":      "<p>after</p>",
				"attributedTo": env.remoteActorID,
			},
		}

		require.NoError(t, handler.processRemoteUpdateActivity(ctx, update, env.local))
		got, err := objectRepo.GetObject(ctx, articleID)
		require.NoError(t, err)
		gotArticle, ok := got.(*activitypub.Article)
		require.True(t, ok)
		require.Equal(t, "Remote Article", gotArticle.Name)
		require.Equal(t, "<p>before</p>", gotArticle.Content)
		require.Empty(t, statusRepo.created)
		require.Empty(t, statusRepo.updated)
	})

	t.Run("update note refreshes canonical status", func(t *testing.T) {
		noteID := "https://remote.example/users/bob/statuses/update-123"
		parentID := "https://another.remote/users/alice/statuses/root-1"
		statusRepo := &recordingStatusRepository{
			statuses: map[string]*models.Status{
				models.CanonicalStatusID(noteID): {
					StatusID:       models.CanonicalStatusID(noteID),
					AuthorID:       env.remoteActorID,
					AuthorUsername: "bob@remote.example",
					LikeCount:      7,
					ReplyCount:     3,
					PublishedAt:    time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC),
					CreatedAt:      time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC),
				},
			},
		}
		env.handler.statusRepository = statusRepo

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-status-materialization"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "hello after an edit",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
				"inReplyTo":    parentID,
			},
		}

		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Len(t, statusRepo.updated, 1)
		assert.Empty(t, statusRepo.created)

		status := statusRepo.updated[0]
		require.NotNil(t, status)
		assert.Equal(t, models.CanonicalStatusID(noteID), status.StatusID)
		assert.Equal(t, env.remoteActorID, status.AuthorID)
		assert.Equal(t, "bob@remote.example", status.AuthorUsername)
		assert.Equal(t, "hello after an edit", status.Content)
		assert.Equal(t, models.CanonicalStatusID(parentID), status.InReplyToID)
		assert.Equal(t, []string{noteID}, status.URLs)
		assert.Equal(t, models.VisibilityPublic, status.Visibility)
		assert.Equal(t, 7, status.LikeCount)
		assert.Equal(t, 3, status.ReplyCount)
		require.NotNil(t, status.Note)
		assert.Equal(t, noteID, status.Note.ID)
	})

	t.Run("create then update cannot reattribute remote status to local actor", func(t *testing.T) {
		localHostAndPath := strings.TrimPrefix(strings.TrimPrefix(env.local.ID, "http://"), "https://")
		for _, attributedTo := range []string{
			env.local.ID + "?x=1",
			"HTTP://" + localHostAndPath,
			"HTTPS://" + localHostAndPath,
			"HttPs://" + localHostAndPath,
			"https://[::1]/users/alice",
			"http://:8443/users/alice",
		} {
			t.Run(attributedTo, func(t *testing.T) {
				objectRepo := inmemory.NewObjectRepository()
				statusRepo := &recordingStatusRepository{}
				handler := *env.handler
				handler.objectRepository = objectRepo
				handler.statusRepository = statusRepo

				noteID := "https://remote.example/objects/reattribution-attack"
				create := &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: "https://remote.example/activities/reattribution-create"},
					Actor:      env.remoteActorID,
					Object: map[string]any{
						"@context":     []any{"https://www.w3.org/ns/activitystreams"},
						"id":           noteID,
						"type":         activitypub.NoteType,
						"content":      "honest remote create",
						"attributedTo": env.remoteActorID,
						"to":           []any{activitypub.PublicAddress},
					},
				}
				require.NoError(t, handler.processRemoteCreateActivity(ctx, create, env.local))
				require.Len(t, statusRepo.created, 1)
				require.Equal(t, env.remoteActorID, statusRepo.created[0].AuthorID)

				update := &activitypub.Activity{
					BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: "https://remote.example/activities/reattribution-update"},
					Actor:      env.remoteActorID,
					Object: map[string]any{
						"@context":     []any{"https://www.w3.org/ns/activitystreams"},
						"id":           noteID,
						"type":         activitypub.NoteType,
						"content":      "forged local author",
						"attributedTo": attributedTo,
						"to":           []any{activitypub.PublicAddress},
					},
				}

				require.Error(t, handler.processRemoteUpdateActivity(ctx, update, env.local))
				// A status write is the DynamoDB Streams trigger for public:actor:alice and
				// user:alice. Refusing the write proves neither stream can receive the attack.
				require.Empty(t, statusRepo.updated)
				require.Len(t, statusRepo.created, 1)
				require.Equal(t, env.remoteActorID, statusRepo.created[0].AuthorID)

				stored, err := objectRepo.GetObject(ctx, noteID)
				require.NoError(t, err)
				storedNote, ok := stored.(*activitypub.Note)
				require.True(t, ok)
				require.Equal(t, env.remoteActorID, storedNote.AttributedTo)
			})
		}
	})

	t.Run("create then honest update preserves remote attribution", func(t *testing.T) {
		objectRepo := inmemory.NewObjectRepository()
		statusRepo := &recordingStatusRepository{}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		noteID := "https://remote.example/objects/honest-update"
		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.CreateType, ID: "https://remote.example/activities/honest-create"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "before honest update",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
			},
		}
		require.NoError(t, handler.processRemoteCreateActivity(ctx, create, env.local))
		require.Len(t, statusRepo.created, 1)

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: "https://remote.example/activities/honest-update"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "after honest update",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
			},
		}

		require.NoError(t, handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Len(t, statusRepo.updated, 1)
		require.Equal(t, env.remoteActorID, statusRepo.updated[0].AuthorID)
		require.Equal(t, "bob@remote.example", statusRepo.updated[0].AuthorUsername)
		require.Equal(t, "after honest update", statusRepo.updated[0].Content)
	})

	t.Run("update with global IPv6 attribution preserves the complete remote acct", func(t *testing.T) {
		noteID := "https://remote.example/objects/ipv6-update"
		objectRepo := inmemory.NewObjectRepository()
		require.NoError(t, objectRepo.CreateObject(ctx, &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: noteID, Type: activitypub.NoteType},
			AttributedTo: env.remoteActorID,
			Content:      "before",
		}))
		statusRepo := &recordingStatusRepository{statuses: map[string]*models.Status{
			models.CanonicalStatusID(noteID): {
				StatusID:       models.CanonicalStatusID(noteID),
				AuthorID:       env.remoteActorID,
				AuthorUsername: "bob@remote.example",
			},
		}}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: "https://remote.example/activities/ipv6-update"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "after ipv6 update",
				"attributedTo": "https://[2001:db8::1]/users/alice",
				"to":           []any{activitypub.PublicAddress},
			},
		}

		require.NoError(t, handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Len(t, statusRepo.updated, 1)
		require.Equal(t, "alice@2001:db8::1", statusRepo.updated[0].AuthorUsername)
		require.NotEqual(t, "alice@2001", statusRepo.updated[0].AuthorUsername)
	})

	t.Run("update note requires complete ActivityPub attribution", func(t *testing.T) {
		noteID := "https://remote.example/users/bob/statuses/update-missing-attribution"
		objectRepo := inmemory.NewObjectRepository()
		require.NoError(t, objectRepo.CreateObject(ctx, &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: noteID, Type: activitypub.NoteType},
			AttributedTo: env.remoteActorID,
			Content:      "before",
		}))
		statusRepo := &recordingStatusRepository{}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: "https://remote.example/activities/update-missing-attribution"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context": []any{"https://www.w3.org/ns/activitystreams"},
				"id":       noteID,
				"type":     activitypub.NoteType,
				"content":  "after",
				"to":       []any{activitypub.PublicAddress},
			},
		}

		require.Error(t, handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Empty(t, statusRepo.updated)
		stored, err := objectRepo.GetObject(ctx, noteID)
		require.NoError(t, err)
		storedNote, ok := stored.(*activitypub.Note)
		require.True(t, ok)
		require.Equal(t, env.remoteActorID, storedNote.AttributedTo)
	})

	t.Run("update note validation rejects malformed id independently of attribution guard", func(t *testing.T) {
		noteID := "not-an-activitypub-url"
		objectRepo := inmemory.NewObjectRepository()
		require.NoError(t, objectRepo.CreateObject(ctx, &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: noteID, Type: activitypub.NoteType},
			AttributedTo: env.remoteActorID,
			Content:      "before",
		}))
		statusRepo := &recordingStatusRepository{}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: "https://remote.example/activities/update-malformed-note-id"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "after",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
			},
		}

		require.Error(t, handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Empty(t, statusRepo.updated)
		require.Empty(t, statusRepo.created)
		stored, err := objectRepo.GetObject(ctx, noteID)
		require.NoError(t, err)
		storedNote, ok := stored.(*activitypub.Note)
		require.True(t, ok)
		require.Equal(t, "before", storedNote.Content)
		require.Equal(t, env.remoteActorID, storedNote.AttributedTo)
	})

	t.Run("update note preserves tombstone metadata", func(t *testing.T) {
		noteID := "https://remote.example/users/bob/statuses/update-deleted-123"
		deletedAt := time.Date(2025, 12, 28, 11, 0, 0, 0, time.UTC)
		statusRepo := &recordingStatusRepository{
			statuses: map[string]*models.Status{
				models.CanonicalStatusID(noteID): {
					StatusID:    models.CanonicalStatusID(noteID),
					AuthorID:    env.remoteActorID,
					Deleted:     true,
					DeletedAt:   &deletedAt,
					PublishedAt: time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC),
					CreatedAt:   time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC),
				},
			},
		}
		env.handler.statusRepository = statusRepo

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-preserves-tombstone"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "hello after delete",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
			},
		}

		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Len(t, statusRepo.updated, 1)
		require.True(t, statusRepo.updated[0].Deleted)
		require.NotNil(t, statusRepo.updated[0].DeletedAt)
		assert.Equal(t, deletedAt, *statusRepo.updated[0].DeletedAt)
	})

	t.Run("update note materializes missing canonical status", func(t *testing.T) {
		noteID := "https://remote.example/users/bob/statuses/update-create-123"
		statusRepo := &recordingStatusRepository{}
		env.handler.statusRepository = statusRepo

		update := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.UpdateType, ID: env.cfg.BaseURL() + "/activities/update-materializes-missing-status"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"@context":     []any{"https://www.w3.org/ns/activitystreams"},
				"id":           noteID,
				"type":         activitypub.NoteType,
				"content":      "late materialized edit",
				"attributedTo": env.remoteActorID,
				"to":           []any{activitypub.PublicAddress},
			},
		}

		require.NoError(t, env.handler.processRemoteUpdateActivity(ctx, update, env.local))
		require.Len(t, statusRepo.created, 1)
		assert.Empty(t, statusRepo.updated)
		assert.Equal(t, models.CanonicalStatusID(noteID), statusRepo.created[0].StatusID)
	})

	t.Run("delete unsupported object returns success", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-unsupported"},
			Actor:      env.remoteActorID,
			Object:     123,
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete missing object id returns success", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-missing-id"},
			Actor:      env.remoteActorID,
			Object:     map[string]any{"type": "Tombstone"},
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete unauthorized returns error", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-unauthorized"},
			Actor:      "https://remote.example/users/other",
			Object:     env.cfg.BaseURL() + "/objects/1",
		}
		require.Error(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete embedded object uses formerType", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-embedded"},
			Actor:      env.remoteActorID,
			Object: map[string]any{
				"id":   env.cfg.BaseURL() + "/objects/1",
				"type": "Article",
			},
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete typed object branch", func(t *testing.T) {
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-typed"},
			Actor:      env.remoteActorID,
			Object:     &activitypub.BaseObject{ID: env.cfg.BaseURL() + "/objects/1"},
		}
		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
	})

	t.Run("delete stored article creates article tombstone", func(t *testing.T) {
		objectRepo := inmemory.NewObjectRepository()
		statusRepo := &recordingStatusRepository{}
		handler := *env.handler
		handler.objectRepository = objectRepo
		handler.statusRepository = statusRepo

		published := time.Date(2026, time.May, 19, 14, 0, 0, 0, time.UTC)
		articleID := "https://remote.example/articles/delete-protocol-article"
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        articleID,
					Type:      activitypub.ArticleType,
					Published: &published,
					To:        []string{activitypub.PublicAddress},
				},
				AttributedTo: env.remoteActorID,
				Content:      "<p>before delete</p>",
			},
			Name: "Remote Article",
		}
		require.NoError(t, objectRepo.CreateObject(ctx, article))

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-remote-article"},
			Actor:      env.remoteActorID,
			Object:     articleID,
		}

		require.NoError(t, handler.processRemoteDeleteActivity(ctx, del, env.local))
		tombstoned, err := objectRepo.IsTombstoned(ctx, articleID)
		require.NoError(t, err)
		require.True(t, tombstoned)
		tombstone, err := objectRepo.GetTombstone(ctx, articleID)
		require.NoError(t, err)
		require.Equal(t, activitypub.ArticleType, tombstone.FormerType)
		require.Equal(t, env.remoteActorID, tombstone.DeletedBy)
	})

	t.Run("delete note tombstones canonical status", func(t *testing.T) {
		noteID := "https://remote.example/users/bob/statuses/delete-123"
		statusRepo := &recordingStatusRepository{}
		env.handler.statusRepository = statusRepo

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-status-materialization"},
			Actor:      env.remoteActorID,
			Object:     noteID,
		}

		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
		assert.Equal(t, []string{models.CanonicalStatusID(noteID)}, statusRepo.deletedIDs)
	})

	t.Run("delete missing canonical status stays idempotent", func(t *testing.T) {
		noteID := "https://remote.example/users/bob/statuses/delete-missing-123"
		statusRepo := &recordingStatusRepository{deleteErr: storage.ErrNotFound}
		env.handler.statusRepository = statusRepo

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, ID: env.cfg.BaseURL() + "/activities/delete-status-missing"},
			Actor:      env.remoteActorID,
			Object:     noteID,
		}

		require.NoError(t, env.handler.processRemoteDeleteActivity(ctx, del, env.local))
		assert.Equal(t, []string{models.CanonicalStatusID(noteID)}, statusRepo.deletedIDs)
	})
}

func TestInboxHandler_Round10_AuthenticateInboxRequest_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	makeToken := func(username string, issuedAt time.Time) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"username": username,
			"scopes":   []string{"read"},
			"iat":      issuedAt.Unix(),
			"exp":      time.Now().Add(time.Hour).Unix(),
		})
		signed, err := token.SignedString([]byte(env.cfg.JWTSecret))
		require.NoError(t, err)
		return signed
	}

	t.Run("invalid header prefix", func(t *testing.T) {
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Basic abc",
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})

	t.Run("invalid token", func(t *testing.T) {
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer invalid",
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})

	t.Run("long-lived unexpired token older than 24h still authenticates", func(t *testing.T) {
		oldToken := makeToken("alice", time.Now().Add(-48*time.Hour))
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer " + oldToken,
		}, nil, nil)

		claims, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.NoError(t, err)
		require.NotNil(t, claims)
		require.Equal(t, "alice", claims.PreferredUsername)
	})

	t.Run("username mismatch is forbidden", func(t *testing.T) {
		bobToken := makeToken("bob", time.Now())
		liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer " + bobToken,
		}, nil, nil)

		_, err := env.handler.authenticateInboxRequest(liftCtx, "alice")
		require.Error(t, err)
	})
}

func TestInboxHandler_Round10_InboxPagination_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)

	headers := map[string]string{
		"Host": "localhost",
	}
	liftCtx := newAppTheoryContext("GET", "/users/alice/inbox", headers, nil, nil)

	innerDB := new(dynamormMocks.MockDB)
	query := new(dynamormMocks.MockQuery)
	db := &extendedMockDB{inner: innerDB}

	innerDB.On("Model", mock.Anything).Return(query).Maybe()
	query.On("WithContext", mock.Anything).Return(query).Maybe()
	query.On("Index", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
	query.On("All", mock.Anything).Return(errors.New("boom")).Maybe()

	badHandler := *env.handler
	badHandler.activityRepository = repositories.NewActivityRepository(db, env.cfg.DynamoTableName, zap.NewNop(), nil)

	_, err := badHandler.returnInboxCollection(liftCtx, env.local, "alice")
	require.Error(t, err)
	_, err = badHandler.returnInboxPage(liftCtx, env.local, "alice", 20, "")
	require.Error(t, err)

	page := env.handler.buildCollectionPage(env.local, []*activitypub.Activity{}, "cursor", "next", 20)
	require.NotEmpty(t, page.Next)
	require.NotEmpty(t, page.Prev)
}

func TestInboxHandler_Round10_HandlePostInbox_EarlyFailures(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("missing username param", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, []byte(`{}`))
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("invalid content type", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "text/plain",
		}, nil, []byte(`{}`))
		ctx.Params["username"] = "alice"
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("missing body", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, nil)
		ctx.Params["username"] = "alice"
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("invalid json body", func(t *testing.T) {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, []byte("{"))
		ctx.Params["username"] = "alice"
		_, err := env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("activity not addressed to actor fails validation", func(t *testing.T) {
		raw := map[string]any{
			"@context": activitypub.Context,
			"type":     activitypub.CreateType,
			"id":       env.cfg.BaseURL() + "/activities/not-addressed",
			"actor":    env.remoteActorID,
			"to":       []string{"https://remote.example/users/other"},
			"object":   env.cfg.BaseURL() + "/objects/1",
		}
		body, err := json.Marshal(raw)
		require.NoError(t, err)

		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, body)
		ctx.Params["username"] = "alice"
		_, err = env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})

	t.Run("invalid attachments fail validation", func(t *testing.T) {
		raw := map[string]any{
			"@context": activitypub.Context,
			"type":     activitypub.CreateType,
			"id":       env.cfg.BaseURL() + "/activities/bad-attachments",
			"actor":    env.remoteActorID,
			"to":       []string{env.local.ID},
			"object": map[string]any{
				"type":       activitypub.NoteType,
				"id":         env.cfg.BaseURL() + "/objects/bad-attachments",
				"content":    "hi",
				"attachment": "not-an-array",
			},
		}
		body, err := json.Marshal(raw)
		require.NoError(t, err)

		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{
			"Host":         "localhost",
			"Content-Type": "application/activity+json",
		}, nil, body)
		ctx.Params["username"] = "alice"
		_, err = env.handler.handlePostInbox(ctx)
		require.Error(t, err)
	})
}
