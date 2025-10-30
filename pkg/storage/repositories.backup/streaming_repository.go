package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// Streaming status constants
const (
	StatusCurrent = "CURRENT"
)

// DeviceProvider interface for getting user devices
type DeviceProvider interface {
	GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error)
}

// StreamingRepository implements the streaming preferences repository using enhanced patterns
type StreamingRepository struct {
	*EnhancedBaseRepository[*models.StreamingPreferences]
	deviceProvider DeviceProvider
}

// NewStreamingRepository creates a new StreamingRepository with enhanced functionality
func NewStreamingRepository(db core.DB, tableName string, logger *zap.Logger, deviceProvider DeviceProvider, costService *cost.TrackingService) *StreamingRepository {
	// Create enhanced repository optimized for streaming operations
	enhancedRepo := NewEnhancedBaseRepository[*models.StreamingPreferences](db, tableName, logger, costService, "StreamingRepository", "streaming")

	// Set up enhanced services for streaming operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Streaming preferences heavily cached
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for streaming events

	return &StreamingRepository{
		EnhancedBaseRepository: enhancedRepo,
		deviceProvider:         deviceProvider,
	}
}

// GetStreamingPreferences retrieves streaming preferences for a user
func (r *StreamingRepository) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	model := &models.StreamingPreferences{}
	pk := fmt.Sprintf("STREAMING_PREFS#%s", username)
	sk := StatusCurrent

	err := r.Get(ctx, pk, sk, model)
	if err != nil {
		if errors.IsNotFound(err) {
			// Return default preferences if none exist - matching legacy behavior
			return &storage.StreamingPreferences{
				Username:          username,
				DefaultQuality:    "auto",
				AutoQuality:       true,
				PreloadNext:       true,
				DataSaverMode:     false,
				PreferredCodec:    "h264",
				MaxBandwidthMbps:  0, // 0 means unlimited
				BufferSizeSeconds: 10,
				Version:           1,
				SchemaVersion:     1,
				UpdatedAt:         time.Now(),
			}, nil
		}
		return nil, ErrorHandler.HandleGetError(err, "streaming preferences", username)
	}

	return r.modelToStorageType(model), nil
}

// UpdateStreamingPreferences updates streaming preferences for a user
func (r *StreamingRepository) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	if err := common.ValidateRequiredParam("prefs.Username", prefs.Username); err != nil {
		return ErrorHandler.HandleCreateError(ErrStreamingUsernameRequired, "streaming preferences", prefs.Username)
	}

	// Get current preferences to handle versioning
	currentPrefs, err := r.GetStreamingPreferences(ctx, prefs.Username)
	if err != nil {
		return err
	}

	// Increment version for conflict detection
	prefs.Version = currentPrefs.Version + 1
	prefs.UpdatedAt = time.Now()

	// Convert to DynamORM model
	model := r.storageTypeToModel(prefs)
	model.SetCurrentPreference()

	// Update with version check - use conditional update to prevent conflicts
	// Note: BaseRepository's Update method handles basic update operations
	err = r.Update(ctx, model)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return storage.ErrVersionConflict
		}
		return ErrorHandler.HandleUpdateError(err, "streaming preferences", prefs.Username)
	}

	// Store version history
	if err := r.storePreferenceVersion(ctx, prefs); err != nil {
		// Log error but don't fail the update - matching legacy behavior
		// Note: BaseRepository doesn't expose logger directly, so we use a simple approach
		// In production, consider adding a GetLogger() method to BaseRepository
		// For now, we silently continue as intended by legacy behavior
		_ = err // acknowledge error but continue execution
	}

	return nil
}

// storePreferenceVersion stores a version of preferences for history
func (r *StreamingRepository) storePreferenceVersion(ctx context.Context, prefs *storage.StreamingPreferences) error {
	model := r.storageTypeToModel(prefs)
	model.SetVersionedPreference() // This sets TTL to 30 days

	err := r.ValidateAndCreate(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "streaming preferences version", prefs.Username)
	}

	return nil
}

// GetStreamingPreferencesByDevice retrieves device-specific streaming preferences
func (r *StreamingRepository) GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error) {
	// First get user preferences as base
	userPrefs, err := r.GetStreamingPreferences(ctx, username)
	if err != nil {
		return nil, err
	}

	// Then check for device-specific overrides
	deviceModel := &models.StreamingPreferences{}
	pk := fmt.Sprintf("STREAMING_PREFS#%s", username)
	sk := fmt.Sprintf("DEVICE#%s", deviceID)

	err = r.Get(ctx, pk, sk, deviceModel)
	if err != nil {
		if errors.IsNotFound(err) {
			// No device-specific preferences, return user defaults - matching legacy behavior
			return userPrefs, nil
		}
		return nil, ErrorHandler.HandleGetError(err, "device streaming preferences", fmt.Sprintf("%s:%s", username, deviceID))
	}

	// Device preferences override user preferences - matching legacy behavior
	return r.modelToStorageType(deviceModel), nil
}

// UpdateDeviceStreamingPreferences updates device-specific streaming preferences
func (r *StreamingRepository) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	if err := common.ValidateMultipleRequiredParams(map[string]string{"prefs.Username": prefs.Username, "deviceID": deviceID}); err != nil {
		return ErrorHandler.HandleCreateError(ErrStreamingDeviceParamsRequired, "device streaming preferences", fmt.Sprintf("%s:%s", prefs.Username, deviceID))
	}

	prefs.DeviceID = deviceID
	prefs.UpdatedAt = time.Now()

	// Convert to DynamORM model
	model := r.storageTypeToModel(prefs)
	model.SetDevicePreference(deviceID)

	err := r.ValidateAndCreate(ctx, model)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "device streaming preferences", fmt.Sprintf("%s:%s", prefs.Username, deviceID))
	}

	return nil
}

// GetStreamingPreferenceHistory retrieves the version history of streaming preferences
func (r *StreamingRepository) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	pk := fmt.Sprintf("STREAMING_PREFS#%s", username)
	skPrefix := "VERSION#"

	if limit <= 0 {
		limit = 50 // Default limit
	}

	streamingModels, err := r.QueryWithSKPrefix(ctx, pk, skPrefix, limit)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.StreamingPreferences{}, nil // Return empty slice, not error
		}
		return nil, ErrorHandler.HandleQueryError(err, "streaming preferences", "history")
	}

	// Convert to storage types
	history := make([]*storage.StreamingPreferences, 0, len(streamingModels))
	for _, model := range streamingModels {
		history = append(history, r.modelToStorageType(model))
	}

	return history, nil
}

// SyncStreamingPreferences syncs preferences across devices
func (r *StreamingRepository) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	// Get source device preferences
	sourcePrefs, err := r.GetStreamingPreferencesByDevice(ctx, username, sourceDeviceID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "source device streaming preferences", fmt.Sprintf("%s:%s", username, sourceDeviceID))
	}

	// Get all user devices via device provider
	devices, err := r.deviceProvider.GetUserDevices(ctx, username)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "user devices", username)
	}

	// Update preferences on all other devices
	for _, device := range devices {
		if device.DeviceID == sourceDeviceID {
			continue // Skip source device
		}

		// Create a copy of preferences for this device
		devicePrefs := *sourcePrefs
		devicePrefs.DeviceID = device.DeviceID

		if err := r.UpdateDeviceStreamingPreferences(ctx, &devicePrefs, device.DeviceID); err != nil {
			// Log error but continue with other devices - matching legacy behavior
			// Note: BaseRepository doesn't expose logger directly
			// In production, consider adding a GetLogger() method to BaseRepository
			// Continue processing other devices despite individual failures
			_ = err // acknowledge error but continue processing
		}
	}

	return nil
}

// ResolvePreferenceConflict resolves conflicts between different preference versions
func (r *StreamingRepository) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	// Get all active preference versions
	pk := fmt.Sprintf("STREAMING_PREFS#%s", username)
	const preferenceChunkLimit = 100

	var (
		streamingModels []*models.StreamingPreferences
		cursor          string
	)

	for {
		page, err := r.FindWithPagination(ctx, pk, BasePaginationOptions{
			Limit:  preferenceChunkLimit,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, storage.ErrNotFound
			}
			return nil, ErrorHandler.HandleQueryError(err, "streaming preferences", "conflict resolution")
		}

		streamingModels = append(streamingModels, page.Items...)

		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}

	if err := common.ValidateSliceNotEmpty("streamingModels", streamingModels); err != nil {
		return nil, storage.ErrNotFound
	}

	// Find the preference to use based on strategy
	var selectedPrefs *storage.StreamingPreferences

	switch strategy {
	case storage.ConflictResolutionLatest:
		// Find the most recently updated preference
		var latestTime time.Time
		for _, model := range streamingModels {
			prefs := r.modelToStorageType(model)
			if prefs.UpdatedAt.After(latestTime) {
				latestTime = prefs.UpdatedAt
				selectedPrefs = prefs
			}
		}

	case storage.ConflictResolutionHighestQuality:
		// Select preferences with highest quality setting
		qualityOrder := map[string]int{
			"4k":    4,
			"1080p": 3,
			"720p":  2,
			"480p":  1,
			"auto":  0,
		}

		highestQuality := -1
		for _, model := range streamingModels {
			prefs := r.modelToStorageType(model)
			if quality, ok := qualityOrder[prefs.DefaultQuality]; ok && quality > highestQuality {
				highestQuality = quality
				selectedPrefs = prefs
			}
		}

	case storage.ConflictResolutionLowestBandwidth:
		// Select preferences with lowest bandwidth usage
		lowestBandwidth := int64(999999)
		for _, model := range streamingModels {
			prefs := r.modelToStorageType(model)
			if prefs.MaxBandwidthMbps > 0 && prefs.MaxBandwidthMbps < lowestBandwidth {
				lowestBandwidth = prefs.MaxBandwidthMbps
				selectedPrefs = prefs
			}
		}
	}

	if selectedPrefs == nil {
		return nil, ErrorHandler.HandleQueryError(ErrStreamingConflictResolutionFailed, "streaming preferences", "conflict resolution")
	}

	// Update the current preference to the selected one
	selectedPrefs.Version++
	if err := r.UpdateStreamingPreferences(ctx, selectedPrefs); err != nil {
		return nil, err
	}

	return selectedPrefs, nil
}

// Helper function to check for conditional check failed exceptions
func isConditionalCheckFailedException(err error) bool {
	// In DynamORM, conditional check failures are typically wrapped
	// This is a simplified check - may need adjustment based on actual DynamORM error handling
	return ErrorHandler.IsConditionalCheckFailed(err)
}

// modelToStorageType converts a DynamORM model to storage.StreamingPreferences
func (r *StreamingRepository) modelToStorageType(model *models.StreamingPreferences) *storage.StreamingPreferences {
	return &storage.StreamingPreferences{
		Username:          model.Username,
		DeviceID:          model.DeviceID,
		DefaultQuality:    model.DefaultQuality,
		AutoQuality:       model.AutoQuality,
		PreloadNext:       model.PreloadNext,
		DataSaverMode:     model.DataSaverMode,
		PreferredCodec:    model.PreferredCodec,
		MaxBandwidthMbps:  model.MaxBandwidthMbps,
		BufferSizeSeconds: model.BufferSizeSeconds,
		Version:           model.Version,
		SchemaVersion:     model.SchemaVersion,
		HDREnabled: func() bool {
			if model.HDREnabled != nil {
				return *model.HDREnabled
			}
			return false
		}(),
		ColorSpace: model.ColorSpace,
		SubtitleEnabled: func() bool {
			if model.SubtitleEnabled != nil {
				return *model.SubtitleEnabled
			}
			return false
		}(),
		SubtitleLanguage: model.SubtitleLanguage,
		AudioDescriptionEnabled: func() bool {
			if model.AudioDescriptionEnabled != nil {
				return *model.AudioDescriptionEnabled
			}
			return false
		}(),
		ClosedCaptionsEnabled: func() bool {
			if model.ClosedCaptionsEnabled != nil {
				return *model.ClosedCaptionsEnabled
			}
			return false
		}(),
		UpdatedAt: model.UpdatedAt,
	}
}

// storageTypeToModel converts storage.StreamingPreferences to DynamORM model
func (r *StreamingRepository) storageTypeToModel(prefs *storage.StreamingPreferences) *models.StreamingPreferences {
	model := &models.StreamingPreferences{
		Username:                prefs.Username,
		DeviceID:                prefs.DeviceID,
		DefaultQuality:          prefs.DefaultQuality,
		AutoQuality:             prefs.AutoQuality,
		PreloadNext:             prefs.PreloadNext,
		DataSaverMode:           prefs.DataSaverMode,
		PreferredCodec:          prefs.PreferredCodec,
		MaxBandwidthMbps:        prefs.MaxBandwidthMbps,
		BufferSizeSeconds:       prefs.BufferSizeSeconds,
		Version:                 prefs.Version,
		SchemaVersion:           prefs.SchemaVersion,
		HDREnabled:              &prefs.HDREnabled,
		ColorSpace:              prefs.ColorSpace,
		SubtitleEnabled:         &prefs.SubtitleEnabled,
		SubtitleLanguage:        prefs.SubtitleLanguage,
		AudioDescriptionEnabled: &prefs.AudioDescriptionEnabled,
		ClosedCaptionsEnabled:   &prefs.ClosedCaptionsEnabled,
		UpdatedAt:               prefs.UpdatedAt,
	}

	// Update DynamoDB keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation
	return model
}
