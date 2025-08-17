package repositories

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// CostTracker interface for tracking DynamoDB operation costs
type CostTracker interface {
	TrackDynamoRead(units int)
	TrackDynamoWrite(units int)
}

// MediaSessionRepository implements session management using DynamORM
type MediaSessionRepository struct {
	db               core.DB
	logger           *zap.Logger
	costTracker      CostTracker
	unifiedTracker   *cost.UnifiedTracker
	tableName        string
	sessionTTL       time.Duration
}

// NewMediaSessionRepository creates a new MediaSessionRepository
func NewMediaSessionRepository(db core.DB, logger *zap.Logger, costTracker CostTracker) *MediaSessionRepository {
	tableName := os.Getenv("DYNAMO_TABLE_NAME")
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}
	
	// Create unified tracker for centralized cost tracking
	unifiedTracker := cost.NewRepositoryTracker(nil, logger, "MediaSessionRepository", "", "")
	
	return &MediaSessionRepository{
		db:               db,
		logger:           logger,
		costTracker:      costTracker,
		unifiedTracker:   unifiedTracker,
		tableName:        tableName,
		sessionTTL:       24 * time.Hour, // Default TTL for streaming sessions
	}
}

// SetSessionTTL configures the TTL for streaming sessions
func (r *MediaSessionRepository) SetSessionTTL(ttl time.Duration) {
	r.sessionTTL = ttl
}

// CreateSession creates a new streaming session
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
	model.UpdateKeys()
	model.SetTTL(r.sessionTTL)

	// Create the session
	err := r.db.WithContext(ctx).Model(model).Create()

	if err != nil {
		r.logger.Error("failed to create session",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return fmt.Errorf("create session: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoWrite(ctx, r.tableName, 1); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	return nil
}

// GetSession retrieves a streaming session
func (r *MediaSessionRepository) GetSession(ctx context.Context, sessionID string) (*types.StreamingSession, error) {
	var model models.MediaSession

	err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("PK", "=", fmt.Sprintf("SESSION#%s", sessionID)).
		Where("SK", "=", "METADATA").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		r.logger.Error("failed to get session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoRead(ctx, r.tableName, 1); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	return r.modelToStreamingSession(&model), nil
}

// UpdateSession updates a streaming session
func (r *MediaSessionRepository) UpdateSession(ctx context.Context, session *types.StreamingSession) error {
	now := time.Now()

	// Get the existing session first
	var existingModel models.MediaSession
	err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("PK", "=", fmt.Sprintf("SESSION#%s", session.SessionID)).
		Where("SK", "=", "METADATA").
		First(&existingModel)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("session not found: %s", session.SessionID)
		}
		return fmt.Errorf("failed to get existing session: %w", err)
	}

	// Update the fields
	existingModel.CurrentQuality = string(session.CurrentQuality)
	existingModel.LastSegmentIndex = session.LastSegmentIndex
	existingModel.BytesTransferred = session.BytesTransferred
	existingModel.BufferHealth = session.BufferHealth
	existingModel.LastUpdate = &now

	// Update the session
	err = r.db.WithContext(ctx).Model(&existingModel).Update()

	if err != nil {
		r.logger.Error("failed to update session",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return fmt.Errorf("update session: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoWrite(ctx, r.tableName, 1); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	// Track quality change for analytics
	r.trackQualityChange(ctx, session)

	return nil
}

// EndSession marks a session as ended
func (r *MediaSessionRepository) EndSession(ctx context.Context, sessionID string) error {
	now := time.Now()

	// Get the existing session first
	var existingModel models.MediaSession
	err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("PK", "=", fmt.Sprintf("SESSION#%s", sessionID)).
		Where("SK", "=", "METADATA").
		First(&existingModel)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("session not found: %s", sessionID)
		}
		return fmt.Errorf("failed to get existing session: %w", err)
	}

	// Calculate duration
	duration := now.Sub(existingModel.StartTime).Seconds()

	// Update the fields
	existingModel.Active = false
	existingModel.EndTime = &now
	existingModel.Duration = duration

	// Update session as ended
	err = r.db.WithContext(ctx).Model(&existingModel).Update()

	if err != nil {
		r.logger.Error("failed to end session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return fmt.Errorf("end session: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoWrite(ctx, r.tableName, 1); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	return nil
}

// GetUserSessions retrieves active sessions for a user
func (r *MediaSessionRepository) GetUserSessions(ctx context.Context, userID string) ([]*types.StreamingSession, error) {
	var sessionModels []models.MediaSession

	err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("Active", "=", true).
		Index("gsi1").
		All(&sessionModels)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*types.StreamingSession{}, nil
		}
		r.logger.Error("failed to query user sessions",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, fmt.Errorf("query user sessions: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoRead(ctx, r.tableName, int64(len(sessionModels))); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	sessions := make([]*types.StreamingSession, 0, len(sessionModels))
	for _, model := range sessionModels {
		sessions = append(sessions, r.modelToStreamingSession(&model))
	}

	return sessions, nil
}

// GetMediaSessions retrieves sessions for a specific media item
func (r *MediaSessionRepository) GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*types.StreamingSession, error) {
	var sessionModels []models.MediaSession

	query := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("MediaID", "=", mediaID).
		Where("Active", "=", true)

	if limit > 0 {
		query = query.Limit(int(limit))
	}

	// Note: This requires a scan since we don't have a GSI on MediaID
	// In production, consider adding GSI2 with MediaID as PK
	err := query.All(&sessionModels)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*types.StreamingSession{}, nil
		}
		r.logger.Error("failed to scan media sessions",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("scan media sessions: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoRead(ctx, r.tableName, int64(len(sessionModels))); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	sessions := make([]*types.StreamingSession, 0, len(sessionModels))
	for _, model := range sessionModels {
		sessions = append(sessions, r.modelToStreamingSession(&model))
	}

	return sessions, nil
}

// CleanupExpiredSessions removes sessions older than the specified duration
func (r *MediaSessionRepository) CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	var expiredSessions []models.MediaSession

	// Scan for expired sessions
	err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("Active", "=", false).
		Where("StartTime", "<", cutoff.Format(time.RFC3339)).
		All(&expiredSessions)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No expired sessions
		}
		return fmt.Errorf("scan expired sessions: %w", err)
	}

	// Delete expired sessions
	for _, session := range expiredSessions {
		err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
			Where("PK", "=", session.PK).
			Where("SK", "=", session.SK).
			Delete()

		if err != nil {
			r.logger.Warn("failed to delete expired session",
				zap.String("sessionID", session.SessionID),
				zap.Error(err))
		}
	}

	r.logger.Info("cleaned up expired sessions",
		zap.Int("count", len(expiredSessions)))

	return nil
}

// trackQualityChange creates a quality change record for analytics
func (r *MediaSessionRepository) trackQualityChange(ctx context.Context, session *types.StreamingSession) {
	qualityChange := &models.QualityChange{
		SessionID: session.SessionID,
		Quality:   string(session.CurrentQuality),
		Timestamp: time.Now(),
	}
	qualityChange.UpdateKeys()

	err := r.db.WithContext(ctx).Model(qualityChange).Create()
	if err != nil {
		r.logger.Warn("failed to track quality change",
			zap.String("sessionID", session.SessionID),
			zap.String("quality", string(session.CurrentQuality)),
			zap.Error(err))
	}
}

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

// GetActiveSessionsCount returns the count of active sessions for monitoring
func (r *MediaSessionRepository) GetActiveSessionsCount(ctx context.Context) (int, error) {
	var activeSessions []models.MediaSession

	err := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("Active", "=", true).
		All(&activeSessions)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to count active sessions: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoRead(ctx, r.tableName, int64(len(activeSessions))); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	return len(activeSessions), nil
}

// GetSessionsByTimeRange retrieves sessions within a specific time range for analytics
func (r *MediaSessionRepository) GetSessionsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int32) ([]*types.StreamingSession, error) {
	var sessionModels []models.MediaSession

	query := r.db.WithContext(ctx).Model(&models.MediaSession{}).
		Where("StartTime", ">=", startTime.Format(time.RFC3339)).
		Where("StartTime", "<=", endTime.Format(time.RFC3339))

	if limit > 0 {
		query = query.Limit(int(limit))
	}

	err := query.All(&sessionModels)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*types.StreamingSession{}, nil
		}
		return nil, fmt.Errorf("failed to query sessions by time range: %w", err)
	}

	// Track cost using centralized tracker
	if err := r.unifiedTracker.TrackDynamoRead(ctx, r.tableName, int64(len(sessionModels))); err != nil {
		r.logger.Warn("failed to track cost", zap.Error(err))
	}

	sessions := make([]*types.StreamingSession, 0, len(sessionModels))
	for _, model := range sessionModels {
		sessions = append(sessions, r.modelToStreamingSession(&model))
	}

	return sessions, nil
}
