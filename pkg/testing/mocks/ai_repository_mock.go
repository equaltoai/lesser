// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockAIRepository is a mock implementation of interfaces.AIRepository
// using testify/mock for expectation-based testing.
type MockAIRepository struct {
	mock.Mock
}

// NewMockAIRepository creates a new mock AI repository
func NewMockAIRepository() *MockAIRepository {
	return &MockAIRepository{}
}

// ===== Core Analysis Operations =====

// SaveAnalysis mocks the SaveAnalysis method
func (m *MockAIRepository) SaveAnalysis(ctx context.Context, analysis *ai.AIAnalysis) error {
	args := m.Called(ctx, analysis)
	return args.Error(0)
}

// GetAnalysis mocks the GetAnalysis method
func (m *MockAIRepository) GetAnalysis(ctx context.Context, objectID string) (*ai.AIAnalysis, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.AIAnalysis), args.Error(1)
}

// GetAnalysisByID mocks the GetAnalysisByID method
func (m *MockAIRepository) GetAnalysisByID(ctx context.Context, objectID, analysisID string) (*ai.AIAnalysis, error) {
	args := m.Called(ctx, objectID, analysisID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.AIAnalysis), args.Error(1)
}

// ===== Statistics and Metrics =====

// GetStats mocks the GetStats method
func (m *MockAIRepository) GetStats(ctx context.Context, period string) (*ai.AIStats, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.AIStats), args.Error(1)
}

// ===== Queue Operations =====

// QueueForAnalysis mocks the QueueForAnalysis method
func (m *MockAIRepository) QueueForAnalysis(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// ===== Content Analysis =====

// AnalyzeContent mocks the AnalyzeContent method
func (m *MockAIRepository) AnalyzeContent(ctx context.Context, content string, modelType string) (*ai.AIAnalysis, error) {
	args := m.Called(ctx, content, modelType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.AIAnalysis), args.Error(1)
}

// GetContentClassifications mocks the GetContentClassifications method
func (m *MockAIRepository) GetContentClassifications(ctx context.Context, contentID string) ([]string, error) {
	args := m.Called(ctx, contentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// ===== Model Management =====

// UpdateModelPerformance mocks the UpdateModelPerformance method
func (m *MockAIRepository) UpdateModelPerformance(ctx context.Context, modelID string, performanceMetrics map[string]float64) error {
	args := m.Called(ctx, modelID, performanceMetrics)
	return args.Error(0)
}

// ProcessMLFeedback mocks the ProcessMLFeedback method
func (m *MockAIRepository) ProcessMLFeedback(ctx context.Context, analysisID string, feedback map[string]interface{}) error {
	args := m.Called(ctx, analysisID, feedback)
	return args.Error(0)
}

// ===== Health Monitoring =====

// MonitorAIHealth mocks the MonitorAIHealth method
func (m *MockAIRepository) MonitorAIHealth(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Ensure MockAIRepository implements interfaces.AIRepository
var _ interfaces.AIRepository = (*MockAIRepository)(nil)
