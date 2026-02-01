package importexport

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeExportRepo struct {
	createErr error
	exports   map[string]*models.Export

	listExports    []*models.Export
	listNextCursor string
	listErr        error

	getErr error

	updateStatusCalls []fakeUpdateExportStatusCall
	updateStatusErr   error
}

type fakeUpdateExportStatusCall struct {
	exportID       string
	status         string
	completionData map[string]any
	errorMsg       string
}

func (r *fakeExportRepo) CreateExport(_ context.Context, export *models.Export) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.exports == nil {
		r.exports = map[string]*models.Export{}
	}
	r.exports[export.ID] = export
	return nil
}

func (r *fakeExportRepo) GetExport(_ context.Context, exportID string) (*models.Export, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.exports == nil {
		return nil, stderrors.New("not found")
	}
	export, ok := r.exports[exportID]
	if !ok {
		return nil, stderrors.New("not found")
	}
	return export, nil
}

func (r *fakeExportRepo) GetExportsForUser(_ context.Context, _ string, _ int, _ string) ([]*models.Export, string, error) {
	if r.listErr != nil {
		return nil, "", r.listErr
	}
	return r.listExports, r.listNextCursor, nil
}

func (r *fakeExportRepo) UpdateExportStatus(_ context.Context, exportID, status string, completionData map[string]any, errorMsg string) error {
	r.updateStatusCalls = append(r.updateStatusCalls, fakeUpdateExportStatusCall{
		exportID:       exportID,
		status:         status,
		completionData: completionData,
		errorMsg:       errorMsg,
	})
	if r.updateStatusErr != nil {
		return r.updateStatusErr
	}
	if r.exports != nil {
		if export, ok := r.exports[exportID]; ok {
			export.Status = status
		}
	}
	return nil
}

type fakeImportRepo struct {
	createErr error
	imports   map[string]*models.Import

	getErr  error
	listErr error

	listImports    []*models.Import
	listNextCursor string
}

func (r *fakeImportRepo) CreateImport(_ context.Context, importRecord *models.Import) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.imports == nil {
		r.imports = map[string]*models.Import{}
	}
	r.imports[importRecord.ID] = importRecord
	return nil
}

func (r *fakeImportRepo) GetImport(_ context.Context, importID string) (*models.Import, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.imports == nil {
		return nil, stderrors.New("not found")
	}
	imp, ok := r.imports[importID]
	if !ok {
		return nil, stderrors.New("not found")
	}
	return imp, nil
}

func (r *fakeImportRepo) GetImportsForUser(_ context.Context, _ string, _ int, _ string) ([]*models.Import, string, error) {
	if r.listErr != nil {
		return nil, "", r.listErr
	}
	return r.listImports, r.listNextCursor, nil
}

type fakeAccountRepo struct {
	accountByUsername map[string]*storage.Account
	errByUsername     map[string]error
}

func (r *fakeAccountRepo) GetAccount(_ context.Context, username string) (*storage.Account, error) {
	if err := r.errByUsername[username]; err != nil {
		return nil, err
	}
	return r.accountByUsername[username], nil
}

type fakeQueueService struct {
	queueExportErr error
	queueImportErr error
	queuedExports  []string
	queuedImports  []string
}

func (q *fakeQueueService) QueueExportJob(_ context.Context, exportID string) error {
	q.queuedExports = append(q.queuedExports, exportID)
	return q.queueExportErr
}

func (q *fakeQueueService) QueueImportJob(_ context.Context, importID string) error {
	q.queuedImports = append(q.queuedImports, importID)
	return q.queueImportErr
}

type fakeStorageClient struct {
	presignedURL string
	presignErr   error
	presignCalls []string
}

func (c *fakeStorageClient) GeneratePresignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	c.presignCalls = append(c.presignCalls, key)
	if c.presignErr != nil {
		return "", c.presignErr
	}
	return c.presignedURL, nil
}

func (c *fakeStorageClient) UploadFile(context.Context, string, []byte) error { return nil }

func (c *fakeStorageClient) GetFile(context.Context, string) ([]byte, error) { return nil, nil }

type fakePublisher struct {
	publishUserCalls []fakePublisherCall
	err              error
}

type fakePublisherCall struct {
	user  string
	event *streaming.Event
}

func (p *fakePublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	p.publishUserCalls = append(p.publishUserCalls, fakePublisherCall{user: userID, event: event})
	return p.err
}

func (p *fakePublisher) PublishToStream(context.Context, string, *streaming.Event) error {
	return p.err
}

func (p *fakePublisher) PublishToConversation(context.Context, string, *streaming.Event) error {
	return p.err
}

func (p *fakePublisher) Close() error { return nil }

func TestService_CreateExport_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("validation_error_is_wrapped", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{},
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{errByUsername: map[string]error{"alice": stderrors.New("missing")}},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		_, err := svc.CreateExport(ctx, &CreateExportCommand{
			Username:    "alice",
			Type:        "archive",
			Format:      "activitypub",
			RequestedBy: "alice",
		})
		require.ErrorIs(t, err, serviceerrors.ErrExportValidationFailed)
		require.ErrorIs(t, err, serviceerrors.ErrUserNotFound)
	})

	t.Run("create_export_repo_error", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{createErr: stderrors.New("boom")},
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{accountByUsername: map[string]*storage.Account{"alice": {}}},
			nil,
			nil,
			nil,
			&fakeQueueService{},
			nil,
			zap.NewNop(),
			"example.com",
		)

		_, err := svc.CreateExport(ctx, &CreateExportCommand{
			Username:    "alice",
			Type:        "archive",
			Format:      "activitypub",
			RequestedBy: "alice",
		})
		require.ErrorIs(t, err, serviceerrors.ErrCreateExport)
	})

	t.Run("queue_service_missing_marks_failed", func(t *testing.T) {
		repo := &fakeExportRepo{}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{accountByUsername: map[string]*storage.Account{"alice": {}}},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		_, err := svc.CreateExport(ctx, &CreateExportCommand{
			Username:    "alice",
			Type:        "archive",
			Format:      "activitypub",
			RequestedBy: "alice",
		})
		require.ErrorIs(t, err, serviceerrors.ErrQueueExport)
		require.Len(t, repo.updateStatusCalls, 1)
		assert.Equal(t, models.StatusFailed, repo.updateStatusCalls[0].status)
	})

	t.Run("queue_export_error_marks_failed", func(t *testing.T) {
		repo := &fakeExportRepo{}
		queue := &fakeQueueService{queueExportErr: stderrors.New("nope")}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{accountByUsername: map[string]*storage.Account{"alice": {}}},
			nil,
			nil,
			nil,
			queue,
			nil,
			zap.NewNop(),
			"example.com",
		)

		_, err := svc.CreateExport(ctx, &CreateExportCommand{
			Username:    "alice",
			Type:        "archive",
			Format:      "activitypub",
			RequestedBy: "alice",
		})
		require.ErrorIs(t, err, serviceerrors.ErrQueueExport)
		require.Len(t, repo.updateStatusCalls, 1)
		assert.Equal(t, models.StatusFailed, repo.updateStatusCalls[0].status)
		require.Len(t, queue.queuedExports, 1)
	})

	t.Run("success_emits_event_and_publishes", func(t *testing.T) {
		repo := &fakeExportRepo{}
		queue := &fakeQueueService{}
		pub := &fakePublisher{err: stderrors.New("ignored")}

		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{accountByUsername: map[string]*storage.Account{"alice": {}}},
			nil,
			nil,
			pub,
			queue,
			nil,
			zap.NewNop(),
			"example.com",
		)

		result, err := svc.CreateExport(ctx, &CreateExportCommand{
			Username:     "alice",
			Type:         "archive",
			Format:       "activitypub",
			IncludeMedia: true,
			Options:      map[string]string{"compress": "true"},
			RequestedBy:  "alice",
			DateRange: &DateRange{
				Start: time.Now().Add(-24 * time.Hour),
				End:   time.Now().Add(-23 * time.Hour),
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Export)
		assert.Equal(t, "alice", result.Export.Username)
		assert.Equal(t, models.StatusPending, result.Export.Status)
		assert.True(t, result.Export.IncludeMedia)
		assert.Equal(t, "true", result.Export.Options["compress"])
		require.Len(t, result.Events, 1)
		require.Len(t, queue.queuedExports, 1)
		require.Len(t, pub.publishUserCalls, 1)
	})
}

func TestService_GetExport_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("repository_error", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{getErr: stderrors.New("boom")},
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		_, err := svc.GetExport(ctx, &GetExportQuery{ExportID: "exp1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrGetExport)
	})

	t.Run("ownership_mismatch_returns_forbidden_app_error", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{"exp1": {ID: "exp1", Username: "bob"}}}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		_, err := svc.GetExport(ctx, &GetExportQuery{ExportID: "exp1", Username: "alice"})
		var appErr common.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, 403, appErr.StatusCode)
		assert.Equal(t, serviceerrors.ErrExportAccessForbidden, appErr.InternalError)
	})

	t.Run("completed_with_storage_client_presigns_url", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{
			"exp1": {ID: "exp1", Username: "alice", Status: "completed", DownloadURL: "stored", S3Key: "key1"},
		}}
		client := &fakeStorageClient{presignedURL: "https://signed.example/file"}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			client,
			zap.NewNop(),
			"example.com",
		)

		result, err := svc.GetExport(ctx, &GetExportQuery{ExportID: "exp1", Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "https://signed.example/file", result.DownloadURL)
		require.NotNil(t, result.ExpiresAt)
		assert.Equal(t, []string{"key1"}, client.presignCalls)
	})

	t.Run("completed_without_storage_client_uses_stored_url", func(t *testing.T) {
		expiresAt := time.Now().Add(2 * time.Hour)
		repo := &fakeExportRepo{exports: map[string]*models.Export{
			"exp1": {ID: "exp1", Username: "alice", Status: "completed", DownloadURL: "https://stored", ExpiresAt: &expiresAt},
		}}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		result, err := svc.GetExport(ctx, &GetExportQuery{ExportID: "exp1", Username: "alice"})
		require.NoError(t, err)
		assert.Equal(t, "https://stored", result.DownloadURL)
		require.NotNil(t, result.ExpiresAt)
	})

	t.Run("storage_client_presign_error_is_non_fatal", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{
			"exp1": {ID: "exp1", Username: "alice", Status: "completed", DownloadURL: "stored", S3Key: "key1"},
		}}
		client := &fakeStorageClient{presignErr: stderrors.New("boom")}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			client,
			zap.NewNop(),
			"example.com",
		)

		result, err := svc.GetExport(ctx, &GetExportQuery{ExportID: "exp1", Username: "alice"})
		require.NoError(t, err)
		assert.Empty(t, result.DownloadURL)
		assert.Nil(t, result.ExpiresAt)
	})
}

func TestService_ListAndUpdateExport_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("ListExports_error", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{listErr: stderrors.New("boom")},
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)
		_, err := svc.ListExports(ctx, &ListExportsQuery{Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrListExports)
	})

	t.Run("ListExports_success", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{listExports: []*models.Export{{ID: "e1"}}, listNextCursor: "next"},
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)
		result, err := svc.ListExports(ctx, &ListExportsQuery{Username: "alice"})
		require.NoError(t, err)
		assert.True(t, result.HasMore)
		assert.Equal(t, "next", result.NextCursor)
	})

	t.Run("UpdateExportProgress_updates_status_and_publishes", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{"e1": {ID: "e1", Username: "alice"}}}
		pub := &fakePublisher{err: stderrors.New("ignored")}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			pub,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		err := svc.UpdateExportProgress(ctx, "e1", 5, 10)
		require.NoError(t, err)
		require.Len(t, repo.updateStatusCalls, 1)
		assert.Equal(t, "processing", repo.updateStatusCalls[0].status)
		require.Len(t, pub.publishUserCalls, 1)
		assert.Equal(t, "alice", pub.publishUserCalls[0].user)
	})

	t.Run("CompleteExport_updates_status_and_publishes", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{"e1": {ID: "e1", Username: "alice"}}}
		pub := &fakePublisher{}
		svc := NewService(
			repo,
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			pub,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)

		err := svc.CompleteExport(ctx, "e1", "s3://bucket/key", 123)
		require.NoError(t, err)
		require.Len(t, repo.updateStatusCalls, 1)
		assert.Equal(t, "completed", repo.updateStatusCalls[0].status)
		require.Len(t, pub.publishUserCalls, 1)
	})
}

func TestService_CancelExport_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("not_found", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{getErr: stderrors.New("boom")},
			&fakeImportRepo{},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)
		_, err := svc.CancelExport(ctx, &CancelExportCommand{ExportID: "e1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrExportNotFound)
	})

	t.Run("not_authorized", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{"e1": {ID: "e1", Username: "bob", Status: models.StatusPending}}}
		svc := NewService(repo, &fakeImportRepo{}, nil, &fakeAccountRepo{}, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.CancelExport(ctx, &CancelExportCommand{ExportID: "e1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrNotAuthorizedCancelExport)
	})

	t.Run("cannot_cancel_completed", func(t *testing.T) {
		repo := &fakeExportRepo{exports: map[string]*models.Export{"e1": {ID: "e1", Username: "alice", Status: models.StatusCompleted}}}
		svc := NewService(repo, &fakeImportRepo{}, nil, &fakeAccountRepo{}, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.CancelExport(ctx, &CancelExportCommand{ExportID: "e1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrCannotCancelCompletedExport)
	})

	t.Run("update_status_error", func(t *testing.T) {
		repo := &fakeExportRepo{
			exports:         map[string]*models.Export{"e1": {ID: "e1", Username: "alice", Status: models.StatusPending}},
			updateStatusErr: stderrors.New("boom"),
		}
		svc := NewService(repo, &fakeImportRepo{}, nil, &fakeAccountRepo{}, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.CancelExport(ctx, &CancelExportCommand{ExportID: "e1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrCancelExport)
	})

	t.Run("success_publishes_event", func(t *testing.T) {
		repo := &fakeExportRepo{
			exports: map[string]*models.Export{"e1": {ID: "e1", Username: "alice", Status: models.StatusPending, Type: "archive", Format: "activitypub"}},
		}
		pub := &fakePublisher{err: stderrors.New("ignored")}
		svc := NewService(repo, &fakeImportRepo{}, nil, &fakeAccountRepo{}, nil, nil, pub, nil, nil, zap.NewNop(), "example.com")

		result, err := svc.CancelExport(ctx, &CancelExportCommand{ExportID: "e1", Username: "alice"})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Export)
		assert.Equal(t, models.StatusCancelled, result.Export.Status)
		require.Len(t, pub.publishUserCalls, 1)
	})
}

func TestService_ImportPaths_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("CreateImport_error", func(t *testing.T) {
		svc := NewService(
			&fakeExportRepo{},
			&fakeImportRepo{createErr: stderrors.New("boom")},
			nil,
			&fakeAccountRepo{},
			nil,
			nil,
			nil,
			nil,
			nil,
			zap.NewNop(),
			"example.com",
		)
		_, err := svc.CreateImport(ctx, &CreateImportCommand{
			Username:      "alice",
			Type:          "archive",
			Format:        "activitypub",
			FileURL:       "s3://bucket/key",
			MergeStrategy: "merge",
			RequestedBy:   "alice",
		})
		require.ErrorIs(t, err, serviceerrors.ErrCreateImport)
	})

	t.Run("CreateImport_queue_error_is_non_fatal", func(t *testing.T) {
		repo := &fakeImportRepo{}
		queue := &fakeQueueService{queueImportErr: stderrors.New("boom")}
		pub := &fakePublisher{err: stderrors.New("ignored")}
		svc := NewService(&fakeExportRepo{}, repo, nil, &fakeAccountRepo{}, nil, nil, pub, queue, nil, zap.NewNop(), "example.com")

		result, err := svc.CreateImport(ctx, &CreateImportCommand{
			Username:      "alice",
			Type:          "archive",
			Format:        "activitypub",
			FileURL:       "s3://bucket/key",
			MergeStrategy: "merge",
			RequestedBy:   "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, result.Import)
		require.Len(t, queue.queuedImports, 1)
		require.Len(t, pub.publishUserCalls, 1)
	})

	t.Run("GetImport_not_found", func(t *testing.T) {
		svc := NewService(&fakeExportRepo{}, &fakeImportRepo{getErr: stderrors.New("boom")}, nil, &fakeAccountRepo{}, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.GetImport(ctx, &GetImportQuery{ImportID: "i1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrImportNotFound)
	})

	t.Run("GetImport_not_authorized", func(t *testing.T) {
		repo := &fakeImportRepo{imports: map[string]*models.Import{"i1": {ID: "i1", Username: "bob"}}}
		svc := NewService(&fakeExportRepo{}, repo, nil, &fakeAccountRepo{}, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
		_, err := svc.GetImport(ctx, &GetImportQuery{ImportID: "i1", Username: "alice"})
		require.ErrorIs(t, err, serviceerrors.ErrNotAuthorizedAccessImport)
	})

	t.Run("ListImports_filters_by_status", func(t *testing.T) {
		repo := &fakeImportRepo{
			listImports: []*models.Import{
				{ID: "i1", Username: "alice", Status: "pending"},
				{ID: "i2", Username: "alice", Status: "completed"},
			},
			listNextCursor: "next",
		}
		svc := NewService(&fakeExportRepo{}, repo, nil, &fakeAccountRepo{}, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")

		result, err := svc.ListImports(ctx, &ListImportsQuery{Username: "alice", Status: "completed"})
		require.NoError(t, err)
		require.Len(t, result.Imports, 1)
		assert.Equal(t, "i2", result.Imports[0].ID)
		assert.True(t, result.HasMore)
	})
}
