package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeExportRepo struct {
	exportByID map[string]*models.Export
	errByID    map[string]error
	calls      []string
}

func (r *fakeExportRepo) GetExport(_ context.Context, exportID string) (*models.Export, error) {
	r.calls = append(r.calls, exportID)
	if err := r.errByID[exportID]; err != nil {
		return nil, err
	}
	export, ok := r.exportByID[exportID]
	if !ok {
		return nil, stderrors.New("export not found")
	}
	return export, nil
}

type fakeImportRepo struct {
	importByID map[string]*models.Import
	errByID    map[string]error
	calls      []string
}

func (r *fakeImportRepo) GetImport(_ context.Context, importID string) (*models.Import, error) {
	r.calls = append(r.calls, importID)
	if err := r.errByID[importID]; err != nil {
		return nil, err
	}
	importRecord, ok := r.importByID[importID]
	if !ok {
		return nil, stderrors.New("import not found")
	}
	return importRecord, nil
}

func TestImportExportQueueService_NewImportExportQueueService_round27_validation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("nil_config", func(t *testing.T) {
		_, err := NewImportExportQueueService(ctx, nil, &fakeExportRepo{}, &fakeImportRepo{}, zap.NewNop())
		require.Error(t, err)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("nil_export_repo", func(t *testing.T) {
		_, err := NewImportExportQueueService(ctx, &config.Config{}, nil, &fakeImportRepo{}, zap.NewNop())
		require.Error(t, err)
		assert.Equal(t, "export repository is required", err.Error())
	})

	t.Run("nil_import_repo", func(t *testing.T) {
		_, err := NewImportExportQueueService(ctx, &config.Config{}, &fakeExportRepo{}, nil, zap.NewNop())
		require.Error(t, err)
		assert.Equal(t, "import repository is required", err.Error())
	})
}

func TestImportExportQueueService_QueueExportJob_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	rangeStart := now.Add(-24 * time.Hour)
	rangeEnd := now

	exportRepo := &fakeExportRepo{
		exportByID: map[string]*models.Export{
			"e1": &models.Export{
				ID:           "e1",
				Username:     "alice",
				Type:         "archive",
				Format:       "mastodon",
				IncludeMedia: true,
				DateRange:    &models.ExportDateRange{Start: rangeStart, End: rangeEnd},
				Options:      map[string]any{"include_bookmarks": true},
			},
		},
	}
	importRepo := &fakeImportRepo{}

	t.Run("missing_export_id", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient:  &mockSQSClient{},
			cfg:        &config.Config{Region: "us-east-1", ExportQueueURL: "https://sqs.example.com/export"},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueExportJob(ctx, "")
		require.Error(t, err)
	})

	t.Run("queue_url_not_configured", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient:  &mockSQSClient{},
			cfg:        &config.Config{Region: "us-east-1", ExportQueueURL: ""},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueExportJob(ctx, "e1")
		appErr := new(apperrors.AppError)
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, apperrors.CodeInternal, appErr.Code)
		assert.Equal(t, "Queue URL not configured", appErr.Message)
		assert.Equal(t, "EXPORT_QUEUE_URL", appErr.Metadata["queue_name"])
	})

	t.Run("export_repo_error_propagates", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient: &mockSQSClient{},
			cfg:       &config.Config{Region: "us-east-1", ExportQueueURL: "https://sqs.example.com/export"},
			exportRepo: &fakeExportRepo{
				errByID: map[string]error{"e2": stderrors.New("boom")},
			},
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueExportJob(ctx, "e2")
		require.Error(t, err)
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("serialization_failure", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient: &mockSQSClient{},
			cfg:       &config.Config{Region: "us-east-1", ExportQueueURL: "https://sqs.example.com/export"},
			exportRepo: &fakeExportRepo{
				exportByID: map[string]*models.Export{
					"e3": &models.Export{
						ID:       "e3",
						Username: "alice",
						Type:     "archive",
						Format:   "mastodon",
						Options:  map[string]any{"bad": func() {}},
					},
				},
			},
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueExportJob(ctx, "e3")
		assert.ErrorIs(t, err, ErrExportJobSerialization)
	})

	t.Run("send_failure", func(t *testing.T) {
		mockClient := &mockSQSClient{sendMessageErr: stderrors.New("send failed")}
		svc := &ImportExportQueueService{
			sqsClient:  mockClient,
			cfg:        &config.Config{Region: "us-east-1", ExportQueueURL: "https://sqs.example.com/export"},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueExportJob(ctx, "e1")
		assert.ErrorIs(t, err, ErrExportJobQueue)
	})

	t.Run("success_sends_message_with_expected_shape", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		svc := &ImportExportQueueService{
			sqsClient:  mockClient,
			cfg:        &config.Config{Region: "us-east-1", ExportQueueURL: "https://sqs.example.com/export"},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueExportJob(ctx, "e1")
		require.NoError(t, err)
		require.Len(t, mockClient.sendMessageInputs, 1)

		input := mockClient.sendMessageInputs[0]
		assert.Equal(t, "https://sqs.example.com/export", aws.ToString(input.QueueUrl))

		var decoded ExportJobMessage
		require.NoError(t, json.Unmarshal([]byte(aws.ToString(input.MessageBody)), &decoded))
		assert.Equal(t, "e1", decoded.ExportID)
		assert.Equal(t, "alice", decoded.Username)
		assert.Equal(t, "archive", decoded.Type)
		assert.Equal(t, "mastodon", decoded.Format)
		assert.True(t, decoded.IncludeMedia)
		require.NotNil(t, decoded.DateRange)
		assert.Equal(t, rangeStart, decoded.DateRange.Start)
		assert.Equal(t, rangeEnd, decoded.DateRange.End)
		assert.Equal(t, map[string]any{"include_bookmarks": true}, decoded.Options)
		assert.NotZero(t, decoded.Timestamp)
	})
}

func TestImportExportQueueService_QueueImportJob_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	importRepo := &fakeImportRepo{
		importByID: map[string]*models.Import{
			"i1": &models.Import{ID: "i1", Username: "alice", Type: "followers", Mode: "merge", S3Key: "imports/i1.zip"},
		},
	}
	exportRepo := &fakeExportRepo{}

	t.Run("missing_import_id", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient:  &mockSQSClient{},
			cfg:        &config.Config{Region: "us-east-1", ImportQueueURL: "https://sqs.example.com/import"},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueImportJob(ctx, "")
		require.Error(t, err)
	})

	t.Run("queue_url_not_configured", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient:  &mockSQSClient{},
			cfg:        &config.Config{Region: "us-east-1", ImportQueueURL: "  "},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueImportJob(ctx, "i1")
		appErr := new(apperrors.AppError)
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, apperrors.CodeInternal, appErr.Code)
		assert.Equal(t, "Queue URL not configured", appErr.Message)
		assert.Equal(t, "IMPORT_QUEUE_URL", appErr.Metadata["queue_name"])
	})

	t.Run("import_repo_error_propagates", func(t *testing.T) {
		svc := &ImportExportQueueService{
			sqsClient:  &mockSQSClient{},
			cfg:        &config.Config{Region: "us-east-1", ImportQueueURL: "https://sqs.example.com/import"},
			exportRepo: exportRepo,
			importRepo: &fakeImportRepo{errByID: map[string]error{"i2": stderrors.New("boom")}},
			logger:     zap.NewNop(),
		}

		err := svc.QueueImportJob(ctx, "i2")
		require.Error(t, err)
		assert.Equal(t, "boom", err.Error())
	})

	t.Run("send_failure", func(t *testing.T) {
		mockClient := &mockSQSClient{sendMessageErr: stderrors.New("send failed")}
		svc := &ImportExportQueueService{
			sqsClient:  mockClient,
			cfg:        &config.Config{Region: "us-east-1", ImportQueueURL: "https://sqs.example.com/import"},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueImportJob(ctx, "i1")
		assert.ErrorIs(t, err, ErrImportJobQueue)
	})

	t.Run("success_sends_message_with_expected_shape", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		svc := &ImportExportQueueService{
			sqsClient:  mockClient,
			cfg:        &config.Config{Region: "us-east-1", ImportQueueURL: "https://sqs.example.com/import"},
			exportRepo: exportRepo,
			importRepo: importRepo,
			logger:     zap.NewNop(),
		}

		err := svc.QueueImportJob(ctx, "i1")
		require.NoError(t, err)
		require.Len(t, mockClient.sendMessageInputs, 1)

		input := mockClient.sendMessageInputs[0]
		assert.Equal(t, "https://sqs.example.com/import", aws.ToString(input.QueueUrl))
		assert.Equal(t, int32(5), input.DelaySeconds)

		var decoded ImportJobMessage
		require.NoError(t, json.Unmarshal([]byte(aws.ToString(input.MessageBody)), &decoded))
		assert.Equal(t, "i1", decoded.ImportID)
		assert.Equal(t, "alice", decoded.Username)
		assert.Equal(t, "followers", decoded.Type)
		assert.Equal(t, "merge", decoded.Mode)
		assert.Equal(t, "imports/i1.zip", decoded.S3Key)
		assert.NotZero(t, decoded.Timestamp)
	})
}
