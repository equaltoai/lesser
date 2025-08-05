package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ThreatIntelRepository implements threat intelligence operations using DynamORM
type ThreatIntelRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewThreatIntelRepository creates a new threat intelligence repository
func NewThreatIntelRepository(db core.DB, tableName string, logger *zap.Logger) *ThreatIntelRepository {
	return &ThreatIntelRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
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
	model.UpdateKeys()

	// Store the threat
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to share threat",
			zap.Error(err),
			zap.String("threat_id", threat.ID),
			zap.String("type", threat.ThreatType))
		return fmt.Errorf("failed to share threat: %w", err)
	}

	// Store indicators for fast lookup
	for _, indicator := range threat.Indicators {
		indicatorModel := &models.ThreatIndicator{}
		indicatorModel.UpdateKeys(indicator, threat.ID)

		if err := r.db.WithContext(ctx).Model(indicatorModel).Create(); err != nil {
			r.logger.Warn("Failed to store threat indicator",
				zap.String("indicator", indicator),
				zap.String("threat_id", threat.ID),
				zap.Error(err))
			// Continue with other indicators
		}
	}

	r.logger.Info("Shared threat",
		zap.String("threat_id", threat.ID),
		zap.String("type", threat.ThreatType),
		zap.String("severity", threat.Severity),
		zap.Int("indicators", len(threat.Indicators)))

	return nil
}

// GetSharedThreats retrieves threats shared since a given time
func (r *ThreatIntelRepository) GetSharedThreats(ctx context.Context, since time.Time) ([]*ThreatIntel, error) {
	var models []models.ThreatIntel

	// Query using GSI2 for time-based queries
	err := r.db.WithContext(ctx).Model(&models).
		Index("gsi2").
		Where("GSI2PK", "=", "THREATS").
		Where("GSI2SK", ">", fmt.Sprintf("%d", since.Unix())).
		Limit(100). // Match legacy limit
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query shared threats: %w", err)
	}

	threats := make([]*ThreatIntel, 0, len(models))
	for _, model := range models {
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

		threats = append(threats, threat)
	}

	return threats, nil
}

// GetThreatsByType retrieves threats of a specific type
func (r *ThreatIntelRepository) GetThreatsByType(ctx context.Context, threatType string, limit int) ([]*ThreatIntel, error) {
	var models []models.ThreatIntel

	// Query using GSI1 for type-based queries
	err := r.db.WithContext(ctx).Model(&models).
		Index("gsi1").
		Where("GSI1PK", "=", fmt.Sprintf("TYPE#%s", threatType)).
		Limit(limit).
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query threats by type: %w", err)
	}

	threats := make([]*ThreatIntel, 0, len(models))
	for _, model := range models {
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

		threats = append(threats, threat)
	}

	return threats, nil
}

// UpdateThreatConfidence updates the confidence score of a threat
func (r *ThreatIntelRepository) UpdateThreatConfidence(ctx context.Context, threatID string, newConfidence float64) error {
	// Get the existing threat first
	var model models.ThreatIntel
	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("THREAT#%s", threatID)).
		Where("SK", "=", "METADATA").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("threat not found: %s", threatID)
		}
		return fmt.Errorf("failed to get threat for confidence update: %w", err)
	}

	// Update fields
	model.Confidence = newConfidence
	model.LastSeen = time.Now()
	model.UpdateKeys() // Refresh GSI keys

	// Update the threat
	if err := r.db.WithContext(ctx).Model(&model).Update(); err != nil {
		return fmt.Errorf("failed to update threat confidence: %w", err)
	}

	return nil
}

// IncrementHitCount increments the hit count for a threat
func (r *ThreatIntelRepository) IncrementHitCount(ctx context.Context, threatID string) error {
	// Get the existing threat first
	var model models.ThreatIntel
	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("THREAT#%s", threatID)).
		Where("SK", "=", "METADATA").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Warn("Threat not found for hit count increment",
				zap.String("threat_id", threatID))
			return nil // Don't fail on missing threats
		}
		return fmt.Errorf("failed to get threat for hit count update: %w", err)
	}

	// Update fields
	model.HitCount++
	model.LastSeen = time.Now()
	model.UpdateKeys() // Refresh GSI keys with new LastSeen

	// Update the threat
	if err := r.db.WithContext(ctx).Model(&model).Update(); err != nil {
		r.logger.Warn("Failed to increment hit count",
			zap.String("threat_id", threatID),
			zap.Error(err))
		return nil // Don't fail the main operation
	}

	return nil
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
		return nil, fmt.Errorf("failed to scan active threats: %w", err)
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

		threats = append(threats, threat)
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
			return nil, fmt.Errorf("threat not found: %s", threatID)
		}
		return nil, fmt.Errorf("failed to get threat: %w", err)
	}

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

	return threat, nil
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
		return "", fmt.Errorf("failed to lookup indicator: %w", err)
	}

	return model.ThreatID, nil
}