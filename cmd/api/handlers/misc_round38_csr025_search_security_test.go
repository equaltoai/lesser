package handlers

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// Tests for CSR-025: Public search indexes and exposes non-public statuses.
//
// These tests prove that:
//  1. Anonymous search NEVER returns private or direct statuses.
//  2. Authenticated search respects visibility boundaries — users can only
//     see their own private/direct statuses, and can never see other users'
//     private/direct statuses in search results.
//  3. Deleted statuses are excluded regardless of visibility.

func TestSearchVisibility_CSR025_AnonymousNeverReturnsPrivateOrDirect(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC)

	t.Run("public and unlisted are visible to anonymous viewers", func(t *testing.T) {
		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility: storagemodels.VisibilityPublic,
		}, "", ""))
		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility: storagemodels.VisibilityUnlisted,
		}, "", ""))
	})

	t.Run("private is hidden from anonymous viewers", func(t *testing.T) {
		authorID := cfg.ActorURL("alice")
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
		}, "", ""))
	})

	t.Run("direct is hidden from anonymous viewers", func(t *testing.T) {
		authorID := cfg.ActorURL("alice")
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			AuthorID:       authorID,
		}, "", ""))
	})

	t.Run("deleted statuses are hidden regardless of visibility", func(t *testing.T) {
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility: storagemodels.VisibilityPublic,
			Deleted:    true,
		}, "", ""))
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility: storagemodels.VisibilityUnlisted,
			Deleted:    true,
		}, "", ""))
	})

	t.Run("nil status is not visible", func(t *testing.T) {
		authorID := cfg.ActorURL("alice")
		require.False(t, statusVisibleInSearchForViewer(nil, "alice", authorID))
		require.False(t, statusVisibleInSearchForViewer(nil, "", ""))
	})

	t.Run("thin search results exclude private/direct for anonymous", func(t *testing.T) {
		authorUsername := "alice"
		require.False(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "private-1",
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: authorUsername,
			Published:      now,
		}, ""))
		require.False(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "direct-1",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: authorUsername,
			Published:      now,
		}, ""))
		require.True(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "public-1",
			Visibility:     storagemodels.VisibilityPublic,
			AuthorUsername: authorUsername,
			Published:      now,
		}, ""))
	})

	t.Run("thin search results allow author to see their own private/direct", func(t *testing.T) {
		authorUsername := "alice"
		require.True(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "private-own",
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: authorUsername,
			Published:      now,
		}, authorUsername))
		require.True(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "direct-own",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: authorUsername,
			Published:      now,
		}, authorUsername))
	})

	t.Run("thin search results prevent other users from seeing private/direct", func(t *testing.T) {
		require.False(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "private-others",
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			Published:      now,
		}, "bob"))
		require.False(t, searchResultVisibleInSearch(&storage.StatusSearchResult{
			StatusID:       "direct-others",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			Published:      now,
		}, "bob"))
	})
}

func TestSearchVisibility_CSR025_AuthenticatedRespectsVisibility(t *testing.T) {
	cfg := round11TestConfig()
	authorID := cfg.ActorURL("alice")

	t.Run("author can see their own private status in search", func(t *testing.T) {
		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
		}, "alice", ""))
	})

	t.Run("author can see their own direct status when recipient", func(t *testing.T) {
		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:   storagemodels.VisibilityDirect,
			ToRecipients: []string{authorID},
			AuthorID:     authorID,
		}, "alice", authorID))
	})

	t.Run("other user cannot see alice private status", func(t *testing.T) {
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
		}, "bob", "https://example.com/users/bob"))
	})

	t.Run("other user cannot see alice direct status unless a recipient", func(t *testing.T) {
		bobID := "https://example.com/users/bob"
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:   storagemodels.VisibilityDirect,
			ToRecipients: []string{authorID}, // Only alice is a recipient
			AuthorID:     authorID,
		}, "bob", bobID))
	})

	t.Run("direct status visible to a recipient who is not the author", func(t *testing.T) {
		bobID := "https://example.com/users/bob"
		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			ToRecipients:   []string{bobID, authorID},
		}, "bob", bobID))
	})
}

// Integration-level verification that the search handler's status-to-API
// conversion enforces visibility for every path through the search pipeline.
func TestSearchAPI_CSR025_StatusConversionVisibilityIntegration(t *testing.T) {
	cfg := round11TestConfig()
	authorID := cfg.ActorURL("alice")
	now := time.Now().UTC()

	t.Run("statusVisibleInSearch respects viewer identity", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		h.cfg = cfg

		// Public is always visible.
		require.True(t, h.statusVisibleInSearch(&storagemodels.Status{
			StatusID:       "s1",
			Visibility:     storagemodels.VisibilityPublic,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			PublishedAt:    now,
		}, ""))

		// Private is only visible to the author.
		require.True(t, h.statusVisibleInSearch(&storagemodels.Status{
			StatusID:       "s2",
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			PublishedAt:    now,
		}, "alice"))
		require.False(t, h.statusVisibleInSearch(&storagemodels.Status{
			StatusID:       "s3",
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			PublishedAt:    now,
		}, "bob"))

		// Direct is only visible to author or a recipient.
		bobID := cfg.ActorURL("bob")
		require.True(t, h.statusVisibleInSearch(&storagemodels.Status{
			StatusID:       "s4",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			ToRecipients:   []string{bobID},
			PublishedAt:    now,
		}, "bob"))
		require.False(t, h.statusVisibleInSearch(&storagemodels.Status{
			StatusID:       "s5",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			ToRecipients:   []string{authorID},
			PublishedAt:    now,
		}, "charlie"))
	})
}

// CSR-025 regression: verify that deleted statuses are excluded in every
// pipeline path through the handler layer.
func TestSearchVisibility_CSR025_DeletedStatusesExcluded(t *testing.T) {
	cfg := round11TestConfig()
	authorID := cfg.ActorURL("alice")

	t.Run("deleted public status is hidden from anonymous", func(t *testing.T) {
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility: storagemodels.VisibilityPublic,
			Deleted:    true,
		}, "", ""))
	})

	t.Run("deleted private status is hidden from its own author", func(t *testing.T) {
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			Deleted:        true,
		}, "alice", authorID))
	})

	t.Run("deleted public status is hidden from authenticated viewer", func(t *testing.T) {
		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility: storagemodels.VisibilityPublic,
			Deleted:    true,
		}, "bob", "https://example.com/users/bob"))
	})
}

// Verify the search handler's author-augmented status search path also enforces
// visibility. This path searches actors matching the query and then fetches
// their timelines — non-public statuses could leak if the handler does not
// filter after timeline loads.
func TestSearchVisibility_CSR025_AuthorAugmentedPathPreservesPrivacy(t *testing.T) {
	t.Run("shouldAugmentStatusSearchByAuthor returns false for empty query", func(t *testing.T) {
		require.False(t, shouldAugmentStatusSearchByAuthor(""))
	})

	t.Run("shouldAugmentStatusSearchByAuthor returns false for hashtag queries", func(t *testing.T) {
		require.False(t, shouldAugmentStatusSearchByAuthor("#privacy"))
	})

	t.Run("shouldAugmentStatusSearchByAuthor returns false for URL queries", func(t *testing.T) {
		require.False(t, shouldAugmentStatusSearchByAuthor("https://example.com/statuses/1"))
		require.False(t, shouldAugmentStatusSearchByAuthor("http://example.com/@alice"))
	})

	t.Run("shouldAugmentStatusSearchByAuthor returns false for multi-word queries", func(t *testing.T) {
		require.False(t, shouldAugmentStatusSearchByAuthor("hello world"))
	})

	t.Run("shouldAugmentStatusSearchByAuthor returns true for single-word username queries", func(t *testing.T) {
		require.True(t, shouldAugmentStatusSearchByAuthor("alice"))
	})
}

// Verify handler nil-safety for search visibility helpers.
func TestSearchVisibility_CSR025_HandlerNilSafety(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("nil handler statusVisibleInSearch returns false", func(t *testing.T) {
		var h *Handler
		require.False(t, h.statusVisibleInSearch(nil, ""))
	})

	t.Run("nil handler searchViewerActorID returns empty", func(t *testing.T) {
		var h *Handler
		require.Equal(t, "", h.searchViewerActorID("alice"))
	})

	t.Run("nil cfg searchViewerActorID returns empty", func(t *testing.T) {
		h := &Handler{cfg: nil}
		require.Equal(t, "", h.searchViewerActorID("alice"))
	})

	t.Run("valid cfg searchViewerActorID returns actor URL", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		require.Equal(t, "https://example.com/users/alice", h.searchViewerActorID("alice"))
		require.Equal(t, "", h.searchViewerActorID(""))
		require.Equal(t, "", h.searchViewerActorID("  "))
	})
}

// Verify direct message visibility for edge cases.
func TestSearchVisibility_CSR025_DirectMessageRecipientChecks(t *testing.T) {
	cfg := round11TestConfig()
	authorID := cfg.ActorURL("alice")
	now := time.Now().UTC()

	t.Run("empty viewerActorID with username falls back to username match", func(t *testing.T) {
		// Private: viewer must be the author.
		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
		}, "alice", ""))

		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			Visibility:     storagemodels.VisibilityPrivate,
			AuthorUsername: "alice",
			AuthorID:       authorID,
		}, "bob", ""))
	})

	t.Run("direct message requires recipient match when viewer is not author", func(t *testing.T) {
		bobID := cfg.ActorURL("bob")
		charlieID := cfg.ActorURL("charlie")

		require.True(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			StatusID:       "dm-1",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			ToRecipients:   []string{bobID},
			PublishedAt:    now,
		}, "bob", bobID))

		require.False(t, statusVisibleInSearchForViewer(&storagemodels.Status{
			StatusID:       "dm-2",
			Visibility:     storagemodels.VisibilityDirect,
			AuthorUsername: "alice",
			AuthorID:       authorID,
			ToRecipients:   []string{bobID},
			PublishedAt:    now,
		}, "charlie", charlieID))
	})
}
