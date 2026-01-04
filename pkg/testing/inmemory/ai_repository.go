// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// AIRepository is a thread-safe in-memory implementation of interfaces.AIRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type AIRepository struct {
	mu sync.RWMutex

	// Analyses storage: objectID -> []AIAnalysis (sorted by timestamp desc)
	analysesByObject map[string][]*ai.AIAnalysis

	// Analysis by ID: analysisID -> AIAnalysis
	analysesById map[string]*ai.AIAnalysis

	// Queue for analysis: objectID -> queued
	analysisQueue map[string]bool

	// Model performance metrics: modelID -> metrics
	modelPerformance map[string]map[string]float64

	// ML feedback: analysisID -> feedback
	mlFeedback map[string]map[string]interface{}
}

// NewAIRepository creates a new in-memory AI repository
func NewAIRepository() *AIRepository {
	return &AIRepository{
		analysesByObject: make(map[string][]*ai.AIAnalysis),
		analysesById:     make(map[string]*ai.AIAnalysis),
		analysisQueue:    make(map[string]bool),
		modelPerformance: make(map[string]map[string]float64),
		mlFeedback:       make(map[string]map[string]interface{}),
	}
}

// ===== Core Analysis Operations =====

// SaveAnalysis stores an AI analysis result
func (r *AIRepository) SaveAnalysis(_ context.Context, analysis *ai.AIAnalysis) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if analysis == nil || analysis.ID == "" {
		return fmt.Errorf("analysis ID is required")
	}

	// Store by ID
	r.analysesById[analysis.ID] = analysis

	// Store by object ID (prepend for most recent first)
	r.analysesByObject[analysis.ObjectID] = append([]*ai.AIAnalysis{analysis}, r.analysesByObject[analysis.ObjectID]...)

	// Remove from queue if present
	delete(r.analysisQueue, analysis.ObjectID)

	return nil
}

// GetAnalysis retrieves the most recent AI analysis for an object
func (r *AIRepository) GetAnalysis(_ context.Context, objectID string) (*ai.AIAnalysis, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	analyses := r.analysesByObject[objectID]
	if len(analyses) == 0 {
		return nil, storage.ErrNotFound
	}

	return analyses[0], nil
}

// GetAnalysisByID retrieves a specific AI analysis by ID
func (r *AIRepository) GetAnalysisByID(_ context.Context, _, analysisID string) (*ai.AIAnalysis, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	analysis, exists := r.analysesById[analysisID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return analysis, nil
}

// ===== Statistics and Metrics =====

// GetStats retrieves AI analysis statistics for a given period
func (r *AIRepository) GetStats(_ context.Context, period string) (*ai.AIStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	startDate := r.calculateStartDate(period)

	stats := &ai.AIStats{
		Period:            period,
		ModerationActions: make(map[string]int),
	}

	// Calculate statistics from all analyses
	for _, analyses := range r.analysesByObject {
		for _, analysis := range analyses {
			if analysis.AnalyzedAt.Before(startDate) {
				continue
			}
			r.updateStatsFromAnalysis(stats, analysis)
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

func (r *AIRepository) calculateStartDate(period string) time.Time {
	now := time.Now()
	switch period {
	case "hour":
		return now.Add(-1 * time.Hour)
	case "day":
		return now.AddDate(0, 0, -1)
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, -1, 0)
	default:
		return now.AddDate(0, 0, -1) // Default to 24 hours
	}
}

func (r *AIRepository) updateStatsFromAnalysis(stats *ai.AIStats, analysis *ai.AIAnalysis) {
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

	if analysis.ModerationAction != "" {
		stats.ModerationActions[analysis.ModerationAction]++
	}
}

// ===== Queue Operations =====

// QueueForAnalysis marks an object for AI analysis
func (r *AIRepository) QueueForAnalysis(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.analysisQueue[objectID] = true
	return nil
}

// ===== Content Analysis =====

// AnalyzeContent performs comprehensive AI content analysis
func (r *AIRepository) AnalyzeContent(ctx context.Context, content string, modelType string) (*ai.AIAnalysis, error) {
	// Create a mock analysis result
	analysis := &ai.AIAnalysis{
		ID:         fmt.Sprintf("analysis_%d", time.Now().UnixNano()),
		ObjectType: modelType,
		AnalyzedAt: time.Now(),
		Version:    "1.0",
		TextAnalysis: &ai.TextAnalysis{
			ToxicityScore: 0.1,
			Sentiment:     "NEUTRAL",
			SentimentScores: map[string]float64{
				"POSITIVE": 0.3,
				"NEGATIVE": 0.2,
				"NEUTRAL":  0.5,
			},
		},
		OverallRisk: 0.1,
		Confidence:  0.9,
	}

	// Store the analysis
	if err := r.SaveAnalysis(ctx, analysis); err != nil {
		return nil, err
	}

	return analysis, nil
}

// GetContentClassifications retrieves AI-powered content categorization
func (r *AIRepository) GetContentClassifications(ctx context.Context, contentID string) ([]string, error) {
	analysis, err := r.GetAnalysis(ctx, contentID)
	if err != nil {
		return nil, err
	}

	var classifications []string
	if analysis.TextAnalysis != nil {
		for _, category := range analysis.TextAnalysis.Categories {
			classifications = append(classifications, category.Name)
		}
	}

	return classifications, nil
}

// ===== Model Management =====

// UpdateModelPerformance tracks AI model performance with accuracy metrics
func (r *AIRepository) UpdateModelPerformance(_ context.Context, modelID string, performanceMetrics map[string]float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.modelPerformance[modelID] = performanceMetrics
	return nil
}

// ProcessMLFeedback handles feedback for continuous learning systems
func (r *AIRepository) ProcessMLFeedback(_ context.Context, analysisID string, feedback map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.mlFeedback[analysisID] = feedback
	return nil
}

// ===== Health Monitoring =====

// MonitorAIHealth performs health checks on AI processing systems
func (r *AIRepository) MonitorAIHealth(_ context.Context) error {
	// In-memory implementation always returns healthy
	return nil
}

// ===== Test Helper Methods =====

// GetQueuedObjects returns all objects queued for analysis (test helper)
func (r *AIRepository) GetQueuedObjects() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var objects []string
	for objectID := range r.analysisQueue {
		objects = append(objects, objectID)
	}
	sort.Strings(objects)
	return objects
}

// GetModelPerformance returns model performance metrics (test helper)
func (r *AIRepository) GetModelPerformance(modelID string) map[string]float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.modelPerformance[modelID]
}

// GetMLFeedback returns ML feedback for an analysis (test helper)
func (r *AIRepository) GetMLFeedback(analysisID string) map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.mlFeedback[analysisID]
}

// Clear clears all data (test helper)
func (r *AIRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.analysesByObject = make(map[string][]*ai.AIAnalysis)
	r.analysesById = make(map[string]*ai.AIAnalysis)
	r.analysisQueue = make(map[string]bool)
	r.modelPerformance = make(map[string]map[string]float64)
	r.mlFeedback = make(map[string]map[string]interface{})
}

// Ensure AIRepository implements interfaces.AIRepository
var _ interfaces.AIRepository = (*AIRepository)(nil)
