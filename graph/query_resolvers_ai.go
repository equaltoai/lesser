package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/ai"
	aiService "github.com/equaltoai/lesser/pkg/services/ai"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ExplainObject is the resolver for the explainObject field.
func (r *queryResolver) ExplainObject(ctx context.Context, id string) (*model.ObjectExplanation, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get object repository from storage
	objectRepo := r.Registry.GetStorage().Object()
	if objectRepo == nil {
		return nil, ErrObjectRepositoryUnavailable
	}

	// Retrieve the object from storage
	obj, err := objectRepo.GetObject(ctx, id)
	if err != nil {
		// If object not found, check if it's a status ID instead
		// Notes create statuses, not objects in the object repository
		errStr := strings.ToLower(err.Error())
		isNotFound := strings.Contains(errStr, "not found") ||
			strings.Contains(errStr, "notfound") ||
			strings.Contains(errStr, "failed to get") && strings.Contains(errStr, "object")

		// Always try status lookup as fallback (notes are stored as statuses)
		statusRepo := r.Registry.GetStorage().Status()
		if statusRepo != nil {
			status, statusErr := statusRepo.GetStatus(ctx, id)
			if statusErr == nil && status != nil {
				// Convert status to object
				modelObject := r.convertStatusToObject(ctx, status)
				if modelObject != nil {
					// Generate fallback explanation for status
					explanation := r.generateFallbackExplanation(id, modelObject, status)
					r.enrichWithStorageAnalysis(ctx, explanation, id)
					return explanation, nil
				}
			}
		}

		// If we reach here, neither object nor status was found
		if isNotFound {
			r.Logger.Debug("object not found for explanation",
				zap.String("object_id", id),
				zap.Error(err))
		} else {
			r.Logger.Error("failed to get object for explanation",
				zap.String("object_id", id),
				zap.Error(err))
		}
		return nil, errors.Join(errors.New("object not found"), err)
	}

	// Convert to GraphQL model object
	modelObject := r.convertObjectToModel(obj)
	if modelObject == nil {
		return nil, ErrUnableToConvertObject
	}

	// Get AI service for content analysis
	aiSvc := r.Registry.AI()
	var explanation *model.ObjectExplanation

	if aiSvc != nil {
		// AI-powered analysis
		explanation = r.generateAIExplanation(ctx, aiSvc, id, modelObject, obj)
	} else {
		// Fallback to basic structural analysis
		explanation = r.generateFallbackExplanation(id, modelObject, obj)
	}

	// Calculate storage cost and access patterns
	r.enrichWithStorageAnalysis(ctx, explanation, id)

	r.Logger.Debug("Generated object explanation",
		zap.String("object_id", id),
		zap.String("object_type", string(modelObject.Type)),
		zap.Int("size_bytes", explanation.SizeBytes),
		zap.Float64("storage_cost", explanation.StorageCost))

	return explanation, nil
}

// AIAnalysis is the resolver for the aiAnalysis field.
func (r *queryResolver) AiAnalysis(ctx context.Context, objectID string) (*model.AIAnalysis, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get AI service from registry
	aiSvc := r.Registry.AI()
	if aiSvc == nil {
		return nil, ErrAIServiceUnavailable
	}

	// Retrieve analysis from storage
	result, err := aiSvc.GetAnalysis(ctx, &aiService.GetAnalysisQuery{
		ObjectID: objectID,
	})
	if err != nil {
		return nil, errors.Join(errors.New("failed to get AI analysis"), err)
	}

	if result.Analysis == nil {
		return nil, nil // No analysis found
	}

	analysis := result.Analysis

	// Convert to GraphQL model
	return &model.AIAnalysis{
		ID:               analysis.ID,
		ObjectID:         analysis.ObjectID,
		ObjectType:       analysis.ObjectType,
		OverallRisk:      analysis.OverallRisk,
		Confidence:       analysis.Confidence,
		ModerationAction: r.convertModerationAction(analysis.ModerationAction),
		AnalyzedAt:       model.Time(analysis.AnalyzedAt),
		TextAnalysis:     r.convertTextAnalysisToModeration(analysis.TextAnalysis),
		ImageAnalysis:    r.convertImageAnalysisToModeration(analysis.ImageAnalysis),
		SpamAnalysis:     r.convertSpamAnalysis(analysis.SpamAnalysis),
		AiDetection:      r.convertAIDetection(analysis.AIDetection),
	}, nil
}

// AIStats is the resolver for the aiStats field.
func (r *queryResolver) AiStats(ctx context.Context, period model.Period) (*model.AIStats, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get AI service if available
	aiSvc := r.Registry.AI()
	if aiSvc == nil {
		// Return zeros if AI service not available
		return &model.AIStats{
			Period:        string(period),
			TotalAnalyses: 0,
			ToxicContent:  0,
			SpamDetected:  0,
			AiGenerated:   0,
			NsfwContent:   0,
			PiiDetected:   0,
			ToxicityRate:  0,
			SpamRate:      0,
			AiContentRate: 0,
			NsfwRate:      0,
			ModerationActions: &model.ModerationActionCounts{
				None:      0,
				Flag:      0,
				Hide:      0,
				Remove:    0,
				ShadowBan: 0,
				Review:    0,
			},
		}, nil
	}

	// Query REAL stats from the AI service - NO FAKE DATA
	result, err := aiSvc.GetStats(ctx, &aiService.GetStatsQuery{
		Period: string(period),
	})
	if err != nil {
		r.Logger.Error("failed to get AI stats",
			zap.String("user", username),
			zap.String("period", string(period)),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get AI stats"), err)
	}

	// Convert service stats to GraphQL model
	stats, ok := result.Stats.(*ai.AIStats)
	if !ok {
		return nil, ErrUnexpectedStatsType
	}

	// Convert moderation actions map to struct
	moderationActions := &model.ModerationActionCounts{
		None:      stats.ModerationActions[NoneValue],
		Flag:      stats.ModerationActions["flag"],
		Hide:      stats.ModerationActions[ModerationActionHide],
		Remove:    stats.ModerationActions[ModerationActionRemove],
		ShadowBan: stats.ModerationActions["shadowban"],
		Review:    stats.ModerationActions["review"],
	}

	r.Logger.Info("Retrieved REAL AI stats from database",
		zap.String("user", username),
		zap.String("period", string(period)),
		zap.Int("total_analyses", stats.TotalAnalyses))

	return &model.AIStats{
		Period:            stats.Period,
		TotalAnalyses:     stats.TotalAnalyses,
		ToxicContent:      stats.ToxicContent,
		SpamDetected:      stats.SpamDetected,
		AiGenerated:       stats.AIGenerated,
		NsfwContent:       stats.NSFWContent,
		PiiDetected:       stats.PIIDetected,
		ToxicityRate:      stats.ToxicityRate,
		SpamRate:          stats.SpamRate,
		AiContentRate:     stats.AIContentRate,
		NsfwRate:          stats.NSFWRate,
		ModerationActions: moderationActions,
	}, nil
}

// AiCapabilities is the resolver for the aiCapabilities field.
func (r *queryResolver) AiCapabilities(ctx context.Context) (*model.AICapabilities, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Return the AI capabilities of this instance
	return &model.AICapabilities{
		TextAnalysis: &model.TextAnalysisCapabilities{
			SentimentAnalysis: true,
			ToxicityDetection: true,
			SpamDetection:     true,
			PiiDetection:      true,
			EntityExtraction:  true,
			LanguageDetection: true,
		},
		ImageAnalysis: &model.ImageAnalysisCapabilities{
			NsfwDetection:        true,
			ViolenceDetection:    true,
			TextExtraction:       true,
			CelebrityRecognition: false, // Disabled for privacy
			DeepfakeDetection:    true,
		},
		AiDetection: &model.AIDetectionCapabilities{
			AiGeneratedContent: true,
			PatternAnalysis:    true,
			StyleConsistency:   true,
		},
		ModerationActions: []string{"flag", ModerationActionHide, "reject", "quarantine"},
		CostPerAnalysis: &model.CostBreakdown{
			Period:           model.PeriodMonth,
			TotalCost:        0.001,
			DynamoDBCost:     0.0001,
			S3StorageCost:    0.0001,
			LambdaCost:       0.0002,
			DataTransferCost: 0.0005,
			Breakdown:        []*model.CostItem{},
		},
	}, nil
}
