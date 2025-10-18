package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// MediaSessionRepository implements session management using DynamORM with BaseRepository
type MediaSessionRepository struct {
	*EnhancedBaseRepository[*models.MediaSession]
	qualityChangeRepo *EnhancedBaseRepository[*models.QualityChange]
	sessionTTL        time.Duration
}

// NewMediaSessionRepository creates a new MediaSessionRepository
func NewMediaSessionRepository(db core.DB, logger *zap.Logger, _ interface{}) *MediaSessionRepository {
	cfg := config.Get()
	tableName := cfg.DynamoTableName
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}

	// Create cost service from legacy cost tracker if provided
	var costService *cost.TrackingService
	// Note: For now, pass nil cost service; can be enhanced later with proper cost tracking

	return &MediaSessionRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.MediaSession](db, tableName, logger, costService, "MediaSessionRepository", "media_session"),
		qualityChangeRepo:      NewEnhancedBaseRepository[*models.QualityChange](db, tableName, logger, costService, "QualityChangeRepository", "quality_change"),
		sessionTTL:             24 * time.Hour, // Default TTL for streaming sessions
	}
}

// NewMediaSessionRepositoryWithCostTracking creates a new MediaSessionRepository with cost tracking
func NewMediaSessionRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MediaSessionRepository {
	return &MediaSessionRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.MediaSession](db, tableName, logger, costService, "MediaSessionRepository", "media_session"),
		qualityChangeRepo:      NewEnhancedBaseRepository[*models.QualityChange](db, tableName, logger, costService, "QualityChangeRepository", "quality_change"),
		sessionTTL:             24 * time.Hour, // Default TTL for streaming sessions
	}
}

// SetSessionTTL configures the TTL for streaming sessions
func (r *MediaSessionRepository) SetSessionTTL(ttl time.Duration) {
	r.sessionTTL = ttl
}

// ====================================================================================
// STREAMING SESSION BUSINESS LOGIC - PRESERVED FROM LEGACY
// ====================================================================================

// StartStreamingSession creates and initializes a new streaming session with validation
func (r *MediaSessionRepository) StartStreamingSession(ctx context.Context, userID, mediaID string, format types.MediaFormat, quality types.Quality) (*types.StreamingSession, error) {
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntitySession, "invalid userID")
	}
	if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntitySession, "invalid mediaID")
	}

	// Generate unique session ID
	sessionID := fmt.Sprintf("%s_%s_%d", userID, mediaID, time.Now().UnixNano())

	// Initialize streaming session with proper defaults
	session := &types.StreamingSession{
		SessionID:        sessionID,
		UserID:           userID,
		MediaID:          mediaID,
		Format:           format,
		CurrentQuality:   quality,
		StartTime:        time.Now(),
		LastSegmentIndex: 0,
		BytesTransferred: 0,
		BufferHealth:     1.0, // Start with full buffer health
		LastActivityTime: time.Now(),
		DurationWatched:  0,
	}

	// Convert to model and create
	model := &models.MediaSession{
		SessionID:        session.SessionID,
		UserID:           session.UserID,
		MediaID:          session.MediaID,
		Format:           string(session.Format),
		CurrentQuality:   string(session.CurrentQuality),
		StartTime:        session.StartTime,
		LastSegmentIndex: session.LastSegmentIndex,
		BytesTransferred: session.BytesTransferred,
		BufferHealth:     session.BufferHealth,
		Active:           true,
	}

	// Set keys and TTL
	if err := model.UpdateKeys(); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntitySession, "update keys")
	}
	model.SetTTL(r.sessionTTL)

	// Create using BaseRepository
	if err := r.ValidateAndCreate(ctx, model); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntitySession, session.SessionID)
	}

	return session, nil
}

// UpdateStreamingMetrics updates session metrics with bandwidth optimization
func (r *MediaSessionRepository) UpdateStreamingMetrics(ctx context.Context, sessionID string, segmentIndex int, bytesTransferred int64, bufferHealth float64, currentQuality types.Quality) error {
	if err := common.ValidateRequiredParam("sessionID", sessionID); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySession, "invalid sessionID")
	}

	// Get existing session
	var existingModel models.MediaSession
	err := r.Get(ctx, fmt.Sprintf("SESSION#%s", sessionID), "METADATA", &existingModel)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	// Validate session is still active
	if !existingModel.Active {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntitySession, sessionID)
	}

	// Update streaming metrics
	now := time.Now()
	existingModel.CurrentQuality = string(currentQuality)
	existingModel.LastSegmentIndex = segmentIndex
	existingModel.BytesTransferred = bytesTransferred
	existingModel.BufferHealth = bufferHealth
	existingModel.LastUpdate = &now

	// Update using BaseRepository
	if err := r.ValidateAndUpdate(ctx, &existingModel); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySession, sessionID)
	}

	// Track quality change for analytics if quality changed
	if existingModel.CurrentQuality != string(currentQuality) {
		session := &types.StreamingSession{
			SessionID:      sessionID,
			CurrentQuality: currentQuality,
		}
		r.trackQualityChange(ctx, session)
	}

	return nil
}

// EndStreamingSession marks a session as ended and calculates final metrics
func (r *MediaSessionRepository) EndStreamingSession(ctx context.Context, sessionID string) error {
	if err := common.ValidateRequiredParam("sessionID", sessionID); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySession, "invalid sessionID")
	}

	// Get existing session
	var existingModel models.MediaSession
	err := r.Get(ctx, fmt.Sprintf("SESSION#%s", sessionID), "METADATA", &existingModel)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	// Calculate final session metrics
	now := time.Now()
	duration := now.Sub(existingModel.StartTime).Seconds()

	// Update session as ended
	existingModel.Active = false
	existingModel.EndTime = &now
	existingModel.Duration = duration
	existingModel.LastUpdate = &now

	// Update using BaseRepository
	if err := r.ValidateAndUpdate(ctx, &existingModel); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySession, sessionID)
	}

	return nil
}

// GetActiveStreams retrieves all active streaming sessions for monitoring
func (r *MediaSessionRepository) GetActiveStreams(ctx context.Context, limit int) ([]*types.StreamingSession, error) {
	// Query for active sessions using filter
	filters := map[string]interface{}{
		"Active": true,
	}

	// Use BaseRepository's QueryWithFilter - scanning for active sessions
	// Note: This is inefficient for large datasets; consider adding GSI for Active field
	models, err := r.QueryWithFilter(ctx, "", filters, limit)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntitySession, "active sessions")
	}

	// Convert to streaming sessions
	sessions := make([]*types.StreamingSession, 0, len(models))
	for _, model := range models {
		if model != nil {
			sessions = append(sessions, r.modelToStreamingSession(model))
		}
	}

	return sessions, nil
}

// ValidateSessionAccess validates if user has access to the streaming session
func (r *MediaSessionRepository) ValidateSessionAccess(ctx context.Context, sessionID, userID string) (bool, error) {
	if err := common.ValidateRequiredParam("sessionID", sessionID); err != nil {
		return false, ErrorHandler.HandleGetError(err, EntitySession, "invalid sessionID")
	}
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return false, ErrorHandler.HandleGetError(err, EntitySession, "invalid userID")
	}

	// Get session
	var model models.MediaSession
	err := r.Get(ctx, fmt.Sprintf("SESSION#%s", sessionID), "METADATA", &model)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return false, nil // Session doesn't exist
		}
		return false, ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	// Check if user owns the session
	return model.UserID == userID, nil
}

// ====================================================================================
// BASIC CRUD OPERATIONS - MIGRATED TO USE BaseRepository
// ====================================================================================

// CreateSession creates a new streaming session (legacy compatibility)
func (r *MediaSessionRepository) CreateSession(ctx context.Context, session *types.StreamingSession) error {
	model := &models.MediaSession{
		SessionID:        session.SessionID,
		UserID:           session.UserID,
		MediaID:          session.MediaID,
		Format:           string(session.Format),
		CurrentQuality:   string(session.CurrentQuality),
		StartTime:        session.StartTime,
		LastSegmentIndex: session.LastSegmentIndex,
		BytesTransferred: session.BytesTransferred,
		BufferHealth:     session.BufferHealth,
		Active:           true,
	}

	// Set keys and TTL
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntitySession, "model keys")
	}
	model.SetTTL(r.sessionTTL)

	// Use BaseRepository Create
	return r.ValidateAndCreate(ctx, model)
}

// GetSession retrieves a streaming session (legacy compatibility)
func (r *MediaSessionRepository) GetSession(ctx context.Context, sessionID string) (*types.StreamingSession, error) {
	var model models.MediaSession

	err := r.Get(ctx, fmt.Sprintf("SESSION#%s", sessionID), "METADATA", &model)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntitySession, sessionID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	return r.modelToStreamingSession(&model), nil
}

// UpdateSession updates a streaming session (legacy compatibility)
func (r *MediaSessionRepository) UpdateSession(ctx context.Context, session *types.StreamingSession) error {
	// Get existing model first
	var existingModel models.MediaSession
	err := r.Get(ctx, fmt.Sprintf("SESSION#%s", session.SessionID), "METADATA", &existingModel)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntitySession, session.SessionID)
	}

	// Update fields
	now := time.Now()
	existingModel.CurrentQuality = string(session.CurrentQuality)
	existingModel.LastSegmentIndex = session.LastSegmentIndex
	existingModel.BytesTransferred = session.BytesTransferred
	existingModel.BufferHealth = session.BufferHealth
	existingModel.LastUpdate = &now

	// Use BaseRepository Update
	if err := r.ValidateAndUpdate(ctx, &existingModel); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntitySession, session.SessionID)
	}

	// Track quality change
	r.trackQualityChange(ctx, session)
	return nil
}

// EndSession marks a session as ended (legacy compatibility)
func (r *MediaSessionRepository) EndSession(ctx context.Context, sessionID string) error {
	return r.EndStreamingSession(ctx, sessionID)
}

// GetUserSessions retrieves active sessions for a user using GSI
func (r *MediaSessionRepository) GetUserSessions(ctx context.Context, userID string) ([]*types.StreamingSession, error) {
	// Query using GSI1 with user partition key
	models, err := r.QueryGSI(ctx, "gsi1", fmt.Sprintf("USER#%s", userID), 0)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return []*types.StreamingSession{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntitySession, userID)
	}

	// Filter for active sessions and convert
	sessions := make([]*types.StreamingSession, 0)
	for _, model := range models {
		if model != nil && model.Active {
			sessions = append(sessions, r.modelToStreamingSession(model))
		}
	}

	return sessions, nil
}

// GetMediaSessions retrieves sessions for a specific media item
func (r *MediaSessionRepository) GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*types.StreamingSession, error) {
	// Use filter to find sessions by MediaID (requires scan)
	filters := map[string]interface{}{
		"MediaID": mediaID,
		"Active":  true,
	}

	models, err := r.QueryWithFilter(ctx, "", filters, int(limit))
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return []*types.StreamingSession{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, mediaID)
	}

	sessions := make([]*types.StreamingSession, 0, len(models))
	for _, model := range models {
		if model != nil {
			sessions = append(sessions, r.modelToStreamingSession(model))
		}
	}

	return sessions, nil
}

// ====================================================================================
// STREAMING ANALYTICS AND MONITORING
// ====================================================================================

// GetActiveSessionsCount returns the count of active sessions for monitoring
func (r *MediaSessionRepository) GetActiveSessionsCount(ctx context.Context) (int, error) {
	// Use filter to count active sessions
	filters := map[string]interface{}{
		"Active": true,
	}

	activeSessions, err := r.QueryWithFilter(ctx, "", filters, 0)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, EntitySession, "active sessions count")
	}

	return len(activeSessions), nil
}

// GetSessionsByTimeRange retrieves sessions within a specific time range for analytics
func (r *MediaSessionRepository) GetSessionsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int32) ([]*types.StreamingSession, error) {
	// Use filter with time range
	filters := map[string]interface{}{
		"StartTime": map[string]interface{}{
			"op":    ">=",
			"value": startTime.Format(time.RFC3339),
		},
	}

	models, err := r.QueryWithFilter(ctx, "", filters, int(limit))
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return []*types.StreamingSession{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntitySession, "time range")
	}

	// Filter by end time and convert
	sessions := make([]*types.StreamingSession, 0)
	for _, model := range models {
		if model != nil && model.StartTime.Before(endTime) {
			sessions = append(sessions, r.modelToStreamingSession(model))
		}
	}

	return sessions, nil
}

// ====================================================================================
// SESSION CLEANUP AND MAINTENANCE
// ====================================================================================

// CleanupExpiredSessions removes sessions older than the specified duration
func (r *MediaSessionRepository) CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)

	// Find expired sessions
	filters := map[string]interface{}{
		"Active": false,
		"StartTime": map[string]interface{}{
			"op":    "<",
			"value": cutoff.Format(time.RFC3339),
		},
	}

	expiredSessions, err := r.QueryWithFilter(ctx, "", filters, 0)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil // No expired sessions
		}
		return ErrorHandler.HandleQueryError(err, EntitySession, "expired sessions")
	}

	// Delete expired sessions
	deletedCount := 0
	for _, session := range expiredSessions {
		if session != nil {
			err := r.ValidateAndDelete(ctx, session.PK, session.SK)
			if err != nil {
				r.logger.Warn("failed to delete expired session",
					zap.String("sessionID", session.SessionID),
					zap.Error(err))
			} else {
				deletedCount++
			}
		}
	}

	r.logger.Info("cleaned up expired sessions",
		zap.Int("count", deletedCount))

	return nil
}

// ====================================================================================
// STREAMING QUALITY ANALYTICS
// ====================================================================================

// trackQualityChange creates a quality change record for analytics
func (r *MediaSessionRepository) trackQualityChange(ctx context.Context, session *types.StreamingSession) {
	qualityChange := &models.QualityChange{
		SessionID: session.SessionID,
		Quality:   string(session.CurrentQuality),
		Timestamp: time.Now(),
	}

	if err := qualityChange.UpdateKeys(); err != nil {
		r.logger.Warn("failed to update quality change keys",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return
	}

	err := r.qualityChangeRepo.Create(ctx, qualityChange)
	if err != nil {
		r.logger.Warn("failed to track quality change",
			zap.String("sessionID", session.SessionID),
			zap.String("quality", string(session.CurrentQuality)),
			zap.Error(err))
	}
}

// ====================================================================================
// MODEL CONVERSION HELPERS
// ====================================================================================

// modelToStreamingSession converts a DynamORM model to types.StreamingSession
func (r *MediaSessionRepository) modelToStreamingSession(model *models.MediaSession) *types.StreamingSession {
	session := &types.StreamingSession{
		SessionID:        model.SessionID,
		UserID:           model.UserID,
		MediaID:          model.MediaID,
		Format:           types.MediaFormat(model.Format),
		CurrentQuality:   types.Quality(model.CurrentQuality),
		StartTime:        model.StartTime,
		LastSegmentIndex: model.LastSegmentIndex,
		BytesTransferred: model.BytesTransferred,
		BufferHealth:     model.BufferHealth,
	}

	// Set LastActivityTime from model's LastUpdate if available
	if model.LastUpdate != nil {
		session.LastActivityTime = *model.LastUpdate
	} else {
		session.LastActivityTime = model.StartTime
	}

	// Calculate duration watched based on session state
	if model.EndTime != nil {
		session.DurationWatched = int64(model.EndTime.Sub(model.StartTime).Seconds())
	} else if model.Duration > 0 {
		session.DurationWatched = int64(model.Duration)
	}

	// Set error field if session is inactive
	if !model.Active && model.EndTime != nil {
		session.Error = "session_ended"
	}

	return session
}
