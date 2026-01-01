// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/types"
)

// MediaSessionRepository defines the interface for media streaming session operations.
// This handles session management for streaming media.
type MediaSessionRepository interface {
	// Session lifecycle operations
	StartStreamingSession(ctx context.Context, userID, mediaID string, format types.MediaFormat, quality types.Quality) (*types.StreamingSession, error)
	EndStreamingSession(ctx context.Context, sessionID string) error
	UpdateStreamingMetrics(ctx context.Context, sessionID string, segmentIndex int, bytesTransferred int64, bufferHealth float64, currentQuality types.Quality) error

	// Legacy session operations
	CreateSession(ctx context.Context, session *types.StreamingSession) error
	GetSession(ctx context.Context, sessionID string) (*types.StreamingSession, error)
	UpdateSession(ctx context.Context, session *types.StreamingSession) error
	EndSession(ctx context.Context, sessionID string) error

	// Session queries
	GetActiveStreams(ctx context.Context, limit int) ([]*types.StreamingSession, error)
	GetUserSessions(ctx context.Context, userID string) ([]*types.StreamingSession, error)
	GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*types.StreamingSession, error)
	GetSessionsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int32) ([]*types.StreamingSession, error)

	// Session validation and access
	ValidateSessionAccess(ctx context.Context, sessionID, userID string) (bool, error)

	// Session analytics and monitoring
	GetActiveSessionsCount(ctx context.Context) (int, error)

	// Session cleanup
	CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error

	// Session TTL configuration
	SetSessionTTL(ttl time.Duration)
}
