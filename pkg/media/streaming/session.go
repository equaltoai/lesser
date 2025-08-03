package streaming

import (
	"context"
	"fmt"
	"time"

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
	repo        MediaSessionRepository
	logger      *zap.Logger
	costTracker CostTracker
	sessionTTL  time.Duration // TTL for sessions (default: 24 hours)
}

// NewSessionManager creates a new session manager
func NewSessionManager(repo MediaSessionRepository, logger *zap.Logger, costTracker CostTracker) *SessionManager {
	return &SessionManager{
		repo:        repo,
		logger:      logger,
		costTracker: costTracker,
		sessionTTL:  24 * time.Hour, // Default TTL for streaming sessions
	}
}

// SetSessionTTL configures the TTL for streaming sessions
// Common values:
// - 6 hours for media streaming sessions
// - 24 hours for analytics data
// - 7 days for historical metrics
//
// Example usage:
//   sm.SetSessionTTL(6 * time.Hour)  // Short-lived media sessions
//   sm.SetSessionTTL(24 * time.Hour) // Analytics data (default)
//   sm.SetSessionTTL(7 * 24 * time.Hour) // Historical metrics
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
		return fmt.Errorf("create session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoWrite(1)
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
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoRead(1)
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
		return fmt.Errorf("update session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoWrite(1)
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
		return fmt.Errorf("end session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoWrite(1)
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
		return nil, fmt.Errorf("query user sessions: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoRead(len(sessions))
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
		return nil, fmt.Errorf("scan media sessions: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoRead(len(sessions))
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
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}

	return nil
}
