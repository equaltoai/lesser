// Package importexport provides data portability services for the Lesser ActivityPub server.
//
// This service handles all operations related to data import and export including:
// - Creating and managing export requests (archive, followers, following, etc.)
// - Processing import requests from various formats (ActivityPub, Mastodon, CSV)
// - Handling large datasets asynchronously with progress tracking
// - Managing media attachments in exports/imports
// - Emitting progress events for real-time status updates
package importexport

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides business logic for import/export operations
type Service struct {
	exportRepo    *repositories.ExportRepository
	importRepo    *repositories.ImportRepository
	statusRepo    *repositories.StatusRepository
	accountRepo   interfaces.AccountRepository
	mediaRepo     *repositories.MediaRepository
	socialRepo    interfaces.SocialRepository
	publisher     streaming.Publisher
	queueService  QueueService
	storageClient StorageClient
	logger        *zap.Logger
	domain        string
}

// QueueService defines the interface for queuing async operations
type QueueService interface {
	QueueExportJob(ctx context.Context, exportID string) error
	QueueImportJob(ctx context.Context, importID string) error
}

// StorageClient defines the interface for file storage operations
type StorageClient interface {
	GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	UploadFile(ctx context.Context, key string, data []byte) error
	GetFile(ctx context.Context, key string) ([]byte, error)
}

// NewService creates a new import/export service
func NewService(
	exportRepo *repositories.ExportRepository,
	importRepo *repositories.ImportRepository,
	statusRepo *repositories.StatusRepository,
	accountRepo interfaces.AccountRepository,
	mediaRepo *repositories.MediaRepository,
	socialRepo interfaces.SocialRepository,
	publisher streaming.Publisher,
	queueService QueueService,
	storageClient StorageClient,
	logger *zap.Logger,
	domain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		exportRepo:    exportRepo,
		importRepo:    importRepo,
		statusRepo:    statusRepo,
		accountRepo:   accountRepo,
		mediaRepo:     mediaRepo,
		socialRepo:    socialRepo,
		publisher:     publisher,
		queueService:  queueService,
		storageClient: storageClient,
		logger:        logger,
		domain:        domain,
	}
}

// Export Commands and Queries (following CQRS pattern)

// CreateExportCommand contains data needed to create an export request
type CreateExportCommand struct {
	Username     string            `json:"username" validate:"required"`
	Type         string            `json:"type" validate:"required,oneof=archive followers following lists bookmarks mutes blocks"`
	Format       string            `json:"format" validate:"required,oneof=activitypub mastodon csv"`
	IncludeMedia bool              `json:"include_media"`
	DateRange    *DateRange        `json:"date_range"`
	Options      map[string]string `json:"options"`
	RequestedBy  string            `json:"requested_by" validate:"required"`
}

// DateRange specifies the time range for exports
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// GetExportQuery contains parameters for retrieving an export
type GetExportQuery struct {
	ExportID string `json:"export_id" validate:"required"`
	Username string `json:"username" validate:"required"` // For authorization
}

// ListExportsQuery contains parameters for listing exports
type ListExportsQuery struct {
	Username   string                       `json:"username" validate:"required"`
	Status     string                       `json:"status"` // pending, processing, completed, failed
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// CancelExportCommand contains data needed to cancel an export
type CancelExportCommand struct {
	ExportID string `json:"export_id" validate:"required"`
	Username string `json:"username" validate:"required"` // For authorization
}

// Import Commands and Queries

// CreateImportCommand contains data needed to create an import request
type CreateImportCommand struct {
	Username      string            `json:"username" validate:"required"`
	Type          string            `json:"type" validate:"required,oneof=archive followers following lists bookmarks"`
	Format        string            `json:"format" validate:"required,oneof=activitypub mastodon csv"`
	FileURL       string            `json:"file_url" validate:"required,url"`
	Options       map[string]string `json:"options"`
	MergeStrategy string            `json:"merge_strategy" validate:"required,oneof=merge replace skip"`
	RequestedBy   string            `json:"requested_by" validate:"required"`
}

// GetImportQuery contains parameters for retrieving an import
type GetImportQuery struct {
	ImportID string `json:"import_id" validate:"required"`
	Username string `json:"username" validate:"required"` // For authorization
}

// ListImportsQuery contains parameters for listing imports
type ListImportsQuery struct {
	Username   string                       `json:"username" validate:"required"`
	Status     string                       `json:"status"` // pending, processing, completed, failed
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// Result types

// ExportResult contains the result of an export operation
type ExportResult struct {
	Export      *models.Export    `json:"export"`
	DownloadURL string            `json:"download_url,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	Events      []streaming.Event `json:"-"` // Events emitted during operation
}

// ImportResult contains the result of an import operation
type ImportResult struct {
	Import    *models.Import    `json:"import"`
	Processed int               `json:"processed"`
	Skipped   int               `json:"skipped"`
	Failed    int               `json:"failed"`
	Events    []streaming.Event `json:"-"` // Events emitted during operation
}

// ExportListResult contains a paginated list of exports
type ExportListResult struct {
	Exports    []*models.Export  `json:"exports"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
	Events     []streaming.Event `json:"-"`
}

// ImportListResult contains a paginated list of imports
type ImportListResult struct {
	Imports    []*models.Import  `json:"imports"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
	Events     []streaming.Event `json:"-"`
}

// CreateExport creates a new export request and queues it for processing
func (s *Service) CreateExport(ctx context.Context, cmd *CreateExportCommand) (*ExportResult, error) {
	// Validate command
	if err := s.validateCreateExportCommand(cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Create export record
	export := &models.Export{
		ID:           uuid.New().String(),
		Username:     cmd.Username,
		Type:         cmd.Type,
		Format:       cmd.Format,
		Status:       "pending",
		IncludeMedia: cmd.IncludeMedia,
		Options:      convertStringMapToAny(cmd.Options),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if cmd.DateRange != nil {
		export.DateRange = &models.ExportDateRange{
			Start: cmd.DateRange.Start,
			End:   cmd.DateRange.End,
		}
	}

	// Save to repository
	if err := s.exportRepo.CreateExport(ctx, export); err != nil {
		s.logger.Error("failed to create export",
			zap.String("username", cmd.Username),
			zap.String("type", cmd.Type),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create export: %w", err)
	}

	// Queue for processing
	if err := s.queueService.QueueExportJob(ctx, export.ID); err != nil {
		s.logger.Error("failed to queue export",
			zap.String("export_id", export.ID),
			zap.Error(err))
		// Update status to failed
		export.Status = "failed"
		export.Error = fmt.Sprintf("Failed to queue: %v", err)
		_ = s.exportRepo.UpdateExportStatus(ctx, export.ID, "failed", nil, export.Error)
		return nil, fmt.Errorf("failed to queue export: %w", err)
	}

	// Emit event for real-time updates
	events := []streaming.Event{
		s.createExportEvent("export.created", export),
	}

	if s.publisher != nil {
		for _, event := range events {
			if err := s.publisher.PublishToUser(ctx, cmd.Username, &event); err != nil {
				s.logger.Warn("failed to publish export event",
					zap.String("username", cmd.Username),
					zap.Error(err))
			}
		}
	}

	return &ExportResult{
		Export: export,
		Events: events,
	}, nil
}

// GetExport retrieves an export by ID
func (s *Service) GetExport(ctx context.Context, query *GetExportQuery) (*ExportResult, error) {
	export, err := s.exportRepo.GetExport(ctx, query.ExportID)
	if err != nil {
		return nil, fmt.Errorf("failed to get export: %w", err)
	}

	// Verify ownership
	if export.Username != query.Username {
		return nil, fmt.Errorf("unauthorized")
	}

	result := &ExportResult{
		Export: export,
	}

	// If completed, generate download URL
	if export.Status == "completed" && export.DownloadURL != "" {
		if s.storageClient != nil {
			url, err := s.storageClient.GeneratePresignedURL(ctx, export.S3Key, 24*time.Hour)
			if err != nil {
				s.logger.Warn("failed to generate download URL",
					zap.String("export_id", export.ID),
					zap.Error(err))
			} else {
				result.DownloadURL = url
				expiresAt := time.Now().Add(24 * time.Hour)
				result.ExpiresAt = &expiresAt
			}
		} else {
			// Use stored download URL if no storage client
			result.DownloadURL = export.DownloadURL
			result.ExpiresAt = export.ExpiresAt
		}
	}

	return result, nil
}

// ListExports lists exports for a user
func (s *Service) ListExports(ctx context.Context, query *ListExportsQuery) (*ExportListResult, error) {
	exports, nextCursor, err := s.exportRepo.GetExportsForUser(ctx, query.Username, query.Pagination.Limit, query.Pagination.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to list exports: %w", err)
	}

	return &ExportListResult{
		Exports:    exports,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}, nil
}

// UpdateExportProgress updates the progress of an export (called by background processor)
func (s *Service) UpdateExportProgress(ctx context.Context, exportID string, processed, total int) error {
	export, err := s.exportRepo.GetExport(ctx, exportID)
	if err != nil {
		return fmt.Errorf("failed to get export: %w", err)
	}

	// Update status using repository method
	completionData := map[string]interface{}{
		"record_count": processed,
	}

	if err := s.exportRepo.UpdateExportStatus(ctx, exportID, "processing", completionData, ""); err != nil {
		return fmt.Errorf("failed to update export: %w", err)
	}

	// Emit progress event
	event := s.createProgressEvent("export.progress", export, processed, total)
	if s.publisher != nil {
		if err := s.publisher.PublishToUser(ctx, export.Username, &event); err != nil {
			s.logger.Warn("failed to publish progress event",
				zap.String("export_id", exportID),
				zap.Error(err))
		}
	}

	return nil
}

// CompleteExport marks an export as completed (called by background processor)
func (s *Service) CompleteExport(ctx context.Context, exportID string, fileURL string, fileSize int64) error {
	export, err := s.exportRepo.GetExport(ctx, exportID)
	if err != nil {
		return fmt.Errorf("failed to get export: %w", err)
	}

	// Update status using repository method
	completionData := map[string]interface{}{
		"download_url": fileURL,
		"file_size":    int(fileSize),
		"s3_key":       fileURL,                            // Assuming fileURL is actually the S3 key
		"expires_at":   time.Now().Add(7 * 24 * time.Hour), // 7 days expiry
	}

	if err := s.exportRepo.UpdateExportStatus(ctx, exportID, "completed", completionData, ""); err != nil {
		return fmt.Errorf("failed to update export: %w", err)
	}

	// Emit completion event
	event := s.createExportEvent("export.completed", export)
	if s.publisher != nil {
		if err := s.publisher.PublishToUser(ctx, export.Username, &event); err != nil {
			s.logger.Warn("failed to publish completion event",
				zap.String("export_id", exportID),
				zap.Error(err))
		}
	}

	return nil
}

// Helper methods

func (s *Service) validateCreateExportCommand(cmd *CreateExportCommand) error {
	// Check if user exists
	account, err := s.accountRepo.GetAccount(context.Background(), cmd.Username)
	if err != nil || account == nil {
		return fmt.Errorf("user not found")
	}

	// Validate date range if provided
	if cmd.DateRange != nil {
		if cmd.DateRange.Start.After(cmd.DateRange.End) {
			return fmt.Errorf("invalid date range: start date after end date")
		}
		if cmd.DateRange.End.After(time.Now()) {
			return fmt.Errorf("invalid date range: end date in the future")
		}
	}

	return nil
}

func (s *Service) createExportEvent(eventType string, export *models.Export) streaming.Event {
	return streaming.Event{
		Type:   eventType,
		Stream: "user",
		Payload: map[string]interface{}{
			"export": export,
		},
		Timestamp: time.Now(),
	}
}

func (s *Service) createProgressEvent(eventType string, export *models.Export, processed, total int) streaming.Event {
	return streaming.Event{
		Type:   eventType,
		Stream: "user",
		Payload: map[string]interface{}{
			"export_id": export.ID,
			"processed": processed,
			"total":     total,
			"percent":   float64(processed) / float64(total) * 100,
		},
		Timestamp: time.Now(),
	}
}

// convertStringMapToAny converts map[string]string to map[string]any
func convertStringMapToAny(input map[string]string) map[string]any {
	result := make(map[string]any)
	for k, v := range input {
		result[k] = v
	}
	return result
}
