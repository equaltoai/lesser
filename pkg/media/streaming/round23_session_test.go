package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type inMemoryMediaSessionRepo struct {
	sessions map[string]*StreamingSession
	ttl      time.Duration

	createErr error
	getErr    error
	updateErr error
	endErr    error
	cleanupErr error
}

func newInMemoryMediaSessionRepo() *inMemoryMediaSessionRepo {
	return &inMemoryMediaSessionRepo{sessions: make(map[string]*StreamingSession)}
}

func (r *inMemoryMediaSessionRepo) SetSessionTTL(ttl time.Duration) { r.ttl = ttl }

func (r *inMemoryMediaSessionRepo) CreateSession(_ context.Context, session *StreamingSession) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.sessions[session.SessionID] = session
	return nil
}

func (r *inMemoryMediaSessionRepo) GetSession(_ context.Context, sessionID string) (*StreamingSession, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	s, ok := r.sessions[sessionID]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}

func (r *inMemoryMediaSessionRepo) UpdateSession(_ context.Context, session *StreamingSession) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.sessions[session.SessionID] = session
	return nil
}

func (r *inMemoryMediaSessionRepo) EndSession(_ context.Context, sessionID string) error {
	if r.endErr != nil {
		return r.endErr
	}
	delete(r.sessions, sessionID)
	return nil
}

func (r *inMemoryMediaSessionRepo) GetUserSessions(_ context.Context, userID string) ([]*StreamingSession, error) {
	out := make([]*StreamingSession, 0)
	for _, s := range r.sessions {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *inMemoryMediaSessionRepo) GetMediaSessions(_ context.Context, mediaID string, _ int32) ([]*StreamingSession, error) {
	out := make([]*StreamingSession, 0)
	for _, s := range r.sessions {
		if s.MediaID == mediaID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *inMemoryMediaSessionRepo) CleanupExpiredSessions(_ context.Context, _ time.Duration) error {
	return r.cleanupErr
}

func TestSessionManager_HappyPathAndErrors(t *testing.T) {
	repo := newInMemoryMediaSessionRepo()
	sm := NewSessionManager(repo, zap.NewNop(), nil)

	sm.SetSessionTTL(6 * time.Hour)
	assert.Equal(t, 6*time.Hour, repo.ttl)

	session := &StreamingSession{
		SessionID: "s1",
		UserID:    "u1",
		MediaID:   "m1",
		StartTime: time.Now().Add(-time.Minute),
	}

	require.NoError(t, sm.CreateSession(context.Background(), session))
	got, err := sm.GetSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "m1", got.MediaID)

	session.LastSegmentIndex = 10
	require.NoError(t, sm.UpdateSession(context.Background(), session))

	userSessions, err := sm.GetUserSessions(context.Background(), "u1")
	require.NoError(t, err)
	assert.Len(t, userSessions, 1)

	mediaSessions, err := sm.GetMediaSessions(context.Background(), "m1", 10)
	require.NoError(t, err)
	assert.Len(t, mediaSessions, 1)

	require.NoError(t, sm.EndSession(context.Background(), "s1"))

	repo.createErr = errors.New("create boom")
	err = sm.CreateSession(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateSession)

	repo.getErr = errors.New("get boom")
	_, err = sm.GetSession(context.Background(), "s1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGetSession)

	repo.updateErr = errors.New("update boom")
	err = sm.UpdateSession(context.Background(), session)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUpdateSession)

	repo.endErr = errors.New("end boom")
	err = sm.EndSession(context.Background(), "s1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEndSession)

	repo.endErr = nil
	repo.getErr = nil
	repo.updateErr = nil
	repo.createErr = nil

	repo.sessions["s2"] = &StreamingSession{SessionID: "s2", UserID: "u2", MediaID: "m2", StartTime: time.Now()}
	assert.NoError(t, sm.CleanupExpiredSessions(context.Background(), time.Hour))

	repo.cleanupErr = errors.New("cleanup boom")
	err = sm.CleanupExpiredSessions(context.Background(), time.Hour)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCleanupExpiredSessions)

	// Cover cost tracking branches in GetUserSessions/GetMediaSessions.
	smWithCost := NewSessionManager(repo, zap.NewNop(), noopCostTracker{})
	_, _ = smWithCost.GetUserSessions(context.Background(), "u2")
	_, _ = smWithCost.GetMediaSessions(context.Background(), "m2", 10)
}
