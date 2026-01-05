package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/common"
	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

type exportRepository interface {
	GetExport(ctx context.Context, exportID string) (*models.Export, error)
}

type importRepository interface {
	GetImport(ctx context.Context, importID string) (*models.Import, error)
}

// ImportExportQueueService adapts the canonical per-job queues (Spec 05) to the importexport.QueueService interface.
//
// It loads the persisted import/export records and emits the SQS message shapes expected by
// cmd/import-processor and cmd/export-generator (services.ImportJobMessage / services.ExportJobMessage).
type ImportExportQueueService struct {
	sqsClient  sqsAPI
	cfg        *pkgconfig.Config
	exportRepo exportRepository
	importRepo importRepository
	logger     *zap.Logger
}

var _ importexport.QueueService = (*ImportExportQueueService)(nil)

// NewImportExportQueueService constructs an ImportExportQueueService backed by AWS SQS using
// the canonical per-job queue URLs provided via config (Spec 05).
func NewImportExportQueueService(
	ctx context.Context,
	cfg *pkgconfig.Config,
	exportRepo exportRepository,
	importRepo importRepository,
	logger *zap.Logger,
) (*ImportExportQueueService, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if exportRepo == nil {
		return nil, errors.New("export repository is required")
	}
	if importRepo == nil {
		return nil, errors.New("import repository is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, errors.Join(ErrAWSConfigLoad, err)
	}

	return &ImportExportQueueService{
		sqsClient:  sqs.NewFromConfig(awsCfg),
		cfg:        cfg,
		exportRepo: exportRepo,
		importRepo: importRepo,
		logger:     logger,
	}, nil
}

// QueueExportJob loads the export record and enqueues an export job message to the configured export queue.
func (s *ImportExportQueueService) QueueExportJob(ctx context.Context, exportID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := common.ValidateRequiredParam("export_id", exportID); err != nil {
		return err
	}
	queueURL := s.cfg.ExportQueueURL
	if strings.TrimSpace(queueURL) == "" {
		return apperrors.QueueURLNotConfigured("EXPORT_QUEUE_URL")
	}

	exportRecord, err := s.exportRepo.GetExport(ctx, exportID)
	if err != nil {
		return err
	}

	var dateRange *ExportDateRange
	if exportRecord.DateRange != nil {
		dateRange = &ExportDateRange{
			Start: exportRecord.DateRange.Start,
			End:   exportRecord.DateRange.End,
		}
	}

	msg := ExportJobMessage{
		ExportID:     exportRecord.ID,
		Username:     exportRecord.Username,
		Type:         exportRecord.Type,
		Format:       exportRecord.Format,
		IncludeMedia: exportRecord.IncludeMedia,
		DateRange:    dateRange,
		Options:      exportRecord.Options,
		Timestamp:    time.Now().Unix(),
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return errors.Join(ErrExportJobSerialization, err)
	}

	if _, err := s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(body)),
	}); err != nil {
		return errors.Join(ErrExportJobQueue, err)
	}

	s.logger.Info("queued export job",
		zap.String("export_id", exportRecord.ID),
		zap.String("username", exportRecord.Username),
		zap.String("type", exportRecord.Type),
		zap.String("format", exportRecord.Format))

	return nil
}

// QueueImportJob loads the import record and enqueues an import job message to the configured import queue.
func (s *ImportExportQueueService) QueueImportJob(ctx context.Context, importID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := common.ValidateRequiredParam("import_id", importID); err != nil {
		return err
	}
	queueURL := s.cfg.ImportQueueURL
	if strings.TrimSpace(queueURL) == "" {
		return apperrors.QueueURLNotConfigured("IMPORT_QUEUE_URL")
	}

	importRecord, err := s.importRepo.GetImport(ctx, importID)
	if err != nil {
		return err
	}

	msg := ImportJobMessage{
		ImportID:  importRecord.ID,
		Username:  importRecord.Username,
		Type:      importRecord.Type,
		Mode:      importRecord.Mode,
		S3Key:     importRecord.S3Key,
		Timestamp: time.Now().Unix(),
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return errors.Join(ErrImportJobSerialization, err)
	}

	if _, err := s.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(queueURL),
		MessageBody:  aws.String(string(body)),
		DelaySeconds: 5,
	}); err != nil {
		return errors.Join(ErrImportJobQueue, err)
	}

	s.logger.Info("queued import job",
		zap.String("import_id", importRecord.ID),
		zap.String("username", importRecord.Username),
		zap.String("type", importRecord.Type),
		zap.String("mode", importRecord.Mode))

	return nil
}
