// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/types"
)

// MediaSessionRepository is a thread-safe in-memory implementation of interfaces.MediaSessionRepository.
type MediaSessionRepository struct {
	mu sync.RWMutex

	// Sessions by ID: sessionID -> StreamingSession
	sessions map[string]*types.StreamingSession

	// Sessions by user: userID -> []sessionID
	sessionsByUser map[string][]string

	// Sessions by media: mediaID -> []sessionID
	sessionsByMedia map[string][]string

	// Session TTL
	sessionTTL time.Duration
}

// NewMediaSessionRepository creates a new in-memory media session repository
func NewMediaSessionRepository() *MediaSessionRepository {
	return &MediaSessionRepository{
		sessions:        make(map[string]*types.StreamingSession),
		sessionsByUser:  make(map[string][]string),
		sessionsByMedia: make(map[string][]string),
		sessionTTL:      24 * time.Hour,
	}
}

// ===== Session Lifecycle Operations =====

// StartStreamingSession creates and initializes a new streaming session
func (r *MediaSessionRepository) StartStreamingSession(_ context.Context, userID, mediaID string, format types.MediaFormat, quality types.Quality) (*types.StreamingSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessionID := fmt.Sprintf("%s_%s_%d", userID, mediaID, time.Now().UnixNano())
	now := time.Now()

	session := &types.StreamingSession{
		SessionID:        sessionID,
		UserID:           userID,
		MediaID:          mediaID,
		Format:           format,
		CurrentQuality:   quality,
		StartTime:        now,
		LastSegmentIndex: 0,
		BytesTransferred: 0,
		BufferHealth:     1.0,
		LastActivityTime: now,
		DurationWatched:  0,
	}

	r.sessions[sessionID] = session
	r.sessionsByUser[userID] = append(r.sessionsByUser[userID], sessionID)
	r.sessionsByMedia[mediaID] = append(r.sessionsByMedia[mediaID], sessionID)

	return session, nil
}

// EndStreamingSession marks a session as ended
func (r *MediaSessionRepository) EndStreamingSession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return storage.ErrNotFound
	}

	session.DurationWatched = int64(time.Since(session.StartTime).Seconds())
	session.Error = "session_ended"

	return nil
}

// UpdateStreamingMetrics updates session metrics
func (r *MediaSessionRepository) UpdateStreamingMetrics(_ context.Context, sessionID string, segmentIndex int, bytesTransferred int64, bufferHealth float64, currentQuality types.Quality) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return storage.ErrNotFound
	}

	session.LastSegmentIndex = segmentIndex
	session.BytesTransferred = bytesTransferred
	session.BufferHealth = bufferHealth
	session.CurrentQuality = currentQuality
	session.LastActivityTime = time.Now()

	return nil
}

// ===== Legacy Session Operations =====

// CreateSession creates a new streaming session (legacy compatibility)
func (r *MediaSessionRepository) CreateSession(_ context.Context, session *types.StreamingSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[session.SessionID]; exists {
		return nil // Already exists
	}

	r.sessions[session.SessionID] = session
	r.sessionsByUser[session.UserID] = append(r.sessionsByUser[session.UserID], session.SessionID)
	r.sessionsByMedia[session.MediaID] = append(r.sessionsByMedia[session.MediaID], session.SessionID)

	return nil
}

// GetSession retrieves a streaming session
func (r *MediaSessionRepository) GetSession(_ context.Context, sessionID string) (*types.StreamingSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return session, nil
}

// UpdateSession updates a streaming session
func (r *MediaSessionRepository) UpdateSession(_ context.Context, session *types.StreamingSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[session.SessionID]; !exists {
		return storage.ErrNotFound
	}

	r.sessions[session.SessionID] = session
	return nil
}

// EndSession marks a session as ended (legacy compatibility)
func (r *MediaSessionRepository) EndSession(ctx context.Context, sessionID string) error {
	return r.EndStreamingSession(ctx, sessionID)
}

// ===== Session Queries =====

// GetActiveStreams retrieves all active streaming sessions
func (r *MediaSessionRepository) GetActiveStreams(_ context.Context, limit int) ([]*types.StreamingSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var activeSessions []*types.StreamingSession
	for _, session := range r.sessions {
		if session.Error == "" { // Active if no error
			activeSessions = append(activeSessions, session)
			if limit > 0 && len(activeSessions) >= limit {
				break
			}
		}
	}
	return activeSessions, nil
}

// GetUserSessions retrieves active sessions for a user
func (r *MediaSessionRepository) GetUserSessions(_ context.Context, userID string) ([]*types.StreamingSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*types.StreamingSession
	for _, sessionID := range r.sessionsByUser[userID] {
		if session, exists := r.sessions[sessionID]; exists && session.Error == "" {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

// GetMediaSessions retrieves sessions for a specific media item
func (r *MediaSessionRepository) GetMediaSessions(_ context.Context, mediaID string, limit int32) ([]*types.StreamingSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*types.StreamingSession
	for _, sessionID := range r.sessionsByMedia[mediaID] {
		if session, exists := r.sessions[sessionID]; exists && session.Error == "" {
			sessions = append(sessions, session)
			if limit > 0 && len(sessions) >= int(limit) {
				break
			}
		}
	}
	return sessions, nil
}

// GetSessionsByTimeRange retrieves sessions within a specific time range
func (r *MediaSessionRepository) GetSessionsByTimeRange(_ context.Context, startTime, endTime time.Time, limit int32) ([]*types.StreamingSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*types.StreamingSession
	for _, session := range r.sessions {
		if !session.StartTime.Before(startTime) && session.StartTime.Before(endTime) {
			sessions = append(sessions, session)
			if limit > 0 && len(sessions) >= int(limit) {
				break
			}
		}
	}
	return sessions, nil
}

// ===== Session Validation and Access =====

// ValidateSessionAccess validates if user has access to the streaming session
func (r *MediaSessionRepository) ValidateSessionAccess(_ context.Context, sessionID, userID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return false, nil
	}
	return session.UserID == userID, nil
}

// ===== Session Analytics and Monitoring =====

// GetActiveSessionsCount returns the count of active sessions
func (r *MediaSessionRepository) GetActiveSessionsCount(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, session := range r.sessions {
		if session.Error == "" {
			count++
		}
	}
	return count, nil
}

// ===== Session Cleanup =====

// CleanupExpiredSessions removes sessions older than the specified duration
func (r *MediaSessionRepository) CleanupExpiredSessions(_ context.Context, maxAge time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for sessionID, session := range r.sessions {
		if session.StartTime.Before(cutoff) {
			delete(r.sessions, sessionID)
		}
	}

	// Rebuild indexes
	r.sessionsByUser = make(map[string][]string)
	r.sessionsByMedia = make(map[string][]string)

	for sessionID, session := range r.sessions {
		r.sessionsByUser[session.UserID] = append(r.sessionsByUser[session.UserID], sessionID)
		r.sessionsByMedia[session.MediaID] = append(r.sessionsByMedia[session.MediaID], sessionID)
	}

	return nil
}

// ===== Session TTL Configuration =====

// SetSessionTTL configures the TTL for streaming sessions
func (r *MediaSessionRepository) SetSessionTTL(ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionTTL = ttl
}

// Clear clears all data (test helper)
func (r *MediaSessionRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions = make(map[string]*types.StreamingSession)
	r.sessionsByUser = make(map[string][]string)
	r.sessionsByMedia = make(map[string][]string)
}

// Ensure MediaSessionRepository implements interfaces.MediaSessionRepository
var _ interfaces.MediaSessionRepository = (*MediaSessionRepository)(nil)
