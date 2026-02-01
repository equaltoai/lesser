package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// ThreatIntelRepository implements threat intelligence operations using BaseRepository
type ThreatIntelRepository struct {
	*EnhancedBaseRepository[*models.ThreatIntel]
	queryUtils *QueryUtils
}

// NewThreatIntelRepository creates a new threat intelligence repository
func NewThreatIntelRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ThreatIntelRepository {
	// Create enhanced repository optimized for threat intel operations
	enhancedRepo := NewEnhancedBaseRepository[*models.ThreatIntel](db, tableName, logger, costService, "ThreatIntelRepository", "threat_intel")

	// Set up enhanced services for threat intel operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &ThreatIntelRepository{
		EnhancedBaseRepository: enhancedRepo,
		queryUtils:             NewQueryUtils(db, logger),
	}
}

// ThreatIntel represents threat intelligence data for the repository interface
type ThreatIntel struct {
	ID           string
	ThreatType   string
	Indicators   []string
	Severity     string
	Description  string
	SourceDomain string
	FirstSeen    time.Time
	LastSeen     time.Time
	HitCount     int64
	Confidence   float64
	TTL          time.Duration
}

// ShareThreat stores a threat in DynamoDB and creates indicator mappings
func (r *ThreatIntelRepository) ShareThreat(ctx context.Context, threat *ThreatIntel) error {
	// Set default TTL if not provided
	if threat.TTL == 0 {
		threat.TTL = 7 * 24 * time.Hour // Default 7 days
	}

	// Create threat model
	model := &models.ThreatIntel{
		ID:           threat.ID,
		ThreatType:   threat.ThreatType,
		Severity:     threat.Severity,
		Description:  threat.Description,
		Indicators:   threat.Indicators,
		FirstSeen:    threat.FirstSeen,
		LastSeen:     threat.LastSeen,
		HitCount:     threat.HitCount,
		Confidence:   threat.Confidence,
		SourceDomain: threat.SourceDomain,
		TTL:          time.Now().Add(threat.TTL).Unix(),
	}
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Store the threat
	if err := r.ValidateAndCreate(ctx, model); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityThreatIntel, threat.ID)
	}

	// Store indicators for fast lookup using BaseRepository for ThreatIndicator
	for _, indicator := range threat.Indicators {
		indicatorModel := &models.ThreatIndicator{}
		if err := indicatorModel.UpdateKeys(indicator, threat.ID); err != nil {
			r.logger.Warn("failed to update indicator keys", zap.Error(err))
			continue
		}

		// Use GetDB() for ThreatIndicator since BaseRepository is typed for ThreatIntel
		if err := r.GetDB().WithContext(ctx).Model(indicatorModel).Create(); err != nil {
			// Continue with other indicators on error
			// Log the error but don't fail the entire operation
			r.logger.Warn("failed to create threat indicator",
				zap.String("threat_id", indicatorModel.ThreatID),
				zap.Error(err))
		}
	}

	r.logger.Info("Shared threat",
		zap.String("threat_id", threat.ID),
		zap.String("type", threat.ThreatType),
		zap.String("severity", threat.Severity),
		zap.Int("indicators", len(threat.Indicators)))

	return nil
}

// convertResultToThreats converts query results to threat intel objects
func (r *ThreatIntelRepository) convertResultToThreats(result *QueryResult[map[string]interface{}]) []*ThreatIntel {
	threats := make([]*ThreatIntel, 0, len(result.Items))
	for _, item := range result.Items {
		threat := r.mapItemToThreatIntel(item)
		if threat != nil {
			threats = append(threats, threat)
		}
	}
	return threats
}

// GetSharedThreats retrieves threats shared since a given time
func (r *ThreatIntelRepository) GetSharedThreats(ctx context.Context, since time.Time) ([]*ThreatIntel, error) {
	// Use TimeRangeQuery for time-based filtering
	result, err := r.queryUtils.TimeRangeQuery(ctx, "THREATS", since.Unix(), 0, &QueryOptions{
		Limit:     100,
		IndexName: "gsi2",
	})

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityThreatIntel, "shared threats")
	}

	return r.convertResultToThreats(result), nil
}

// GetThreatsByType retrieves threats of a specific type
func (r *ThreatIntelRepository) GetThreatsByType(ctx context.Context, threatType string, limit int) ([]*ThreatIntel, error) {
	// Use QueryByGSI for type-based queries
	result, err := r.queryUtils.QueryByGSI(ctx, "gsi1", fmt.Sprintf("TYPE#%s", threatType), "", &QueryOptions{
		Limit: limit,
	})

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityThreatIntel, "threats by type")
	}

	return r.convertResultToThreats(result), nil
}

// updateThreat is a generic helper for updating threat fields
func (r *ThreatIntelRepository) updateThreat(ctx context.Context, threatID string, updateFunc func(*models.ThreatIntel), ignoreMissing bool) error {
	// Get the existing threat first
	var model models.ThreatIntel
	err := r.queryUtils.GetItemByPK(ctx, fmt.Sprintf("THREAT#%s", threatID), "METADATA", &model)

	if err != nil {
		if errors.IsNotFound(err) {
			if ignoreMissing {
				r.logger.Warn("Threat not found for update",
					zap.String("threat_id", threatID))
				return nil // Don't fail on missing threats
			}
			return ErrorHandler.HandleGetError(err, EntityThreatIntel, threatID)
		}
		return ErrorHandler.HandleGetError(err, EntityThreatIntel, threatID)
	}

	// Apply the update function
	updateFunc(&model)
	model.LastSeen = time.Now()
	_ = model.UpdateKeys() // Ignore error as this is internal model operation // Refresh GSI keys

	// Update the threat using generic update
	if err := r.queryUtils.UpdateItem(ctx, &model); err != nil {
		if ignoreMissing {
			r.logger.Warn("Failed to update threat",
				zap.String("threat_id", threatID),
				zap.Error(err))
			return nil // Don't fail the main operation
		}
		return ErrorHandler.HandleUpdateError(err, EntityThreatIntel, threatID)
	}

	return nil
}

// UpdateThreatConfidence updates the confidence score of a threat
func (r *ThreatIntelRepository) UpdateThreatConfidence(ctx context.Context, threatID string, newConfidence float64) error {
	return r.updateThreat(ctx, threatID, func(model *models.ThreatIntel) {
		model.Confidence = newConfidence
	}, false)
}

// IncrementHitCount increments the hit count for a threat
func (r *ThreatIntelRepository) IncrementHitCount(ctx context.Context, threatID string) error {
	return r.updateThreat(ctx, threatID, func(model *models.ThreatIntel) {
		model.HitCount++
	}, true)
}

// convertModelToThreat converts a ThreatIntel model to domain object
func (r *ThreatIntelRepository) convertModelToThreat(model *models.ThreatIntel) *ThreatIntel {
	threat := &ThreatIntel{
		ID:           model.ID,
		ThreatType:   model.ThreatType,
		Severity:     model.Severity,
		Description:  model.Description,
		Indicators:   model.Indicators,
		FirstSeen:    model.FirstSeen,
		LastSeen:     model.LastSeen,
		HitCount:     model.HitCount,
		Confidence:   model.Confidence,
		SourceDomain: model.SourceDomain,
	}

	// Calculate TTL duration from Unix timestamp
	if model.TTL > 0 {
		threat.TTL = time.Until(time.Unix(model.TTL, 0))
	}

	return threat
}

// LoadActiveThreats loads all active (non-expired) threats
func (r *ThreatIntelRepository) LoadActiveThreats(ctx context.Context) ([]*ThreatIntel, error) {
	var models []models.ThreatIntel

	// We need to scan since DynamORM doesn't support filter expressions directly
	// This matches the legacy scan operation
	err := r.db.WithContext(ctx).Model(&models).
		Where("SK", "=", "METADATA"). // Only get threat metadata records
		All(&models)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityThreatIntel, "active threats")
	}

	threats := make([]*ThreatIntel, 0)
	now := time.Now().Unix()

	for _, model := range models {
		// Filter out expired threats (TTL check)
		if model.TTL > 0 && model.TTL <= now {
			continue
		}

		// Only include threats (filter by PK prefix)
		if len(model.PK) < 7 || model.PK[:7] != "THREAT#" {
			continue
		}

		threats = append(threats, r.convertModelToThreat(&model))
	}

	r.logger.Info("Loaded active threats", zap.Int("count", len(threats)))

	return threats, nil
}

// GetThreatByID retrieves a specific threat by ID
func (r *ThreatIntelRepository) GetThreatByID(ctx context.Context, threatID string) (*ThreatIntel, error) {
	var model models.ThreatIntel

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("THREAT#%s", threatID)).
		Where("SK", "=", "METADATA").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityThreatIntel, threatID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityThreatIntel, threatID)
	}

	return r.convertModelToThreat(&model), nil
}

// GetIndicatorThreat looks up threat ID by indicator
func (r *ThreatIntelRepository) GetIndicatorThreat(ctx context.Context, indicator string) (string, error) {
	var model models.ThreatIndicator

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("INDICATOR#%s", indicator)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil // No threat found for this indicator
		}
		return "", ErrorHandler.HandleGetError(err, EntityThreatIndicator, indicator)
	}

	return model.ThreatID, nil
}

// mapItemToThreatIntel converts a map[string]interface{} to ThreatIntel
func (r *ThreatIntelRepository) mapItemToThreatIntel(item map[string]interface{}) *ThreatIntel {
	threat := &ThreatIntel{}

	// Map fields from the item
	if id, ok := item["ID"].(string); ok {
		threat.ID = id
	}
	if threatType, ok := item["ThreatType"].(string); ok {
		threat.ThreatType = threatType
	}
	if severity, ok := item["Severity"].(string); ok {
		threat.Severity = severity
	}
	if description, ok := item["Description"].(string); ok {
		threat.Description = description
	}
	if indicators, ok := item["Indicators"].([]string); ok {
		threat.Indicators = indicators
	}
	if firstSeen, ok := item["FirstSeen"].(time.Time); ok {
		threat.FirstSeen = firstSeen
	}
	if lastSeen, ok := item["LastSeen"].(time.Time); ok {
		threat.LastSeen = lastSeen
	}
	if hitCount, ok := item["HitCount"].(int64); ok {
		threat.HitCount = hitCount
	}
	if confidence, ok := item["Confidence"].(float64); ok {
		threat.Confidence = confidence
	}
	if sourceDomain, ok := item["SourceDomain"].(string); ok {
		threat.SourceDomain = sourceDomain
	}

	// Calculate TTL duration from Unix timestamp
	if ttl, ok := item["TTL"].(int64); ok && ttl > 0 {
		threat.TTL = time.Until(time.Unix(ttl, 0))
	}

	return threat
}
