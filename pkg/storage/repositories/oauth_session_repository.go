package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// OAuthSessionRepository handles OAuth authorization session operations using BaseRepository
type OAuthSessionRepository struct {
	*EnhancedBaseRepository[*models.OAuthAuthSession]
}

// NewOAuthSessionRepository creates a new OAuth session repository with enhanced functionality
func NewOAuthSessionRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *OAuthSessionRepository {
	// Create enhanced repository optimized for OAuth session operations
	enhancedRepo := NewEnhancedBaseRepository[*models.OAuthAuthSession](db, tableName, logger, costService, "OAuthSessionRepository", "oauth_session")

	// Set up enhanced services for OAuth session operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // OAuth sessions cached
	enhancedRepo.SetEventService(NewDefaultEventService())      // OAuth events

	return &OAuthSessionRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateOAuthSession creates a new OAuth authorization session
func (r *OAuthSessionRepository) CreateOAuthSession(ctx context.Context, session *models.OAuthAuthSession) error {
	if session == nil {
		return ErrorHandler.HandleCreateError(errors.New("session cannot be nil"), EntitySession, "nil")
	}

	err := r.ValidateAndCreate(ctx, session)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntitySession, session.SessionID)
	}

	return nil
}

// GetOAuthSession retrieves an OAuth session by session ID
func (r *OAuthSessionRepository) GetOAuthSession(ctx context.Context, sessionID string) (*models.OAuthAuthSession, error) {
	var session models.OAuthAuthSession
	pk := fmt.Sprintf("OAUTH_AUTH#%s", sessionID)
	sk := fmt.Sprintf("SESSION#%s", sessionID)

	err := r.Get(ctx, pk, sk, &session)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrorHandler.HandleGetError(errors.New("OAuth session not found"), EntitySession, sessionID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	// Check if session is expired
	if session.IsExpired() {
		r.logger.Debug("OAuth session expired",
			zap.String("sessionID", sessionID),
			zap.Time("expiresAt", time.Unix(session.ExpiresAt, 0)))
		return nil, ErrorHandler.HandleGetError(errors.New("OAuth session expired"), EntitySession, sessionID)
	}

	return &session, nil
}

// GetOAuthSessionByState retrieves an OAuth session by OAuth state parameter
func (r *OAuthSessionRepository) GetOAuthSessionByState(ctx context.Context, state string) (*models.OAuthAuthSession, error) {
	var sessions []models.OAuthAuthSession

	err := r.GetDB().WithContext(ctx).Model(&models.OAuthAuthSession{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("OAUTH_STATE#%s", state)).
		All(&sessions)

	if err != nil || len(sessions) == 0 {
		return nil, ErrorHandler.HandleGetError(errors.New("OAuth session not found for state"), EntityOAuthState, state)
	}

	session := &sessions[0]

	// Check if session is expired
	if session.IsExpired() {
		return nil, ErrorHandler.HandleGetError(errors.New("OAuth session expired"), EntityOAuthState, state)
	}

	return session, nil
}

// UpdateOAuthSession updates an existing OAuth session
func (r *OAuthSessionRepository) UpdateOAuthSession(ctx context.Context, session *models.OAuthAuthSession) error {
	if session == nil {
		return ErrorHandler.HandleUpdateError(errors.New("session cannot be nil"), EntitySession, "nil")
	}

	// Touch the session to update last used time
	session.Touch()

	err := r.ValidateAndUpdate(ctx, session)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySession, session.SessionID)
	}

	return nil
}

// DeleteOAuthSession deletes an OAuth session
func (r *OAuthSessionRepository) DeleteOAuthSession(ctx context.Context, sessionID string) error {
	pk := fmt.Sprintf("OAUTH_AUTH#%s", sessionID)
	sk := fmt.Sprintf("SESSION#%s", sessionID)

	err := r.ValidateAndDelete(ctx, pk, sk)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil // Already deleted is not an error
		}
		return ErrorHandler.HandleDeleteError(err, EntitySession, sessionID)
	}

	return nil
}

// GetUserOAuthSessions retrieves all OAuth sessions for a user
func (r *OAuthSessionRepository) GetUserOAuthSessions(ctx context.Context, username string, limit int) ([]*models.OAuthAuthSession, error) {
	var sessions []models.OAuthAuthSession

	query := r.GetDB().WithContext(ctx).Model(&models.OAuthAuthSession{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("USER_OAUTH#%s", username))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&sessions)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntitySession, "user sessions")
	}

	// Filter out expired sessions
	var validSessions []*models.OAuthAuthSession
	for i := range sessions {
		if sessions[i].IsValid() {
			validSessions = append(validSessions, &sessions[i])
		}
	}

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
		err := errors.New("session cannot be authorized in current state: " + session.FlowStep)
		return ErrorHandler.HandleUpdateError(err, EntitySession, sessionID)
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
func (r *OAuthSessionRepository) CleanupExpiredOAuthSessions(_ context.Context, _ int) (int, error) {
	// DynamoDB TTL will automatically handle cleanup, but we can implement
	// manual cleanup for immediate removal if needed

	// For now, just return 0 since TTL handles cleanup
	// In a production system, you might scan for expired sessions and delete them
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
