// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/types"
	"github.com/stretchr/testify/mock"
)

// MockMediaSessionRepository is a mock implementation of interfaces.MediaSessionRepository
// using testify/mock for expectation-based testing.
type MockMediaSessionRepository struct {
	mock.Mock
}

// NewMockMediaSessionRepository creates a new mock media session repository
func NewMockMediaSessionRepository() *MockMediaSessionRepository {
	return &MockMediaSessionRepository{}
}

// ===== Session Lifecycle Operations =====

// StartStreamingSession mocks the StartStreamingSession method
func (m *MockMediaSessionRepository) StartStreamingSession(ctx context.Context, userID, mediaID string, format types.MediaFormat, quality types.Quality) (*types.StreamingSession, error) {
	args := m.Called(ctx, userID, mediaID, format, quality)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.StreamingSession), args.Error(1)
}

// EndStreamingSession mocks the EndStreamingSession method
func (m *MockMediaSessionRepository) EndStreamingSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

// UpdateStreamingMetrics mocks the UpdateStreamingMetrics method
func (m *MockMediaSessionRepository) UpdateStreamingMetrics(ctx context.Context, sessionID string, segmentIndex int, bytesTransferred int64, bufferHealth float64, currentQuality types.Quality) error {
	args := m.Called(ctx, sessionID, segmentIndex, bytesTransferred, bufferHealth, currentQuality)
	return args.Error(0)
}

// ===== Legacy Session Operations =====

// CreateSession mocks the CreateSession method
func (m *MockMediaSessionRepository) CreateSession(ctx context.Context, session *types.StreamingSession) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

// GetSession mocks the GetSession method
func (m *MockMediaSessionRepository) GetSession(ctx context.Context, sessionID string) (*types.StreamingSession, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.StreamingSession), args.Error(1)
}

// UpdateSession mocks the UpdateSession method
func (m *MockMediaSessionRepository) UpdateSession(ctx context.Context, session *types.StreamingSession) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

// EndSession mocks the EndSession method
func (m *MockMediaSessionRepository) EndSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

// ===== Session Queries =====

// GetActiveStreams mocks the GetActiveStreams method
func (m *MockMediaSessionRepository) GetActiveStreams(ctx context.Context, limit int) ([]*types.StreamingSession, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.StreamingSession), args.Error(1)
}

// GetUserSessions mocks the GetUserSessions method
func (m *MockMediaSessionRepository) GetUserSessions(ctx context.Context, userID string) ([]*types.StreamingSession, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.StreamingSession), args.Error(1)
}

// GetMediaSessions mocks the GetMediaSessions method
func (m *MockMediaSessionRepository) GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*types.StreamingSession, error) {
	args := m.Called(ctx, mediaID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.StreamingSession), args.Error(1)
}

// GetSessionsByTimeRange mocks the GetSessionsByTimeRange method
func (m *MockMediaSessionRepository) GetSessionsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int32) ([]*types.StreamingSession, error) {
	args := m.Called(ctx, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.StreamingSession), args.Error(1)
}

// ===== Session Validation and Access =====

// ValidateSessionAccess mocks the ValidateSessionAccess method
func (m *MockMediaSessionRepository) ValidateSessionAccess(ctx context.Context, sessionID, userID string) (bool, error) {
	args := m.Called(ctx, sessionID, userID)
	return args.Bool(0), args.Error(1)
}

// ===== Session Analytics and Monitoring =====

// GetActiveSessionsCount mocks the GetActiveSessionsCount method
func (m *MockMediaSessionRepository) GetActiveSessionsCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// ===== Session Cleanup =====

// CleanupExpiredSessions mocks the CleanupExpiredSessions method
func (m *MockMediaSessionRepository) CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error {
	args := m.Called(ctx, maxAge)
	return args.Error(0)
}

// ===== Session TTL Configuration =====

// SetSessionTTL mocks the SetSessionTTL method
func (m *MockMediaSessionRepository) SetSessionTTL(ttl time.Duration) {
	m.Called(ttl)
}

// Ensure MockMediaSessionRepository implements interfaces.MediaSessionRepository
var _ interfaces.MediaSessionRepository = (*MockMediaSessionRepository)(nil)
