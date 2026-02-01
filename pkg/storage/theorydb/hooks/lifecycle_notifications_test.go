package hooks

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type pushCall struct {
	UserID       string
	Notification map[string]any
}

type recordingNotificationRepo struct {
	mu sync.Mutex

	created []map[string]any
	pushed  []pushCall

	createErr        error
	createErrForUser map[string]error
	pushErr          error

	pushWG *sync.WaitGroup
}

func (r *recordingNotificationRepo) CreateNotification(_ context.Context, notification any) error {
	notif, ok := notification.(map[string]any)
	if !ok {
		return errors.New("unexpected notification type")
	}

	r.mu.Lock()
	r.created = append(r.created, notif)
	r.mu.Unlock()

	if r.createErr != nil {
		return r.createErr
	}
	if userID, ok := notif["user_id"].(string); ok && r.createErrForUser != nil {
		if err, ok := r.createErrForUser[userID]; ok {
			return err
		}
	}
	return nil
}

func (r *recordingNotificationRepo) GetUserPushSubscriptions(_ context.Context, _ string) ([]any, error) {
	return nil, nil
}

func (r *recordingNotificationRepo) SendPushNotification(_ context.Context, username string, notification any) error {
	notif, ok := notification.(map[string]any)
	if !ok {
		return errors.New("unexpected notification type")
	}

	r.mu.Lock()
	r.pushed = append(r.pushed, pushCall{UserID: username, Notification: notif})
	r.mu.Unlock()

	if r.pushWG != nil {
		r.pushWG.Done()
	}

	if r.pushErr != nil {
		return r.pushErr
	}
	return nil
}

type followModel struct {
	followerID string
	followeeID string
}

func (f followModel) GetFollowerID() string { return f.followerID }
func (f followModel) GetFolloweeID() string { return f.followeeID }

type mentionModel struct {
	userID          string
	mentionedUserID string
	statusID        string
}

func (m mentionModel) GetUserID() string          { return m.userID }
func (m mentionModel) GetMentionedUserID() string { return m.mentionedUserID }
func (m mentionModel) GetStatusID() string        { return m.statusID }

type reblogModel struct {
	userID           string
	statusID         string
	originalAuthorID string
}

func (r reblogModel) GetUserID() string           { return r.userID }
func (r reblogModel) GetStatusID() string         { return r.statusID }
func (r reblogModel) GetOriginalAuthorID() string { return r.originalAuthorID }

type favoriteModel struct {
	userID         string
	statusID       string
	statusAuthorID string
}

func (f favoriteModel) GetUserID() string         { return f.userID }
func (f favoriteModel) GetStatusID() string       { return f.statusID }
func (f favoriteModel) GetStatusAuthorID() string { return f.statusAuthorID }

type pollModel struct {
	pollID   string
	authorID string
	voterID  string
	ended    bool
}

func (p pollModel) GetPollID() string   { return p.pollID }
func (p pollModel) GetAuthorID() string { return p.authorID }
func (p pollModel) GetVoterID() string  { return p.voterID }
func (p pollModel) HasEnded() bool      { return p.ended }

func TestNotificationHook_WithRepository_CoversAllModelTypes(t *testing.T) {
	logger := zap.NewNop()
	repo := &recordingNotificationRepo{
		pushErr: errors.New("push failed"),
	}
	var wg sync.WaitGroup
	repo.pushWG = &wg

	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, notificationRepositoryKey, repo)

	wg.Add(1)
	require.NoError(t, NotificationHook(ctx, followModel{followerID: "u1", followeeID: "u2"}))

	status := &TestStatus{
		ID:       "s1",
		UserID:   "author",
		Content:  "Hello @u3 @u4",
		Mentions: []string{"u3", "u4"},
	}
	wg.Add(len(status.Mentions))
	require.NoError(t, NotificationHook(ctx, status))

	wg.Add(1)
	require.NoError(t, NotificationHook(ctx, mentionModel{userID: "u1", mentionedUserID: "u2", statusID: "s1"}))

	wg.Add(1)
	require.NoError(t, NotificationHook(ctx, reblogModel{userID: "u1", statusID: "s1", originalAuthorID: "u2"}))

	wg.Add(1)
	require.NoError(t, NotificationHook(ctx, favoriteModel{userID: "u1", statusID: "s1", statusAuthorID: "u2"}))

	// Poll that has not ended should not create notifications.
	require.NoError(t, NotificationHook(ctx, pollModel{pollID: "p1", authorID: "u2", voterID: "u1", ended: false}))

	// Ended poll creates a notification and sends push.
	wg.Add(1)
	require.NoError(t, NotificationHook(ctx, pollModel{pollID: "p2", authorID: "u2", voterID: "u1", ended: true}))

	wg.Wait()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Len(t, repo.created, 7)
	assert.Len(t, repo.pushed, 7)
}

func TestNotificationHook_WithRepository_ReturnsCreateErrors(t *testing.T) {
	logger := zap.NewNop()
	expectedErr := errors.New("create failed")
	repo := &recordingNotificationRepo{createErr: expectedErr}

	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, notificationRepositoryKey, repo)

	err := NotificationHook(ctx, mentionModel{userID: "u1", mentionedUserID: "u2", statusID: "s1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestNotificationHook_StatusMentions_ContinuesOnCreateError(t *testing.T) {
	logger := zap.NewNop()
	createErr := errors.New("create failed")
	repo := &recordingNotificationRepo{
		createErrForUser: map[string]error{
			"u3": createErr,
		},
	}
	var wg sync.WaitGroup
	repo.pushWG = &wg

	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, notificationRepositoryKey, repo)

	status := &TestStatus{
		ID:       "s1",
		UserID:   "author",
		Content:  "Hello @u3 @u4",
		Mentions: []string{"u3", "u4"},
	}

	// Only the second mention should produce a push notification.
	wg.Add(1)
	require.NoError(t, NotificationHook(ctx, status))
	wg.Wait()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Len(t, repo.created, 2)
	assert.Len(t, repo.pushed, 1)
}
