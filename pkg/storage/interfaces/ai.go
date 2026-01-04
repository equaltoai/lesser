// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/ai"
)

// AIRepository defines the interface for AI analysis operations.
// This handles AI content analysis, moderation, and ML model management.
type AIRepository interface {
	// ===== Core Analysis Operations =====

	// SaveAnalysis stores an AI analysis result
	SaveAnalysis(ctx context.Context, analysis *ai.AIAnalysis) error

	// GetAnalysis retrieves the most recent AI analysis for an object
	GetAnalysis(ctx context.Context, objectID string) (*ai.AIAnalysis, error)

	// GetAnalysisByID retrieves a specific AI analysis by ID
	GetAnalysisByID(ctx context.Context, objectID, analysisID string) (*ai.AIAnalysis, error)

	// ===== Statistics and Metrics =====

	// GetStats retrieves AI analysis statistics for a given period
	GetStats(ctx context.Context, period string) (*ai.AIStats, error)

	// ===== Queue Operations =====

	// QueueForAnalysis marks an object for AI analysis
	QueueForAnalysis(ctx context.Context, objectID string) error

	// ===== Content Analysis =====

	// AnalyzeContent performs comprehensive AI content analysis
	AnalyzeContent(ctx context.Context, content string, modelType string) (*ai.AIAnalysis, error)

	// GetContentClassifications retrieves AI-powered content categorization
	GetContentClassifications(ctx context.Context, contentID string) ([]string, error)

	// ===== Model Management =====

	// UpdateModelPerformance tracks AI model performance with accuracy metrics
	UpdateModelPerformance(ctx context.Context, modelID string, performanceMetrics map[string]float64) error

	// ProcessMLFeedback handles feedback for continuous learning systems
	ProcessMLFeedback(ctx context.Context, analysisID string, feedback map[string]interface{}) error

	// ===== Health Monitoring =====

	// MonitorAIHealth performs health checks on AI processing systems
	MonitorAIHealth(ctx context.Context) error
}
