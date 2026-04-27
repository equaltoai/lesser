package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/hashtags"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

// Basic tests for hashtag query resolvers
// More comprehensive integration tests are in the tests/ directory

func TestHashtagQueryResolver_BasicStructure(t *testing.T) {
	// Test that the resolver exists and has correct signature
	// This is a compile-time check - if this compiles, the resolver is correctly structured
	var r queryResolver
	ctx := context.Background()

	t.Run("Hashtag resolver exists", func(t *testing.T) {
		// This will panic without proper setup, but we're just checking structure
		defer func() {
			if r := recover(); r != nil {
				// Expected - we don't have a full resolver setup
			}
		}()
		_, _ = r.Hashtag(ctx, "golang")
	})

	t.Run("FollowedHashtags resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		_, _ = r.FollowedHashtags(ctx, nil, nil)
	})

	t.Run("HashtagTimeline resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		_, _ = r.HashtagTimeline(ctx, "golang", nil, nil, nil)
	})

	t.Run("MultiHashtagTimeline resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		_, _ = r.MultiHashtagTimeline(ctx, []string{"golang"}, model.HashtagModeAny, nil, nil)
	})

	t.Run("SuggestedHashtags resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		_, _ = r.SuggestedHashtags(ctx, nil)
	})
}

func TestConvertHashtagToModel(t *testing.T) {
	// Test the hashtag conversion helper
	r := &Resolver{}
	ctx := context.Background()

	t.Run("nil hashtag returns nil", func(t *testing.T) {
		result := r.convertHashtagToModel(ctx, nil, "testuser")
		assert.Nil(t, result)
	})

	t.Run("converts service hashtag to model", func(t *testing.T) {
		serviceHashtag := &hashtags.Hashtag{
			Name:          "golang",
			URL:           "https://example.com/tags/golang",
			PostCount:     100,
			FollowerCount: 50,
			IsFollowing:   true,
			IsMuted:       false,
			Related:       []string{"programming", "software"},
		}

		result := r.convertHashtagToModel(ctx, serviceHashtag, "testuser")
		assert.NotNil(t, result)
		assert.Equal(t, "golang", result.Name)
		assert.Equal(t, 100, result.PostCount)
		assert.Equal(t, 50, result.FollowerCount)
		assert.True(t, result.IsFollowing)
		assert.Equal(t, "#golang", result.DisplayName)
	})
}

func TestFetchHashtagNotificationSettings(t *testing.T) {
	// Test the notification settings helper
	r := &Resolver{}
	ctx := context.Background()

	t.Run("empty hashtag returns default settings", func(t *testing.T) {
		result := r.fetchHashtagNotificationSettings(ctx, "", "testuser")
		assert.NotNil(t, result)
		assert.False(t, result.Muted)
	})

	t.Run("empty userID returns default settings", func(t *testing.T) {
		result := r.fetchHashtagNotificationSettings(ctx, "golang", "")
		assert.NotNil(t, result)
		assert.False(t, result.Muted)
	})
}

func TestIsFollowingHashtag(t *testing.T) {
	r := &Resolver{}
	ctx := context.Background()

	t.Run("empty userID returns false", func(t *testing.T) {
		result := r.isFollowingHashtag(ctx, "", "golang")
		assert.False(t, result)
	})

	t.Run("empty hashtag returns false", func(t *testing.T) {
		result := r.isFollowingHashtag(ctx, "testuser", "")
		assert.False(t, result)
	})
}

func TestIsHashtagMuted(t *testing.T) {
	r := &Resolver{}
	ctx := context.Background()

	t.Run("empty userID returns false", func(t *testing.T) {
		result := r.isHashtagMuted(ctx, "", "golang")
		assert.False(t, result)
	})

	t.Run("empty hashtag returns false", func(t *testing.T) {
		result := r.isHashtagMuted(ctx, "testuser", "")
		assert.False(t, result)
	})
}

func TestNormalizeTimelineArgs(t *testing.T) {
	defaultLimit := 20

	limit, cursor := normalizeTimelineArgs(nil, nil, defaultLimit)
	assert.Equal(t, defaultLimit, limit)
	assert.Empty(t, cursor)

	first := 5
	after := "cursor123"
	limit, cursor = normalizeTimelineArgs(&first, &after, defaultLimit)
	assert.Equal(t, first, limit)
	assert.Equal(t, after, cursor)

	invalid := -10
	limit, cursor = normalizeTimelineArgs(&invalid, nil, defaultLimit)
	assert.Equal(t, defaultLimit, limit)
	assert.Empty(t, cursor)
}

func TestBuildHashtagTimelineEdges_MediaFilter(t *testing.T) {
	ctx := context.Background()
	resolver := &Resolver{}
	query := &queryResolver{resolver}

	posts := []*storage.StatusSearchResult{
		{StatusID: "1", Content: "first"},
		{StatusID: "2", Content: "second"},
		{StatusID: "3", Content: "third"},
	}

	statusByID := map[string]*models.Status{
		"1": {StatusID: "1", MediaCount: 1},
		"2": {StatusID: "2", MediaCount: 0},
	}

	edges := query.buildHashtagTimelineEdges(ctx, posts, statusByID, true)
	assert.Len(t, edges, 1)
	assert.Equal(t, model.Cursor("1"), edges[0].Cursor)
	assert.NotNil(t, edges[0].Node)
}

func TestBuildHashtagTimelineEdges_FiltersNonPublicVisibility(t *testing.T) {
	ctx := context.Background()
	resolver := &Resolver{}
	query := &queryResolver{resolver}

	posts := []*storage.StatusSearchResult{
		{StatusID: "public", Content: "public", Visibility: models.VisibilityPublic},
		{StatusID: "private", Content: "private", Visibility: models.VisibilityPrivate},
		{StatusID: "direct", Content: "direct", Visibility: models.VisibilityDirect},
	}

	statusByID := map[string]*models.Status{
		"public":  {StatusID: "public", Visibility: models.VisibilityPublic},
		"private": {StatusID: "private", Visibility: models.VisibilityPrivate},
	}

	edges := query.buildHashtagTimelineEdges(ctx, posts, statusByID, false)
	assert.Len(t, edges, 1)
	assert.Equal(t, model.Cursor("public"), edges[0].Cursor)
}

func TestBuildPostConnection(t *testing.T) {
	edges := []*model.PostEdge{
		{Cursor: "a"},
		{Cursor: "b"},
	}

	connection := buildPostConnection(edges, true)
	assert.NotNil(t, connection)
	assert.Equal(t, edges, connection.Edges)
	assert.Equal(t, len(edges), connection.TotalCount)
	assert.True(t, connection.PageInfo.HasNextPage)
	assert.False(t, connection.PageInfo.HasPreviousPage)
	if assert.NotNil(t, connection.PageInfo.StartCursor) {
		assert.Equal(t, model.Cursor("a"), *connection.PageInfo.StartCursor)
	}
	if assert.NotNil(t, connection.PageInfo.EndCursor) {
		assert.Equal(t, model.Cursor("b"), *connection.PageInfo.EndCursor)
	}
}

// Additional integration tests with full mock setup should be added
// in the tests/graphql directory
