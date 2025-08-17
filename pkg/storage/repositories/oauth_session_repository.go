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

// OAuthSessionRepository handles OAuth authorization session operations using DynamORM
type OAuthSessionRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewOAuthSessionRepository creates a new OAuth session repository
func NewOAuthSessionRepository(db core.DB, logger *zap.Logger) *OAuthSessionRepository {
	return &OAuthSessionRepository{
		db:     db,
		logger: logger,
	}
}

// CreateOAuthSession creates a new OAuth authorization session
func (r *OAuthSessionRepository) CreateOAuthSession(ctx context.Context, session *models.OAuthAuthSession) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}

	err := r.db.WithContext(ctx).Model(session).Create()
	if err != nil {
		r.logger.Error("failed to create OAuth session",
			zap.String("sessionID", session.SessionID),
			zap.String("clientID", session.ClientID),
			zap.Error(err))
		return fmt.Errorf("failed to create OAuth session: %w", err)
	}

	r.logger.Debug("OAuth session created successfully",
		zap.String("sessionID", session.SessionID),
		zap.String("clientID", session.ClientID),
		zap.String("flowStep", session.FlowStep))

	return nil
}

// GetOAuthSession retrieves an OAuth session by session ID
func (r *OAuthSessionRepository) GetOAuthSession(ctx context.Context, sessionID string) (*models.OAuthAuthSession, error) {
	var session models.OAuthAuthSession

	err := r.db.WithContext(ctx).Model(&session).
		Where("PK", "=", fmt.Sprintf("OAUTH_AUTH#%s", sessionID)).
		Where("SK", "=", fmt.Sprintf("SESSION#%s", sessionID)).
		First(&session)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("OAuth session not found",
				zap.String("sessionID", sessionID))
			return nil, fmt.Errorf("OAuth session not found")
		}
		r.logger.Error("failed to get OAuth session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get OAuth session: %w", err)
	}

	// Check if session is expired
	if session.IsExpired() {
		r.logger.Debug("OAuth session expired",
			zap.String("sessionID", sessionID),
			zap.Time("expiresAt", time.Unix(session.ExpiresAt, 0)))
		return nil, fmt.Errorf("OAuth session expired")
	}

	return &session, nil
}

// GetOAuthSessionByState retrieves an OAuth session by OAuth state parameter
func (r *OAuthSessionRepository) GetOAuthSessionByState(ctx context.Context, state string) (*models.OAuthAuthSession, error) {
	var session models.OAuthAuthSession

	err := r.db.WithContext(ctx).Model(&session).
		Index("state-index").
		Where("GSI2PK", "=", fmt.Sprintf("OAUTH_STATE#%s", state)).
		First(&session)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("OAuth session not found by state",
				zap.String("state", state))
			return nil, fmt.Errorf("OAuth session not found for state")
		}
		r.logger.Error("failed to get OAuth session by state",
			zap.String("state", state),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get OAuth session by state: %w", err)
	}

	// Check if session is expired
	if session.IsExpired() {
		r.logger.Debug("OAuth session expired",
			zap.String("sessionID", session.SessionID),
			zap.String("state", state),
			zap.Time("expiresAt", time.Unix(session.ExpiresAt, 0)))
		return nil, fmt.Errorf("OAuth session expired")
	}

	return &session, nil
}

// UpdateOAuthSession updates an existing OAuth session
func (r *OAuthSessionRepository) UpdateOAuthSession(ctx context.Context, session *models.OAuthAuthSession) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}

	// Touch the session to update last used time
	session.Touch()

	err := r.db.WithContext(ctx).Model(session).Update()
	if err != nil {
		r.logger.Error("failed to update OAuth session",
			zap.String("sessionID", session.SessionID),
			zap.String("flowStep", session.FlowStep),
			zap.Error(err))
		return fmt.Errorf("failed to update OAuth session: %w", err)
	}

	r.logger.Debug("OAuth session updated successfully",
		zap.String("sessionID", session.SessionID),
		zap.String("flowStep", session.FlowStep))

	return nil
}

// DeleteOAuthSession deletes an OAuth session
func (r *OAuthSessionRepository) DeleteOAuthSession(ctx context.Context, sessionID string) error {
	err := r.db.WithContext(ctx).Model(&models.OAuthAuthSession{}).
		Where("PK", "=", fmt.Sprintf("OAUTH_AUTH#%s", sessionID)).
		Where("SK", "=", fmt.Sprintf("SESSION#%s", sessionID)).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("OAuth session not found for deletion",
				zap.String("sessionID", sessionID))
			return nil // Already deleted is not an error
		}
		r.logger.Error("failed to delete OAuth session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return fmt.Errorf("failed to delete OAuth session: %w", err)
	}

	r.logger.Debug("OAuth session deleted successfully",
		zap.String("sessionID", sessionID))

	return nil
}

// GetUserOAuthSessions retrieves all OAuth sessions for a user
func (r *OAuthSessionRepository) GetUserOAuthSessions(ctx context.Context, username string, limit int) ([]*models.OAuthAuthSession, error) {
	var sessions []models.OAuthAuthSession

	query := r.db.WithContext(ctx).Model(&models.OAuthAuthSession{}).
		Index("user-sessions-index").
		Where("GSI1PK", "=", fmt.Sprintf("USER_OAUTH#%s", username))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&sessions)
	if err != nil {
		r.logger.Error("failed to get user OAuth sessions",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user OAuth sessions: %w", err)
	}

	// Filter out expired sessions
	var validSessions []*models.OAuthAuthSession
	for _, session := range sessions {
		if session.IsValid() {
			validSessions = append(validSessions, &session)
		}
	}

	r.logger.Debug("retrieved user OAuth sessions",
		zap.String("username", username),
		zap.Int("total", len(sessions)),
		zap.Int("valid", len(validSessions)))

	return validSessions, nil
}

// SetOAuthSessionUser associates a user with an OAuth session (after login)
func (r *OAuthSessionRepository) SetOAuthSessionUser(ctx context.Context, sessionID, username string) error {
	// Get the session first
	session, err := r.GetOAuthSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Set the user and update GSI keys
	session.SetUser(username)
	session.SetFlowStep("authenticated", map[string]interface{}{
		"authenticated_at": time.Now(),
	})

	return r.UpdateOAuthSession(ctx, session)
}

// AuthorizeOAuthSession marks an OAuth session as authorized by the user
func (r *OAuthSessionRepository) AuthorizeOAuthSession(ctx context.Context, sessionID string) error {
	// Get the session first
	session, err := r.GetOAuthSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Check if session can be authorized
	if !session.CanAuthorize() {
		return fmt.Errorf("session cannot be authorized in current state: %s", session.FlowStep)
	}

	// Authorize the session
	session.Authorize()

	return r.UpdateOAuthSession(ctx, session)
}

// SetOAuthSessionFlowStep updates the flow step of an OAuth session
func (r *OAuthSessionRepository) SetOAuthSessionFlowStep(ctx context.Context, sessionID, step string, data map[string]interface{}) error {
	// Get the session first
	session, err := r.GetOAuthSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Update flow step
	session.SetFlowStep(step, data)

	return r.UpdateOAuthSession(ctx, session)
}

// CleanupExpiredOAuthSessions removes expired OAuth sessions (to be called by cleanup job)
func (r *OAuthSessionRepository) CleanupExpiredOAuthSessions(_ context.Context, limit int) (int, error) {
	// DynamoDB TTL will automatically handle cleanup, but we can implement
	// manual cleanup for immediate removal if needed
	
	// For now, just return 0 since TTL handles cleanup
	// In a production system, you might scan for expired sessions and delete them
	r.logger.Debug("OAuth session cleanup called",
		zap.Int("limit", limit))
	
	return 0, nil
}

// CountUserOAuthSessions counts active OAuth sessions for a user
func (r *OAuthSessionRepository) CountUserOAuthSessions(ctx context.Context, username string) (int, error) {
	sessions, err := r.GetUserOAuthSessions(ctx, username, 0) // Get all
	if err != nil {
		return 0, err
	}

	return len(sessions), nil
}

// TouchOAuthSession updates the last activity timestamp for a session
func (r *OAuthSessionRepository) TouchOAuthSession(ctx context.Context, sessionID string) error {
	// Get the session first
	session, err := r.GetOAuthSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Touch the session and update
	session.Touch()

	return r.UpdateOAuthSession(ctx, session)
}