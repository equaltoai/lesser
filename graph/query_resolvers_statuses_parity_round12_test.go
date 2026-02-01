package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_StatusesParity_HelperFunctions(t *testing.T) {
	require.Nil(t, optionalString(""))
	require.Nil(t, optionalString("   "))
	require.NotNil(t, optionalString("x"))

	require.Equal(t, "fallback", getStringFromMapWithFallback(nil, "k", "fallback"))
	require.Equal(t, "fallback", getStringFromMapWithFallback(map[string]any{"k": ""}, "k", "fallback"))
	require.Equal(t, "value", getStringFromMapWithFallback(map[string]any{"k": " value "}, "k", "fallback"))

	require.True(t, getBoolFromMap(map[string]any{"k": true}, "k", false))
	require.False(t, getBoolFromMap(map[string]any{"k": "nope"}, "k", false))

	require.False(t, statusContainsLink(nil, "https://example.com"))
	require.False(t, statusContainsLink(&storageModels.Status{}, ""))
	require.True(t, statusContainsLink(&storageModels.Status{
		URLs: []string{"https://example.com"},
	}, "https://EXAMPLE.com"))
	require.True(t, statusContainsLink(&storageModels.Status{
		Content: "see https://example.com",
	}, "https://example.com"))
}

func TestRound12QueryResolvers_StatusesParity_LinkTimelineAndTranslateDisabled(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	q := &queryResolver{resolver}
	ctx := context.Background()

	// Seed statuses for SearchStatuses.
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-1",
		AuthorID:       config.Get().ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "link: https://example.com",
		URLs:           []string{"https://example.com"},
		PublishedAt:    time.Now().Add(-time.Minute),
		CreatedAt:      time.Now().Add(-time.Minute),
		UpdatedAt:      time.Now().Add(-time.Minute),
		Visibility:     storageModels.VisibilityPublic,
	}))
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-2",
		AuthorID:       config.Get().ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "uppercase mismatch https://example.com",
		PublishedAt:    time.Now().Add(-2 * time.Minute),
		CreatedAt:      time.Now().Add(-2 * time.Minute),
		UpdatedAt:      time.Now().Add(-2 * time.Minute),
		Visibility:     storageModels.VisibilityPublic,
	}))

	first := 10
	link := "HTTPS://EXAMPLE.COM"
	conn, err := q.LinkTimeline(ctx, link, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 1)

	// TranslateStatus should fail fast when translation is disabled.
	_, err = q.TranslateStatus(round12AuthContext("alice"), "status-1", nil)
	require.Error(t, err)
}

func TestRound12QueryResolvers_StatusesParity_StatusActorListPage_CustomFetcher(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	page, err := q.resolveStatusActorListPage(context.Background(), "status-1", nil, nil, "custom", func(ctx context.Context, _ *notes.Service, _ string, _ interfaces.PaginationOptions) (*notes.UsersResult, error) {
		return &notes.UsersResult{
			Users: []*storage.Account{
				{
					User: &storage.User{Username: "bob"},
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: "https://localhost/users/bob", Type: activitypub.PersonType},
						PreferredUsername: "bob",
					},
				},
				nil,
			},
			Pagination: &interfaces.PaginatedResult[*storage.Account]{
				NextCursor: "next",
				HasMore:    true,
				Total:      42,
			},
		}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, page)
	require.Equal(t, 42, page.TotalCount)
	require.NotNil(t, page.NextCursor)

	_, err = q.StatusFavouritedBy(context.Background(), "", nil, nil)
	require.Error(t, err)
	_, err = q.StatusRebloggedBy(context.Background(), "", nil, nil)
	require.Error(t, err)
}
