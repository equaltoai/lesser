package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"go.uber.org/zap"
)

// MediaSessionRepository interface for session persistence
type MediaSessionRepository interface {
	CreateSession(ctx context.Context, session *StreamingSession) error
	GetSession(ctx context.Context, sessionID string) (*StreamingSession, error)
	UpdateSession(ctx context.Context, session *StreamingSession) error
	EndSession(ctx context.Context, sessionID string) error
	GetUserSessions(ctx context.Context, userID string) ([]*StreamingSession, error)
	GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*StreamingSession, error)
	CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error
	SetSessionTTL(ttl time.Duration)
}

// SessionManager manages streaming sessions
// Uses DynamoDB TTL for automatic session expiration in serverless environments
type SessionManager struct {
	repo           MediaSessionRepository
	logger         *zap.Logger
	costTracker    CostTracker
	unifiedTracker *cost.UnifiedTracker
	tableName      string
	sessionTTL     time.Duration // TTL for sessions (default: 24 hours)
}

// NewSessionManager creates a new session manager
func NewSessionManager(repo MediaSessionRepository, logger *zap.Logger, costTracker CostTracker) *SessionManager {
	cfg := config.Get()
	tableName := cfg.DynamoTableName
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}

	// Create unified tracker for centralized cost tracking
	unifiedTracker := cost.NewRepositoryTracker(nil, logger, "SessionManager", "", "")

	return &SessionManager{
		repo:           repo,
		logger:         logger,
		costTracker:    costTracker,
		unifiedTracker: unifiedTracker,
		tableName:      tableName,
		sessionTTL:     24 * time.Hour, // Default TTL for streaming sessions
	}
}

// SetSessionTTL configures the TTL for streaming sessions
// Common values:
// - 6 hours for media streaming sessions
// - 24 hours for analytics data
// - 7 days for historical metrics
//
// Example usage:
//
//	sm.SetSessionTTL(6 * time.Hour)  // Short-lived media sessions
//	sm.SetSessionTTL(24 * time.Hour) // Analytics data (default)
//	sm.SetSessionTTL(7 * 24 * time.Hour) // Historical metrics
func (sm *SessionManager) SetSessionTTL(ttl time.Duration) {
	sm.sessionTTL = ttl
	sm.repo.SetSessionTTL(ttl)
}

// CreateSession creates a new streaming session
func (sm *SessionManager) CreateSession(ctx context.Context, session *StreamingSession) error {
	err := sm.repo.CreateSession(ctx, session)
	if err != nil {
		sm.logger.Error("failed to create session",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrCreateSession, err)
	}

	// Track cost using centralized tracker
	if err := sm.unifiedTracker.TrackDynamoWrite(ctx, sm.tableName, 1); err != nil {
		sm.logger.Warn("failed to track cost", zap.Error(err))
	}

	return nil
}

// GetSession retrieves a streaming session
func (sm *SessionManager) GetSession(ctx context.Context, sessionID string) (*StreamingSession, error) {
	session, err := sm.repo.GetSession(ctx, sessionID)
	if err != nil {
		sm.logger.Error("failed to get session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrGetSession, err)
	}

	// Track cost using centralized tracker
	if err := sm.unifiedTracker.TrackDynamoRead(ctx, sm.tableName, 1); err != nil {
		sm.logger.Warn("failed to track cost", zap.Error(err))
	}

	return session, nil
}

// UpdateSession updates a streaming session
func (sm *SessionManager) UpdateSession(ctx context.Context, session *StreamingSession) error {
	err := sm.repo.UpdateSession(ctx, session)
	if err != nil {
		sm.logger.Error("failed to update session",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrUpdateSession, err)
	}

	// Track cost using centralized tracker
	if err := sm.unifiedTracker.TrackDynamoWrite(ctx, sm.tableName, 1); err != nil {
		sm.logger.Warn("failed to track cost", zap.Error(err))
	}

	return nil
}

// EndSession marks a session as ended
func (sm *SessionManager) EndSession(ctx context.Context, sessionID string) error {
	err := sm.repo.EndSession(ctx, sessionID)
	if err != nil {
		sm.logger.Error("failed to end session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrEndSession, err)
	}

	// Track cost using centralized tracker
	if err := sm.unifiedTracker.TrackDynamoWrite(ctx, sm.tableName, 1); err != nil {
		sm.logger.Warn("failed to track cost", zap.Error(err))
	}

	return nil
}

// GetUserSessions retrieves active sessions for a user
func (sm *SessionManager) GetUserSessions(ctx context.Context, userID string) ([]*StreamingSession, error) {
	sessions, err := sm.repo.GetUserSessions(ctx, userID)
	if err != nil {
		sm.logger.Error("failed to query user sessions",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrQueryUserSessions, err)
	}

	// Track cost
	if sm.costTracker != nil {
		// Track cost using centralized tracker
		if err := sm.unifiedTracker.TrackDynamoRead(ctx, sm.tableName, int64(len(sessions))); err != nil {
			sm.logger.Warn("failed to track cost", zap.Error(err))
		}
	}

	return sessions, nil
}

// GetMediaSessions retrieves sessions for a specific media item
func (sm *SessionManager) GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*StreamingSession, error) {
	sessions, err := sm.repo.GetMediaSessions(ctx, mediaID, limit)
	if err != nil {
		sm.logger.Error("failed to scan media sessions",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrScanMediaSessions, err)
	}

	// Track cost
	if sm.costTracker != nil {
		// Track cost using centralized tracker
		if err := sm.unifiedTracker.TrackDynamoRead(ctx, sm.tableName, int64(len(sessions))); err != nil {
			sm.logger.Warn("failed to track cost", zap.Error(err))
		}
	}

	return sessions, nil
}

// CleanupExpiredSessions removes sessions older than the specified duration
// NOTE: This method is optional in serverless environments. DynamoDB TTL will
// automatically remove expired sessions. This method can be used for immediate
// cleanup if needed, but incurs additional DynamoDB scan costs.
func (sm *SessionManager) CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error {
	err := sm.repo.CleanupExpiredSessions(ctx, maxAge)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCleanupExpiredSessions, err)
	}

	return nil
}
