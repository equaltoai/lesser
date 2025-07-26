package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
)

// StreamingPreferences constants
const (
	streamingPrefsPrefix = "STREAMING_PREFS#"
	prefsVersionPrefix   = "PREFS_VERSION#"
)

// GetStreamingPreferences retrieves streaming preferences for a user
func (s *dynamoDBStorage) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username},
			"SK": &types.AttributeValueMemberS{Value: "CURRENT"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get streaming preferences: %w", err)
	}

	if result.Item == nil {
		// Return default preferences if none exist
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
			UpdatedAt:         time.Now(),
		}, nil
	}

	var prefs storage.StreamingPreferences
	if err := attributevalue.UnmarshalMap(result.Item, &prefs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal streaming preferences: %w", err)
	}

	return &prefs, nil
}

// UpdateStreamingPreferences updates streaming preferences for a user
func (s *dynamoDBStorage) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	if prefs.Username == "" {
		return fmt.Errorf("username is required")
	}

	// Get current preferences to handle versioning
	currentPrefs, err := s.GetStreamingPreferences(ctx, prefs.Username)
	if err != nil {
		return err
	}

	// Increment version for conflict detection
	prefs.Version = currentPrefs.Version + 1
	prefs.UpdatedAt = time.Now()

	// Marshal preferences
	item, err := attributevalue.MarshalMap(prefs)
	if err != nil {
		return fmt.Errorf("failed to marshal streaming preferences: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: streamingPrefsPrefix + prefs.Username}
	item["SK"] = &types.AttributeValueMemberS{Value: "CURRENT"}

	// Add GSI for device sync queries
	item["GSI1PK"] = &types.AttributeValueMemberS{Value: "USER#" + prefs.Username}
	item["GSI1SK"] = &types.AttributeValueMemberS{Value: "STREAMING_PREFS#" + prefs.UpdatedAt.Format(time.RFC3339)}

	// Update with version check
	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) OR Version = :currentVersion"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":currentVersion": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", currentPrefs.Version)},
		},
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		if isConditionalCheckFailedException(err) {
			return storage.ErrVersionConflict
		}
		return fmt.Errorf("failed to update streaming preferences: %w", err)
	}

	// Store version history
	if err := s.storePreferenceVersion(ctx, prefs); err != nil {
		// Log error but don't fail the update
		fmt.Printf("failed to store preference version: %v\n", err)
	}

	return nil
}

// storePreferenceVersion stores a version of preferences for history
func (s *dynamoDBStorage) storePreferenceVersion(ctx context.Context, prefs *storage.StreamingPreferences) error {
	item, err := attributevalue.MarshalMap(prefs)
	if err != nil {
		return fmt.Errorf("failed to marshal preference version: %w", err)
	}

	// Add version history keys
	item["PK"] = &types.AttributeValueMemberS{Value: streamingPrefsPrefix + prefs.Username}
	item["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("VERSION#%d#%s", prefs.Version, prefs.UpdatedAt.Format(time.RFC3339))}

	// Set TTL to 30 days
	ttl := time.Now().Add(30 * 24 * time.Hour).Unix()
	item["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	return err
}

// GetStreamingPreferencesByDevice retrieves device-specific streaming preferences
func (s *dynamoDBStorage) GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error) {
	// First get user preferences
	userPrefs, err := s.GetStreamingPreferences(ctx, username)
	if err != nil {
		return nil, err
	}

	// Then check for device-specific overrides
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username},
			"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + deviceID},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get device preferences: %w", err)
	}

	if result.Item == nil {
		// No device-specific preferences, return user defaults
		return userPrefs, nil
	}

	var devicePrefs storage.StreamingPreferences
	if err := attributevalue.UnmarshalMap(result.Item, &devicePrefs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal device preferences: %w", err)
	}

	// Merge device preferences with user preferences
	// Device preferences override user preferences
	return &devicePrefs, nil
}

// UpdateDeviceStreamingPreferences updates device-specific streaming preferences
func (s *dynamoDBStorage) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	if prefs.Username == "" || deviceID == "" {
		return fmt.Errorf("username and deviceID are required")
	}

	prefs.DeviceID = deviceID
	prefs.UpdatedAt = time.Now()

	// Marshal preferences
	item, err := attributevalue.MarshalMap(prefs)
	if err != nil {
		return fmt.Errorf("failed to marshal device preferences: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: streamingPrefsPrefix + prefs.Username}
	item["SK"] = &types.AttributeValueMemberS{Value: "DEVICE#" + deviceID}

	// Add GSI for device queries
	item["GSI2PK"] = &types.AttributeValueMemberS{Value: "DEVICE#" + deviceID}
	item["GSI2SK"] = &types.AttributeValueMemberS{Value: "STREAMING_PREFS#" + prefs.Username}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update device streaming preferences: %w", err)
	}

	return nil
}

// GetStreamingPreferenceHistory retrieves the version history of streaming preferences
func (s *dynamoDBStorage) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :version)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":      &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username},
			":version": &types.AttributeValueMemberS{Value: "VERSION#"},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query preference history: %w", err)
	}

	history := make([]*storage.StreamingPreferences, 0, len(result.Items))
	for _, item := range result.Items {
		var prefs storage.StreamingPreferences
		if err := attributevalue.UnmarshalMap(item, &prefs); err != nil {
			continue
		}
		history = append(history, &prefs)
	}

	return history, nil
}

// SyncStreamingPreferences syncs preferences across devices
func (s *dynamoDBStorage) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	// Get source device preferences
	sourcePrefs, err := s.GetStreamingPreferencesByDevice(ctx, username, sourceDeviceID)
	if err != nil {
		return fmt.Errorf("failed to get source device preferences: %w", err)
	}

	// Get all user devices
	devices, err := s.GetUserDevices(ctx, username)
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

		if err := s.UpdateDeviceStreamingPreferences(ctx, &devicePrefs, device.DeviceID); err != nil {
			// Log error but continue with other devices
			fmt.Printf("failed to sync preferences to device %s: %v\n", device.DeviceID, err)
		}
	}

	return nil
}

// ResolvePreferenceConflict resolves conflicts between different preference versions
func (s *dynamoDBStorage) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	// Get all active preference versions
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query preferences: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, storage.ErrNotFound
	}

	// Find the preference to use based on strategy
	var selectedPrefs *storage.StreamingPreferences

	switch strategy {
	case storage.ConflictResolutionLatest:
		// Find the most recently updated preference
		var latestTime time.Time
		for _, item := range result.Items {
			var prefs storage.StreamingPreferences
			if err := attributevalue.UnmarshalMap(item, &prefs); err != nil {
				continue
			}
			if prefs.UpdatedAt.After(latestTime) {
				latestTime = prefs.UpdatedAt
				selectedPrefs = &prefs
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
		for _, item := range result.Items {
			var prefs storage.StreamingPreferences
			if err := attributevalue.UnmarshalMap(item, &prefs); err != nil {
				continue
			}
			if quality, ok := qualityOrder[prefs.DefaultQuality]; ok && quality > highestQuality {
				highestQuality = quality
				selectedPrefs = &prefs
			}
		}

	case storage.ConflictResolutionLowestBandwidth:
		// Select preferences with lowest bandwidth usage
		lowestBandwidth := int64(999999)
		for _, item := range result.Items {
			var prefs storage.StreamingPreferences
			if err := attributevalue.UnmarshalMap(item, &prefs); err != nil {
				continue
			}
			if prefs.MaxBandwidthMbps > 0 && prefs.MaxBandwidthMbps < lowestBandwidth {
				lowestBandwidth = prefs.MaxBandwidthMbps
				selectedPrefs = &prefs
			}
		}
	}

	if selectedPrefs == nil {
		return nil, fmt.Errorf("failed to resolve preference conflict")
	}

	// Update the current preference to the selected one
	selectedPrefs.Version++
	if err := s.UpdateStreamingPreferences(ctx, selectedPrefs); err != nil {
		return nil, err
	}

	return selectedPrefs, nil
}

// MigrateUserPreferences migrates preferences from old schema to new schema
func (s *dynamoDBStorage) MigrateUserPreferences(ctx context.Context, username string, fromVersion, toVersion int) error {
	// Get current preferences
	currentPrefs, err := s.GetStreamingPreferences(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get current preferences: %w", err)
	}

	// Create backup before migration
	if err := s.backupPreferencesBeforeMigration(ctx, username, currentPrefs); err != nil {
		return fmt.Errorf("failed to backup preferences: %w", err)
	}

	// Apply migration based on version
	migratedPrefs, err := s.applyPreferenceMigration(currentPrefs, fromVersion, toVersion)
	if err != nil {
		return fmt.Errorf("failed to apply migration: %w", err)
	}

	// Update with migrated preferences
	if err := s.UpdateStreamingPreferences(ctx, migratedPrefs); err != nil {
		// If migration fails, restore from backup
		s.restorePreferencesFromBackup(ctx, username)
		return fmt.Errorf("failed to update migrated preferences: %w", err)
	}

	return nil
}

// backupPreferencesBeforeMigration creates a backup before migration
func (s *dynamoDBStorage) backupPreferencesBeforeMigration(ctx context.Context, username string, prefs *storage.StreamingPreferences) error {
	item, err := attributevalue.MarshalMap(prefs)
	if err != nil {
		return fmt.Errorf("failed to marshal preferences for backup: %w", err)
	}

	// Add backup keys
	item["PK"] = &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username}
	item["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("BACKUP#%s", time.Now().Format(time.RFC3339))}

	// Set TTL to 90 days
	ttl := time.Now().Add(90 * 24 * time.Hour).Unix()
	item["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	return err
}

// applyPreferenceMigration applies version-specific migrations
func (s *dynamoDBStorage) applyPreferenceMigration(prefs *storage.StreamingPreferences, fromVersion, toVersion int) (*storage.StreamingPreferences, error) {
	migratedPrefs := *prefs // Copy current preferences

	// Apply migrations step by step
	for version := fromVersion; version < toVersion; version++ {
		switch version {
		case 1:
			// Migration from v1 to v2: Add new fields with defaults
			if migratedPrefs.PreferredCodec == "" {
				migratedPrefs.PreferredCodec = "h264"
			}
			if migratedPrefs.BufferSizeSeconds == 0 {
				migratedPrefs.BufferSizeSeconds = 10
			}

		case 2:
			// Migration from v2 to v3: Add HDR support
			if migratedPrefs.HDREnabled == nil {
				enabled := false
				migratedPrefs.HDREnabled = &enabled
			}
			if migratedPrefs.ColorSpace == "" {
				migratedPrefs.ColorSpace = "bt709"
			}

		case 3:
			// Migration from v3 to v4: Add subtitle preferences
			if migratedPrefs.SubtitleLanguage == "" {
				migratedPrefs.SubtitleLanguage = "en"
			}
			if migratedPrefs.SubtitleEnabled == nil {
				enabled := false
				migratedPrefs.SubtitleEnabled = &enabled
			}

		case 4:
			// Migration from v4 to v5: Add accessibility features
			if migratedPrefs.AudioDescriptionEnabled == nil {
				enabled := false
				migratedPrefs.AudioDescriptionEnabled = &enabled
			}
			if migratedPrefs.ClosedCaptionsEnabled == nil {
				enabled := false
				migratedPrefs.ClosedCaptionsEnabled = &enabled
			}

		default:
			return nil, fmt.Errorf("unsupported migration from version %d", version)
		}
	}

	// Update schema version
	migratedPrefs.SchemaVersion = toVersion
	migratedPrefs.UpdatedAt = time.Now()

	return &migratedPrefs, nil
}

// restorePreferencesFromBackup restores preferences from the most recent backup
func (s *dynamoDBStorage) restorePreferencesFromBackup(ctx context.Context, username string) error {
	// Get the most recent backup
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :backup)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username},
			":backup": &types.AttributeValueMemberS{Value: "BACKUP#"},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(1),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to query backups: %w", err)
	}

	if len(result.Items) == 0 {
		return fmt.Errorf("no backup found")
	}

	var backupPrefs storage.StreamingPreferences
	if err := attributevalue.UnmarshalMap(result.Items[0], &backupPrefs); err != nil {
		return fmt.Errorf("failed to unmarshal backup preferences: %w", err)
	}

	// Restore the backup as current preferences
	return s.UpdateStreamingPreferences(ctx, &backupPrefs)
}

// CleanupOldPreferenceVersions removes old preference versions beyond retention period
func (s *dynamoDBStorage) CleanupOldPreferenceVersions(ctx context.Context, username string, retentionDays int) error {
	cutoffTime := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	// Query old versions
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :version)"),
		FilterExpression:       aws.String("UpdatedAt < :cutoff"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":      &types.AttributeValueMemberS{Value: streamingPrefsPrefix + username},
			":version": &types.AttributeValueMemberS{Value: "VERSION#"},
			":cutoff":  &types.AttributeValueMemberS{Value: cutoffTime.Format(time.RFC3339)},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to query old versions: %w", err)
	}

	// Delete old versions in batches
	for i := 0; i < len(result.Items); i += 25 { // DynamoDB batch write limit
		end := i + 25
		if end > len(result.Items) {
			end = len(result.Items)
		}

		var writeRequests []types.WriteRequest
		for j := i; j < end; j++ {
			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"PK": result.Items[j]["PK"],
						"SK": result.Items[j]["SK"],
					},
				},
			})
		}

		batchInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				s.tableName: writeRequests,
			},
		}

		_, err := s.client.BatchWriteItem(ctx, batchInput)
		if err != nil {
			return fmt.Errorf("failed to batch delete old versions: %w", err)
		}
	}

	return nil
}

// ValidatePreferences validates streaming preferences
func (s *dynamoDBStorage) ValidatePreferences(prefs *storage.StreamingPreferences) error {
	// Validate quality setting
	validQualities := map[string]bool{
		"auto":  true,
		"4k":    true,
		"1080p": true,
		"720p":  true,
		"480p":  true,
		"360p":  true,
	}
	if !validQualities[prefs.DefaultQuality] {
		return fmt.Errorf("invalid default quality: %s", prefs.DefaultQuality)
	}

	// Validate codec
	validCodecs := map[string]bool{
		"h264": true,
		"h265": true,
		"av1":  true,
		"vp9":  true,
	}
	if prefs.PreferredCodec != "" && !validCodecs[prefs.PreferredCodec] {
		return fmt.Errorf("invalid preferred codec: %s", prefs.PreferredCodec)
	}

	// Validate bandwidth limits
	if prefs.MaxBandwidthMbps < 0 {
		return fmt.Errorf("max bandwidth cannot be negative")
	}

	// Validate buffer size
	if prefs.BufferSizeSeconds < 1 || prefs.BufferSizeSeconds > 60 {
		return fmt.Errorf("buffer size must be between 1 and 60 seconds")
	}

	return nil
}
