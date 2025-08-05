package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// AIRepository handles AI analysis data persistence
type AIRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewAIRepository creates a new AI repository
func NewAIRepository(db core.DB, tableName string, logger *zap.Logger) *AIRepository {
	return &AIRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// SaveAnalysis stores an AI analysis result
func (r *AIRepository) SaveAnalysis(ctx context.Context, analysis *ai.AIAnalysis) error {
	// Convert to DynamORM model
	model := &models.AIAnalysis{
		PK:               fmt.Sprintf("AI#%s", analysis.ObjectID),
		SK:               fmt.Sprintf("ANALYSIS#%s", analysis.ID),
		ID:               analysis.ID,
		ObjectID:         analysis.ObjectID,
		ObjectType:       analysis.ObjectType,
		AnalyzedAt:       analysis.AnalyzedAt,
		Version:          analysis.Version,
		TextAnalysis:     analysis.TextAnalysis,
		ImageAnalysis:    analysis.ImageAnalysis,
		AIDetection:      analysis.AIDetection,
		SpamAnalysis:     analysis.SpamAnalysis,
		OverallRisk:      analysis.OverallRisk,
		ModerationAction: analysis.ModerationAction,
		Confidence:       analysis.Confidence,
		GSI4PK:           fmt.Sprintf("AI#ANALYSIS#%s", analysis.AnalyzedAt.Format("2006-01-02")),
		GSI4SK:           analysis.AnalyzedAt.Format(time.RFC3339Nano),
		Type:             "AIAnalysis",
		CreatedAt:        analysis.AnalyzedAt,
	}

	// Create the analysis record
	err := r.db.Model(model).Create()
	if err != nil {
		return fmt.Errorf("failed to save AI analysis: %w", err)
	}

	r.logger.Debug("saved AI analysis",
		zap.String("id", analysis.ID),
		zap.String("object_id", analysis.ObjectID))

	return nil
}

// GetAnalysis retrieves the most recent AI analysis for an object
func (r *AIRepository) GetAnalysis(ctx context.Context, objectID string) (*ai.AIAnalysis, error) {
	var analyses []*models.AIAnalysis

	// Query for analyses of this object
	// Note: DynamORM doesn't support Order method, so we get all and sort manually
	err := r.db.Model(&models.AIAnalysis{}).
		Where("PK", "=", fmt.Sprintf("AI#%s", objectID)).
		Where("SK", "begins_with", "ANALYSIS#").
		Limit(100). // Reasonable limit
		All(&analyses)

	if err != nil {
		return nil, fmt.Errorf("failed to get AI analysis: %w", err)
	}

	if len(analyses) == 0 {
		return nil, fmt.Errorf("no analysis found for object %s", objectID)
	}

	// Sort by SK descending to get most recent first
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].SK > analyses[j].SK
	})

	// Convert back to ai.AIAnalysis
	model := analyses[0]
	return &ai.AIAnalysis{
		ID:               model.ID,
		ObjectID:         model.ObjectID,
		ObjectType:       model.ObjectType,
		AnalyzedAt:       model.AnalyzedAt,
		Version:          model.Version,
		TextAnalysis:     model.TextAnalysis,
		ImageAnalysis:    model.ImageAnalysis,
		AIDetection:      model.AIDetection,
		SpamAnalysis:     model.SpamAnalysis,
		OverallRisk:      model.OverallRisk,
		ModerationAction: model.ModerationAction,
		Confidence:       model.Confidence,
	}, nil
}

// GetAnalysisByID retrieves a specific AI analysis by ID
func (r *AIRepository) GetAnalysisByID(ctx context.Context, objectID, analysisID string) (*ai.AIAnalysis, error) {
	var model models.AIAnalysis

	// Get specific analysis
	err := r.db.Model(&models.AIAnalysis{}).
		Where("PK", "=", fmt.Sprintf("AI#%s", objectID)).
		Where("SK", "=", fmt.Sprintf("ANALYSIS#%s", analysisID)).
		First(&model)

	if err != nil {
		return nil, fmt.Errorf("failed to get AI analysis by ID: %w", err)
	}

	// Convert back to ai.AIAnalysis
	return &ai.AIAnalysis{
		ID:               model.ID,
		ObjectID:         model.ObjectID,
		ObjectType:       model.ObjectType,
		AnalyzedAt:       model.AnalyzedAt,
		Version:          model.Version,
		TextAnalysis:     model.TextAnalysis,
		ImageAnalysis:    model.ImageAnalysis,
		AIDetection:      model.AIDetection,
		SpamAnalysis:     model.SpamAnalysis,
		OverallRisk:      model.OverallRisk,
		ModerationAction: model.ModerationAction,
		Confidence:       model.Confidence,
	}, nil
}

// GetStats retrieves AI analysis statistics for a given period
func (r *AIRepository) GetStats(ctx context.Context, period string) (*ai.AIStats, error) {
	// Calculate date range based on period
	now := time.Now()
	var startDate time.Time
	
	switch period {
	case "hour":
		startDate = now.Add(-1 * time.Hour)
	case "day":
		startDate = now.AddDate(0, 0, -1)
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	default:
		startDate = now.AddDate(0, 0, -1) // Default to 24 hours
	}

	// Query analyses using GSI4
	var analyses []*models.AIAnalysis
	dateStr := startDate.Format("2006-01-02")
	
	err := r.db.Model(&models.AIAnalysis{}).
		Index("cost-date-index"). // GSI4
		Where("GSI4PK", ">=", fmt.Sprintf("AI#ANALYSIS#%s", dateStr)).
		All(&analyses)

	if err != nil {
		return nil, fmt.Errorf("failed to get AI stats: %w", err)
	}

	// Calculate statistics
	stats := &ai.AIStats{
		Period:            period,
		TotalAnalyses:     0,
		ToxicContent:      0,
		SpamDetected:      0,
		AIGenerated:       0,
		NSFWContent:       0,
		PIIDetected:       0,
		ToxicityRate:      0,
		SpamRate:          0,
		AIContentRate:     0,
		NSFWRate:          0,
		ModerationActions: make(map[string]int),
	}

	for _, analysis := range analyses {
		// Only count analyses within our time window
		if analysis.AnalyzedAt.Before(startDate) {
			continue
		}

		stats.TotalAnalyses++
		
		// Count based on analysis results
		if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.7 {
			stats.ToxicContent++
		}
		
		if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.7 {
			stats.SpamDetected++
		}
		
		if analysis.AIDetection != nil && analysis.AIDetection.AIGeneratedProbability > 0.7 {
			stats.AIGenerated++
		}
		
		if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW {
			stats.NSFWContent++
		}
		
		if analysis.TextAnalysis != nil && len(analysis.TextAnalysis.PIIEntities) > 0 {
			stats.PIIDetected++
		}
		
		// Count moderation actions
		if analysis.ModerationAction != "" {
			stats.ModerationActions[analysis.ModerationAction]++
		}
	}

	// Calculate rates
	if stats.TotalAnalyses > 0 {
		stats.ToxicityRate = float64(stats.ToxicContent) / float64(stats.TotalAnalyses)
		stats.SpamRate = float64(stats.SpamDetected) / float64(stats.TotalAnalyses)
		stats.AIContentRate = float64(stats.AIGenerated) / float64(stats.TotalAnalyses)
		stats.NSFWRate = float64(stats.NSFWContent) / float64(stats.TotalAnalyses)
	}

	return stats, nil
}

// QueueForAnalysis marks an object for AI analysis
func (r *AIRepository) QueueForAnalysis(ctx context.Context, objectID string) error {
	// Update the object to trigger analysis (via DynamoDB streams)
	// This is a simplified version - in production you might use a proper queue
	model := &models.AIAnalysisQueue{
		PK:        fmt.Sprintf("OBJECT#%s", objectID),
		SK:        fmt.Sprintf("OBJECT#%s", objectID),
		UpdatedAt: time.Now(),
		ForceAnalysis: true,
	}

	err := r.db.Model(model).Update()
	if err != nil {
		return fmt.Errorf("failed to queue for analysis: %w", err)
	}

	return nil
}