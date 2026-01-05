package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type userRepoDepsStub struct {
	GetFollowersFunc func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	GetListsContainingAccountFunc func(ctx context.Context, accountID, username string) ([]*storage.List, error)

	CreateTimelineEntriesFunc func(ctx context.Context, entries []*models.Timeline) error

	GetPendingFollowRequestsFunc func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	RemoveFollowFunc func(ctx context.Context, followerUsername, username string) error
}

func (s *userRepoDepsStub) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if s.GetFollowersFunc != nil {
		return s.GetFollowersFunc(ctx, username, limit, cursor)
	}
	return nil, "", nil
}

func (s *userRepoDepsStub) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	if s.GetListsContainingAccountFunc != nil {
		return s.GetListsContainingAccountFunc(ctx, accountID, username)
	}
	return nil, nil
}

func (s *userRepoDepsStub) CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error {
	if s.CreateTimelineEntriesFunc != nil {
		return s.CreateTimelineEntriesFunc(ctx, entries)
	}
	return nil
}

func (s *userRepoDepsStub) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if s.GetPendingFollowRequestsFunc != nil {
		return s.GetPendingFollowRequestsFunc(ctx, username, limit, cursor)
	}
	return nil, "", nil
}

func (s *userRepoDepsStub) RemoveFollow(ctx context.Context, followerUsername, username string) error {
	if s.RemoveFollowFunc != nil {
		return s.RemoveFollowFunc(ctx, followerUsername, username)
	}
	return nil
}

func TestUserRepository_FanOutPost_NonCreate_NoOp(t *testing.T) {
	repo := &UserRepository{logger: zap.NewNop()}

	createCalled := false
	repo.SetDependencies(&userRepoDepsStub{
		CreateTimelineEntriesFunc: func(_ context.Context, _ []*models.Timeline) error {
			createCalled = true
			return nil
		},
	})

	err := repo.FanOutPost(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "activity-1",
			Type: activitypub.UpdateType,
		},
		Actor:  "http://localhost/users/alice",
		Object: map[string]any{},
	})
	require.NoError(t, err)
	assert.False(t, createCalled)
}

func TestUserRepository_FanOutPost_UnsupportedObject_Ignored(t *testing.T) {
	repo := &UserRepository{logger: zap.NewNop()}

	createCalled := false
	repo.SetDependencies(&userRepoDepsStub{
		CreateTimelineEntriesFunc: func(_ context.Context, _ []*models.Timeline) error {
			createCalled = true
			return nil
		},
	})

	err := repo.FanOutPost(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "activity-2",
			Type: activitypub.CreateType,
		},
		Actor:  "http://localhost/users/alice",
		Object: 123,
	})
	require.NoError(t, err)
	assert.False(t, createCalled)
}

func TestUserRepository_FanOutPost_Direct_NoEntries(t *testing.T) {
	repo := &UserRepository{logger: zap.NewNop()}

	createCalled := false
	repo.SetDependencies(&userRepoDepsStub{
		CreateTimelineEntriesFunc: func(_ context.Context, _ []*models.Timeline) error {
			createCalled = true
			return nil
		},
	})

	err := repo.FanOutPost(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "activity-3",
			Type: activitypub.CreateType,
		},
		Actor: "http://localhost/users/alice",
		Object: map[string]any{
			"id":           "http://localhost/objects/1",
			"type":         "Note",
			"content":      "hello",
			"attributedTo": "http://localhost/users/alice",
			"to":           []any{"http://localhost/users/bob"},
		},
	})
	require.NoError(t, err)
	assert.False(t, createCalled)
}

func TestUserRepository_FanOutPost_Public_CreatesExpectedEntries(t *testing.T) {
	repo := &UserRepository{logger: zap.NewNop()}

	var gotEntries []*models.Timeline
	followersCallCount := 0

	repo.SetDependencies(&userRepoDepsStub{
		GetFollowersFunc: func(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
			followersCallCount++
			switch cursor {
			case "":
				return []string{
					"https://remote.example/users/bob",
					"invalid-follower-id",
				}, "next", nil
			case "next":
				return []string{
					"https://remote.example/users/carol",
				}, "", nil
			default:
				return nil, "", nil
			}
		},
		GetListsContainingAccountFunc: func(_ context.Context, _ string, _ string) ([]*storage.List, error) {
			return []*storage.List{
				{ID: "list-1", RepliesPolicy: "none"},
				{ID: "list-2", RepliesPolicy: "followed"},
				{ID: "list-3", RepliesPolicy: "list"},
			}, nil
		},
		CreateTimelineEntriesFunc: func(_ context.Context, entries []*models.Timeline) error {
			gotEntries = append([]*models.Timeline(nil), entries...)
			return nil
		},
	})

	published := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	content := strings.Repeat("x", 600)

	err := repo.FanOutPost(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "activity-4",
			Type: activitypub.CreateType,
		},
		Actor: "http://localhost/users/alice",
		Object: map[string]any{
			"id":           "http://localhost/objects/2",
			"type":         "Note",
			"content":      content,
			"attributedTo": "http://localhost/users/alice",
			"to":           []any{activitypub.PublicAddress},
			"cc":           []any{},
			"tag": []any{
				map[string]any{"type": "Hashtag", "name": "#GoLang", "href": "https://example.com/tags/golang"},
				"invalid-tag",
				map[string]any{"type": "Mention", "name": "@bob", "href": "https://remote.example/users/bob"},
				map[string]any{"type": "Hashtag", "name": "", "href": "https://example.com/tags/empty"},
			},
			"attachment": []any{map[string]any{"type": "Image", "url": "https://example.com/img.png"}},
			"language":   "es",
			"published":  published.Format(time.RFC3339),
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, followersCallCount)
	require.NotEmpty(t, gotEntries)

	typeCounts := map[string]int{}
	for _, entry := range gotEntries {
		typeCounts[entry.TimelineType]++
		assert.Equal(t, "http://localhost/objects/2", entry.PostID)
		assert.Equal(t, "alice", entry.ActorHandle)
		assert.Equal(t, "http://localhost/users/alice", entry.ActorID)
		assert.Len(t, entry.Content, 500)
		assert.True(t, entry.HasMedia)
		assert.Equal(t, "es", entry.Language)
		assert.Equal(t, published, entry.CreatedAt)
		assert.NotEmpty(t, entry.EntryID)
	}

	assert.Equal(t, 2, typeCounts["HOME"])
	assert.Equal(t, 2, typeCounts["PUBLIC"])
	assert.Equal(t, 1, typeCounts["HASHTAG"])
	assert.Equal(t, 3, typeCounts["LIST"])

	var hashtagFound bool
	for _, entry := range gotEntries {
		if entry.TimelineType == "HASHTAG" {
			hashtagFound = true
			assert.Equal(t, "golang", entry.TimelineID)
		}
	}
	assert.True(t, hashtagFound)
}
