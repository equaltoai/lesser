// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaMetadataRepository defines the interface for media metadata operations.
// This handles media processing metadata, status tracking, and cleanup.
type MediaMetadataRepository interface {
	// Core metadata operations
	CreateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error
	GetMediaMetadata(ctx context.Context, mediaID string) (*models.MediaMetadata, error)
	UpdateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error
	DeleteMediaMetadata(ctx context.Context, mediaID string) error

	// Status-based queries
	GetMediaMetadataByStatus(ctx context.Context, status string, limit int) ([]*models.MediaMetadata, error)
	GetPendingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error)
	GetProcessingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error)

	// Processing status updates
	MarkProcessingStarted(ctx context.Context, mediaID string) error
	MarkProcessingComplete(ctx context.Context, mediaID string, result ProcessingResult) error
	MarkProcessingFailed(ctx context.Context, mediaID string, errorMsg string) error

	// Cleanup operations
	CleanupExpiredMetadata(ctx context.Context) error
}

// ProcessingResult represents the result of media processing
type ProcessingResult struct {
	Width    int                 `json:"width"`
	Height   int                 `json:"height"`
	Duration int                 `json:"duration"` // Duration in milliseconds
	FileSize int                 `json:"file_size"`
	Blurhash string              `json:"blurhash"`
	Sizes    map[string]SizeInfo `json:"sizes"`
}

// SizeInfo contains information about a processed media size
type SizeInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	S3Key  string `json:"s3_key"`
	URL    string `json:"url"`
}
