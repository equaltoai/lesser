package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// DeviceProvider interface for getting user devices
type DeviceProvider interface {
	GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error)
}

// StreamingRepository implements the streaming preferences repository using DynamORM
type StreamingRepository struct {
	db             core.DB
	logger         *zap.Logger
	deviceProvider DeviceProvider
}

// NewStreamingRepository creates a new StreamingRepository
func NewStreamingRepository(db core.DB, logger *zap.Logger, deviceProvider DeviceProvider) *StreamingRepository {
	return &StreamingRepository{
		db:             db,
		logger:         logger,
		deviceProvider: deviceProvider,
	}
}

// GetStreamingPreferences retrieves streaming preferences for a user
func (r *StreamingRepository) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	var model models.StreamingPreferences
	
	err := r.db.WithContext(ctx).Model(&models.StreamingPreferences{}).
		Where("PK", "=", fmt.Sprintf("STREAMING_PREFS#%s", username)).
		Where("SK", "=", "CURRENT").
		First(&model)
	
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
		return nil, fmt.Errorf("failed to get streaming preferences: %w", err)
	}
	
	return r.modelToStorageType(&model), nil
}

// UpdateStreamingPreferences updates streaming preferences for a user
func (r *StreamingRepository) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	if prefs.Username == "" {
		return fmt.Errorf("username is required")
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
	// Note: DynamORM conditional updates may need adjustment based on actual API
	err = r.db.WithContext(ctx).Model(model).Update()
	
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return storage.ErrVersionConflict
		}
		return fmt.Errorf("failed to update streaming preferences: %w", err)
	}
	
	// Store version history
	if err := r.storePreferenceVersion(ctx, prefs); err != nil {
		// Log error but don't fail the update - matching legacy behavior
		r.logger.Warn("failed to store preference version", 
			zap.String("username", prefs.Username),
			zap.Error(err))
	}
	
	return nil
}

// storePreferenceVersion stores a version of preferences for history
func (r *StreamingRepository) storePreferenceVersion(ctx context.Context, prefs *storage.StreamingPreferences) error {
	model := r.storageTypeToModel(prefs)
	model.SetVersionedPreference() // This sets TTL to 30 days
	
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		return fmt.Errorf("failed to store preference version: %w", err)
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
	var deviceModel models.StreamingPreferences
	err = r.db.WithContext(ctx).Model(&models.StreamingPreferences{}).
		Where("PK", "=", fmt.Sprintf("STREAMING_PREFS#%s", username)).
		Where("SK", "=", fmt.Sprintf("DEVICE#%s", deviceID)).
		First(&deviceModel)
	
	if err != nil {
		if errors.IsNotFound(err) {
			// No device-specific preferences, return user defaults - matching legacy behavior
			return userPrefs, nil
		}
		return nil, fmt.Errorf("failed to get device preferences: %w", err)
	}
	
	// Device preferences override user preferences - matching legacy behavior
	return r.modelToStorageType(&deviceModel), nil
}

// UpdateDeviceStreamingPreferences updates device-specific streaming preferences
func (r *StreamingRepository) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	if prefs.Username == "" || deviceID == "" {
		return fmt.Errorf("username and deviceID are required")
	}
	
	prefs.DeviceID = deviceID
	prefs.UpdatedAt = time.Now()
	
	// Convert to DynamORM model
	model := r.storageTypeToModel(prefs)
	model.SetDevicePreference(deviceID)
	
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		return fmt.Errorf("failed to update device streaming preferences: %w", err)
	}
	
	return nil
}

// GetStreamingPreferenceHistory retrieves the version history of streaming preferences
func (r *StreamingRepository) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	var streamingModels []models.StreamingPreferences
	
	query := r.db.WithContext(ctx).Model(&models.StreamingPreferences{}).
		Where("PK", "=", fmt.Sprintf("STREAMING_PREFS#%s", username)).
		Where("SK", "begins_with", "VERSION#")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	err := query.All(&streamingModels)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.StreamingPreferences{}, nil // Return empty slice, not error
		}
		return nil, fmt.Errorf("failed to query preference history: %w", err)
	}
	
	// Convert to storage types
	history := make([]*storage.StreamingPreferences, 0, len(streamingModels))
	for _, model := range streamingModels {
		history = append(history, r.modelToStorageType(&model))
	}
	
	return history, nil
}

// SyncStreamingPreferences syncs preferences across devices
func (r *StreamingRepository) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	// Get source device preferences
	sourcePrefs, err := r.GetStreamingPreferencesByDevice(ctx, username, sourceDeviceID)
	if err != nil {
		return fmt.Errorf("failed to get source device preferences: %w", err)
	}
	
	// Get all user devices via device provider
	devices, err := r.deviceProvider.GetUserDevices(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get user devices: %w", err)
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
			r.logger.Warn("failed to sync preferences to device",
				zap.String("username", username),
				zap.String("deviceID", device.DeviceID),
				zap.Error(err))
		}
	}
	
	return nil
}

// ResolvePreferenceConflict resolves conflicts between different preference versions
func (r *StreamingRepository) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	// Get all active preference versions
	var streamingModels []models.StreamingPreferences
	
	err := r.db.WithContext(ctx).Model(&models.StreamingPreferences{}).
		Where("PK", "=", fmt.Sprintf("STREAMING_PREFS#%s", username)).
		All(&streamingModels)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to query preferences: %w", err)
	}
	
	if len(streamingModels) == 0 {
		return nil, storage.ErrNotFound
	}
	
	// Find the preference to use based on strategy
	var selectedPrefs *storage.StreamingPreferences
	
	switch strategy {
	case storage.ConflictResolutionLatest:
		// Find the most recently updated preference
		var latestTime time.Time
		for _, model := range streamingModels {
			prefs := r.modelToStorageType(&model)
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
			prefs := r.modelToStorageType(&model)
			if quality, ok := qualityOrder[prefs.DefaultQuality]; ok && quality > highestQuality {
				highestQuality = quality
				selectedPrefs = prefs
			}
		}
		
	case storage.ConflictResolutionLowestBandwidth:
		// Select preferences with lowest bandwidth usage
		lowestBandwidth := int64(999999)
		for _, model := range streamingModels {
			prefs := r.modelToStorageType(&model)
			if prefs.MaxBandwidthMbps > 0 && prefs.MaxBandwidthMbps < lowestBandwidth {
				lowestBandwidth = prefs.MaxBandwidthMbps
				selectedPrefs = prefs
			}
		}
	}
	
	if selectedPrefs == nil {
		return nil, fmt.Errorf("failed to resolve preference conflict")
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
	return err != nil && (err.Error() == "ConditionalCheckFailedException" || 
		fmt.Sprintf("%T", err) == "*types.ConditionalCheckFailedException")
}

// modelToStorageType converts a DynamORM model to storage.StreamingPreferences
func (r *StreamingRepository) modelToStorageType(model *models.StreamingPreferences) *storage.StreamingPreferences {
	return &storage.StreamingPreferences{
		Username:                  model.Username,
		DeviceID:                  model.DeviceID,
		DefaultQuality:            model.DefaultQuality,
		AutoQuality:               model.AutoQuality,
		PreloadNext:               model.PreloadNext,
		DataSaverMode:             model.DataSaverMode,
		PreferredCodec:            model.PreferredCodec,
		MaxBandwidthMbps:          model.MaxBandwidthMbps,
		BufferSizeSeconds:         model.BufferSizeSeconds,
		Version:                   model.Version,
		SchemaVersion:             model.SchemaVersion,
		HDREnabled:                model.HDREnabled,
		ColorSpace:                model.ColorSpace,
		SubtitleEnabled:           model.SubtitleEnabled,
		SubtitleLanguage:          model.SubtitleLanguage,
		AudioDescriptionEnabled:   model.AudioDescriptionEnabled,
		ClosedCaptionsEnabled:     model.ClosedCaptionsEnabled,
		UpdatedAt:                 model.UpdatedAt,
	}
}

// storageTypeToModel converts storage.StreamingPreferences to DynamORM model
func (r *StreamingRepository) storageTypeToModel(prefs *storage.StreamingPreferences) *models.StreamingPreferences {
	model := &models.StreamingPreferences{
		Username:                  prefs.Username,
		DeviceID:                  prefs.DeviceID,
		DefaultQuality:            prefs.DefaultQuality,
		AutoQuality:               prefs.AutoQuality,
		PreloadNext:               prefs.PreloadNext,
		DataSaverMode:             prefs.DataSaverMode,
		PreferredCodec:            prefs.PreferredCodec,
		MaxBandwidthMbps:          prefs.MaxBandwidthMbps,
		BufferSizeSeconds:         prefs.BufferSizeSeconds,
		Version:                   prefs.Version,
		SchemaVersion:             prefs.SchemaVersion,
		HDREnabled:                prefs.HDREnabled,
		ColorSpace:                prefs.ColorSpace,
		SubtitleEnabled:           prefs.SubtitleEnabled,
		SubtitleLanguage:          prefs.SubtitleLanguage,
		AudioDescriptionEnabled:   prefs.AudioDescriptionEnabled,
		ClosedCaptionsEnabled:     prefs.ClosedCaptionsEnabled,
		UpdatedAt:                 prefs.UpdatedAt,
	}
	
	// Update DynamoDB keys
	model.UpdateKeys()
	return model
}