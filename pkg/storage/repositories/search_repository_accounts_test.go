package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// Pure Function Tests: Query Normalization
// ============================================================================

func TestNormalizeSearchQuery(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trims_leading_whitespace",
			input:    "   alice",
			expected: "alice",
		},
		{
			name:     "trims_trailing_whitespace",
			input:    "alice   ",
			expected: "alice",
		},
		{
			name:     "trims_both_whitespace",
			input:    "   alice   ",
			expected: "alice",
		},
		{
			name:     "lowercases_input",
			input:    "ALICE",
			expected: "alice",
		},
		{
			name:     "mixed_case_lowercased",
			input:    "AlIcE",
			expected: "alice",
		},
		{
			name:     "strips_leading_at_sign",
			input:    "@alice",
			expected: "alice",
		},
		{
			name:     "strips_at_sign_with_whitespace",
			input:    "  @Alice  ",
			expected: "alice",
		},
		{
			name:     "preserves_internal_at_sign",
			input:    "alice@domain.com",
			expected: "alice@domain.com",
		},
		{
			name:     "handles_empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "handles_single_at_sign",
			input:    "@",
			expected: "",
		},
		{
			name:     "combined_normalization",
			input:    "  @ALiCe  ",
			expected: "alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.normalizeSearchQuery(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Pure Function Tests: Relevance Sorting
// ============================================================================

func TestCompareRelevance(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	tests := []struct {
		name        string
		actorA      *activitypub.Actor
		actorB      *activitypub.Actor
		query       string
		expectAWins bool
		description string
	}{
		{
			name: "exact_match_beats_prefix_match",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "alice",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "alicewonderland",
			},
			query:       "alice",
			expectAWins: true,
			description: "exact username match should rank higher than prefix match",
		},
		{
			name: "prefix_match_beats_non_prefix",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "alicewonderland",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "winalice",
			},
			query:       "alice",
			expectAWins: true,
			description: "prefix match should rank higher than non-prefix",
		},
		{
			name: "shorter_username_wins_when_both_prefix_match",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "alice123",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "alicewonderland",
			},
			query:       "alice",
			expectAWins: true,
			description: "shorter preferredUsername should rank higher when both prefix match",
		},
		{
			name: "shorter_username_wins_when_neither_prefix",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "bob",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "charlie",
			},
			query:       "alice",
			expectAWins: true,
			description: "shorter preferredUsername wins when neither is prefix/exact",
		},
		{
			name: "case_insensitive_exact_match",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "ALICE",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "alicewonderland",
			},
			query:       "alice",
			expectAWins: true,
			description: "case-insensitive exact match should still be considered exact",
		},
		{
			name: "case_insensitive_prefix_match",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "ALICEWONDER",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "wonderalice",
			},
			query:       "alice",
			expectAWins: true,
			description: "case-insensitive prefix match should rank higher",
		},
		{
			name: "both_exact_matches_shorter_wins",
			actorA: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-a"},
				PreferredUsername: "alice",
			},
			actorB: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "actor-b"},
				PreferredUsername: "alice",
			},
			query:       "alice",
			expectAWins: false, // Equal length, so neither strictly wins
			description: "equal lengths should not have A strictly winning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.compareRelevance(tt.actorA, tt.actorB, tt.query)
			assert.Equal(t, tt.expectAWins, result, tt.description)
		})
	}
}

func TestSortAccountResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	tests := []struct {
		name             string
		results          []*activitypub.Actor
		query            string
		expectedOrderIDs []string
		description      string
	}{
		{
			name: "sorts_exact_first_then_prefix_then_length",
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alicewonderland"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "alice"},
				{BaseObject: activitypub.BaseObject{ID: "3"}, PreferredUsername: "alice123"},
			},
			query:            "alice",
			expectedOrderIDs: []string{"2", "3", "1"},
			description:      "exact match first, then shorter prefix, then longer prefix",
		},
		{
			name: "all_prefix_matches_sorted_by_length",
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alicewonderland"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "alice123"},
				{BaseObject: activitypub.BaseObject{ID: "3"}, PreferredUsername: "alicex"},
			},
			query:            "alice",
			expectedOrderIDs: []string{"3", "2", "1"},
			description:      "all prefix: shortest first",
		},
		{
			name: "no_matches_sorted_by_length",
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "charlie"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "bob"},
				{BaseObject: activitypub.BaseObject{ID: "3"}, PreferredUsername: "dave"},
			},
			query:            "alice",
			expectedOrderIDs: []string{"2", "3", "1"},
			description:      "no matches: sorted by username length",
		},
		{
			name:             "empty_results",
			results:          []*activitypub.Actor{},
			query:            "alice",
			expectedOrderIDs: []string{},
			description:      "empty results should not panic",
		},
		{
			name: "single_result",
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
			},
			query:            "alice",
			expectedOrderIDs: []string{"1"},
			description:      "single result should return unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data
			resultsCopy := make([]*activitypub.Actor, len(tt.results))
			copy(resultsCopy, tt.results)

			repo.sortAccountResults(resultsCopy, tt.query)

			var resultIDs []string
			for _, actor := range resultsCopy {
				resultIDs = append(resultIDs, actor.ID)
			}

			if len(tt.expectedOrderIDs) == 0 {
				assert.Empty(t, resultIDs)
			} else {
				assert.Equal(t, tt.expectedOrderIDs, resultIDs, tt.description)
			}
		})
	}
}

// ============================================================================
// Pure Function Tests: Pagination
// ============================================================================

func TestSearchRepositoryPaginateResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	// Create test actors
	actors := []*activitypub.Actor{
		{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
		{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "bob"},
		{BaseObject: activitypub.BaseObject{ID: "3"}, PreferredUsername: "charlie"},
		{BaseObject: activitypub.BaseObject{ID: "4"}, PreferredUsername: "dave"},
		{BaseObject: activitypub.BaseObject{ID: "5"}, PreferredUsername: "eve"},
	}

	tests := []struct {
		name          string
		results       []*activitypub.Actor
		offset        int
		limit         int
		expectedCount int
		expectedIDs   []string
		description   string
	}{
		{
			name:          "offset_zero_with_limit",
			results:       actors,
			offset:        0,
			limit:         3,
			expectedCount: 3,
			expectedIDs:   []string{"1", "2", "3"},
			description:   "offset 0 returns first 'limit' items",
		},
		{
			name:          "offset_applies_before_limit",
			results:       actors,
			offset:        2,
			limit:         2,
			expectedCount: 2,
			expectedIDs:   []string{"3", "4"},
			description:   "offset is applied, then limit",
		},
		{
			name:          "offset_at_end_of_results",
			results:       actors,
			offset:        3,
			limit:         10,
			expectedCount: 2,
			expectedIDs:   []string{"4", "5"},
			description:   "offset near end, fewer than limit returned",
		},
		{
			name:          "offset_beyond_results_returns_empty",
			results:       actors,
			offset:        10,
			limit:         5,
			expectedCount: 0,
			expectedIDs:   []string{},
			description:   "offset >= len(results) returns empty slice",
		},
		{
			name:          "offset_exactly_at_length_returns_empty",
			results:       actors,
			offset:        5,
			limit:         5,
			expectedCount: 0,
			expectedIDs:   []string{},
			description:   "offset == len(results) returns empty slice",
		},
		{
			name:          "limit_greater_than_remaining",
			results:       actors,
			offset:        0,
			limit:         100,
			expectedCount: 5,
			expectedIDs:   []string{"1", "2", "3", "4", "5"},
			description:   "limit greater than available returns all",
		},
		{
			name:          "empty_results",
			results:       []*activitypub.Actor{},
			offset:        0,
			limit:         10,
			expectedCount: 0,
			expectedIDs:   []string{},
			description:   "empty input returns empty output",
		},
		{
			name:          "zero_limit_after_offset",
			results:       actors,
			offset:        0,
			limit:         0,
			expectedCount: 0,
			expectedIDs:   []string{},
			description:   "zero limit returns empty after offset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.paginateResults(tt.results, tt.offset, tt.limit)

			assert.Len(t, result, tt.expectedCount, tt.description)

			var resultIDs []string
			for _, actor := range result {
				resultIDs = append(resultIDs, actor.ID)
			}

			if len(tt.expectedIDs) == 0 {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tt.expectedIDs, resultIDs)
			}
		})
	}
}

// ============================================================================
// Dependency Tests: Following Filter
// ============================================================================

// mockSearchRepositoryDeps is a simple mock for SearchRepositoryDeps interface
type mockSearchRepositoryDeps struct {
	getFollowingFunc           func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	isBlockedFunc              func(ctx context.Context, blockerActor, blockedActor string) (bool, error)
	isBlockedBidirectionalFunc func(ctx context.Context, actor1, actor2 string) (bool, error)
	getFollowersFunc           func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

func (m *mockSearchRepositoryDeps) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if m.getFollowingFunc != nil {
		return m.getFollowingFunc(ctx, username, limit, cursor)
	}
	return nil, "", nil
}

func (m *mockSearchRepositoryDeps) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	if m.isBlockedFunc != nil {
		return m.isBlockedFunc(ctx, blockerActor, blockedActor)
	}
	return false, nil
}

func (m *mockSearchRepositoryDeps) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	if m.isBlockedBidirectionalFunc != nil {
		return m.isBlockedBidirectionalFunc(ctx, actor1, actor2)
	}
	return false, nil
}

func (m *mockSearchRepositoryDeps) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if m.getFollowersFunc != nil {
		return m.getFollowersFunc(ctx, username, limit, cursor)
	}
	return nil, "", nil
}

func TestApplyFollowingFilter(t *testing.T) {
	mockDB := new(mocks.MockDB)

	tests := []struct {
		name        string
		deps        SearchRepositoryDeps
		results     []*activitypub.Actor
		accountID   string
		expectedIDs []string
		description string
	}{
		{
			name: "nil_deps_returns_original_results",
			deps: nil,
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "bob"},
			},
			accountID:   "user1",
			expectedIDs: []string{"1", "2"},
			description: "when deps is nil, return original results without panic",
		},
		{
			name: "filters_to_following_only",
			deps: &mockSearchRepositoryDeps{
				getFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
					return []string{"alice", "charlie"}, "", nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "bob"},
				{BaseObject: activitypub.BaseObject{ID: "3"}, PreferredUsername: "charlie"},
			},
			accountID:   "user1",
			expectedIDs: []string{"1", "3"},
			description: "only actors with PreferredUsername in following list are returned",
		},
		{
			name: "getfollowing_error_returns_original",
			deps: &mockSearchRepositoryDeps{
				getFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
					return nil, "", errors.New("database error")
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "bob"},
			},
			accountID:   "user1",
			expectedIDs: []string{"1", "2"},
			description: "on error, fail open and return original results",
		},
		{
			name: "empty_following_list",
			deps: &mockSearchRepositoryDeps{
				getFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
					return []string{}, "", nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
			},
			accountID:   "user1",
			expectedIDs: []string{},
			description: "empty following list returns empty results",
		},
		{
			name: "all_in_following_list",
			deps: &mockSearchRepositoryDeps{
				getFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
					return []string{"alice", "bob", "charlie"}, "", nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "1"}, PreferredUsername: "alice"},
				{BaseObject: activitypub.BaseObject{ID: "2"}, PreferredUsername: "bob"},
			},
			accountID:   "user1",
			expectedIDs: []string{"1", "2"},
			description: "all results returned when all are in following list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &SearchRepository{
				EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
					mockDB, "test-table", zap.NewNop(), nil, "SearchRepository", "search",
				),
				deps: tt.deps,
			}

			ctx := context.Background()
			result := repo.applyFollowingFilter(ctx, tt.results, tt.accountID)

			var resultIDs []string
			for _, actor := range result {
				resultIDs = append(resultIDs, actor.ID)
			}

			if len(tt.expectedIDs) == 0 {
				assert.Empty(t, result, tt.description)
			} else {
				assert.Equal(t, tt.expectedIDs, resultIDs, tt.description)
			}
		})
	}
}

// ============================================================================
// Dependency Tests: Privacy Filtering (Block Rules)
// ============================================================================

func TestFilterAccountsByPrivacy(t *testing.T) {
	mockDB := new(mocks.MockDB)

	tests := []struct {
		name            string
		deps            *mockSearchRepositoryDeps
		results         []*activitypub.Actor
		searcherActorID string
		expectedIDs     []string
		description     string
	}{
		{
			name: "searcher_own_actor_always_included",
			deps: &mockSearchRepositoryDeps{
				isBlockedBidirectionalFunc: func(ctx context.Context, actor1, actor2 string) (bool, error) {
					// Should never be called for self
					return true, nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "searcher-1"}, PreferredUsername: "searcher"},
			},
			searcherActorID: "searcher-1",
			expectedIDs:     []string{"searcher-1"},
			description:     "searcher's own actor passes without block check (ID equality short-circuit)",
		},
		{
			name: "blocked_actor_excluded",
			deps: &mockSearchRepositoryDeps{
				isBlockedBidirectionalFunc: func(ctx context.Context, actor1, actor2 string) (bool, error) {
					if actor2 == "blocked-user" {
						return true, nil
					}
					return false, nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "normal-user"}, PreferredUsername: "normal"},
				{BaseObject: activitypub.BaseObject{ID: "blocked-user"}, PreferredUsername: "blocked"},
			},
			searcherActorID: "searcher-1",
			expectedIDs:     []string{"normal-user"},
			description:     "blocked actor is excluded from results",
		},
		{
			name: "error_on_block_check_includes_target",
			deps: &mockSearchRepositoryDeps{
				isBlockedBidirectionalFunc: func(ctx context.Context, actor1, actor2 string) (bool, error) {
					return false, errors.New("database error")
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "user-1"}, PreferredUsername: "user1"},
			},
			searcherActorID: "searcher-1",
			expectedIDs:     []string{"user-1"},
			description:     "on error, fail open and include the target",
		},
		{
			name: "nil_actor_in_results_skipped",
			deps: &mockSearchRepositoryDeps{
				isBlockedBidirectionalFunc: func(ctx context.Context, actor1, actor2 string) (bool, error) {
					return false, nil
				},
			},
			results: []*activitypub.Actor{
				nil,
				{BaseObject: activitypub.BaseObject{ID: "user-1"}, PreferredUsername: "user1"},
				nil,
			},
			searcherActorID: "searcher-1",
			expectedIDs:     []string{"user-1"},
			description:     "nil actors in results are skipped",
		},
		{
			name: "mixed_blocked_and_allowed",
			deps: &mockSearchRepositoryDeps{
				isBlockedBidirectionalFunc: func(ctx context.Context, actor1, actor2 string) (bool, error) {
					blockedIDs := map[string]bool{"blocked-1": true, "blocked-2": true}
					return blockedIDs[actor2], nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "allowed-1"}, PreferredUsername: "allowed1"},
				{BaseObject: activitypub.BaseObject{ID: "blocked-1"}, PreferredUsername: "blocked1"},
				{BaseObject: activitypub.BaseObject{ID: "allowed-2"}, PreferredUsername: "allowed2"},
				{BaseObject: activitypub.BaseObject{ID: "blocked-2"}, PreferredUsername: "blocked2"},
				{BaseObject: activitypub.BaseObject{ID: "searcher-1"}, PreferredUsername: "searcher"},
			},
			searcherActorID: "searcher-1",
			expectedIDs:     []string{"allowed-1", "allowed-2", "searcher-1"},
			description:     "mixed scenario: blocked excluded, allowed and self included",
		},
		{
			name: "all_actors_blocked",
			deps: &mockSearchRepositoryDeps{
				isBlockedBidirectionalFunc: func(ctx context.Context, actor1, actor2 string) (bool, error) {
					return true, nil
				},
			},
			results: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "user-1"}, PreferredUsername: "user1"},
				{BaseObject: activitypub.BaseObject{ID: "user-2"}, PreferredUsername: "user2"},
			},
			searcherActorID: "searcher-1",
			expectedIDs:     []string{},
			description:     "all blocked returns empty (except self)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &SearchRepository{
				EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
					mockDB, "test-table", zap.NewNop(), nil, "SearchRepository", "search",
				),
				deps: tt.deps,
			}

			ctx := context.Background()
			result := repo.filterAccountsByPrivacy(ctx, tt.results, tt.searcherActorID)

			var resultIDs []string
			for _, actor := range result {
				resultIDs = append(resultIDs, actor.ID)
			}

			if len(tt.expectedIDs) == 0 {
				assert.Empty(t, result, tt.description)
			} else {
				assert.Equal(t, tt.expectedIDs, resultIDs, tt.description)
			}
		})
	}
}
