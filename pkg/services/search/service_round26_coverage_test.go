package search

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeSearchRepo struct {
	searchAccountsCalls []fakeSearchAccountsCall
	searchAccountsErr   error
	searchAccountsResp  []*activitypub.Actor

	searchStatusesCalls []fakeSearchStatusesCall
	searchStatusesErr   error
	searchStatusesResp  []*storage.StatusSearchResult

	searchHashtagsCalls []fakeSearchHashtagsCall
	searchHashtagsErr   error
	searchHashtagsResp  []*storage.Hashtag
}

type fakeSearchAccountsCall struct {
	query         string
	limit         int
	followingOnly bool
	offset        int
}

func (r *fakeSearchRepo) SearchAccounts(_ context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	r.searchAccountsCalls = append(r.searchAccountsCalls, fakeSearchAccountsCall{
		query:         query,
		limit:         limit,
		followingOnly: followingOnly,
		offset:        offset,
	})
	if r.searchAccountsErr != nil {
		return nil, r.searchAccountsErr
	}
	return r.searchAccountsResp, nil
}

type fakeSearchStatusesCall struct {
	query string
	limit int
}

func (r *fakeSearchRepo) SearchStatuses(_ context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	r.searchStatusesCalls = append(r.searchStatusesCalls, fakeSearchStatusesCall{query: query, limit: limit})
	if r.searchStatusesErr != nil {
		return nil, r.searchStatusesErr
	}
	return r.searchStatusesResp, nil
}

type fakeSearchHashtagsCall struct {
	query string
	limit int
}

func (r *fakeSearchRepo) SearchHashtags(_ context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	r.searchHashtagsCalls = append(r.searchHashtagsCalls, fakeSearchHashtagsCall{query: query, limit: limit})
	if r.searchHashtagsErr != nil {
		return nil, r.searchHashtagsErr
	}
	return r.searchHashtagsResp, nil
}

type fakeActorRepo struct {
	getSuggestionsCalls []fakeSuggestionCall
	getSuggestionsErr   error
	getSuggestionsResp  []*activitypub.Actor

	removeCalls []fakeRemoveSuggestionCall
	removeErr   error
}

type fakeSuggestionCall struct {
	user  string
	limit int
}

func (r *fakeActorRepo) GetAccountSuggestions(_ context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	r.getSuggestionsCalls = append(r.getSuggestionsCalls, fakeSuggestionCall{user: userID, limit: limit})
	if r.getSuggestionsErr != nil {
		return nil, r.getSuggestionsErr
	}
	return r.getSuggestionsResp, nil
}

type fakeRemoveSuggestionCall struct {
	user     string
	targetID string
}

func (r *fakeActorRepo) RemoveAccountSuggestion(_ context.Context, userID, targetID string) error {
	r.removeCalls = append(r.removeCalls, fakeRemoveSuggestionCall{user: userID, targetID: targetID})
	return r.removeErr
}

type fakeRelationshipRepo struct {
	followersCount int
	followingCount int
}

func (r *fakeRelationshipRepo) CountFollowers(context.Context, string) (int, error) {
	return r.followersCount, nil
}

func (r *fakeRelationshipRepo) CountFollowing(context.Context, string) (int, error) {
	return r.followingCount, nil
}

type fakeStatusRepo struct {
	statusesByAuthor int

	statusCountsByID map[string][3]int
	statusCountsErr  error
	calls            []string
}

func (r *fakeStatusRepo) CountStatusesByAuthor(context.Context, string) (int, error) {
	return r.statusesByAuthor, nil
}

func (r *fakeStatusRepo) GetStatusCounts(_ context.Context, statusID string) (likes, reblogs, replies int, err error) {
	r.calls = append(r.calls, statusID)
	if r.statusCountsErr != nil {
		return 0, 0, 0, r.statusCountsErr
	}
	counts := r.statusCountsByID[statusID]
	return counts[0], counts[1], counts[2], nil
}

type fakeHashtagRepo struct {
	followingByUserHashtag map[string]bool
	errByUserHashtag       map[string]error
	calls                  []string
}

func (r *fakeHashtagRepo) IsFollowingHashtag(_ context.Context, userID, hashtag string) (bool, error) {
	key := userID + "#" + hashtag
	r.calls = append(r.calls, key)
	if err := r.errByUserHashtag[key]; err != nil {
		return false, err
	}
	return r.followingByUserHashtag[key], nil
}

func TestService_Search_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("accounts_error_is_wrapped", func(t *testing.T) {
		repo := &fakeSearchRepo{searchAccountsErr: stderrors.New("boom")}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		_, err := svc.Search(ctx, &Query{Query: "a", Type: "accounts"})
		require.ErrorIs(t, err, pkgerrors.ErrSearchAccounts)
	})

	t.Run("hashtags_error_is_wrapped", func(t *testing.T) {
		repo := &fakeSearchRepo{searchHashtagsErr: stderrors.New("boom")}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		_, err := svc.Search(ctx, &Query{Query: "a", Type: "hashtags"})
		require.ErrorIs(t, err, pkgerrors.ErrSearchHashtags)
	})

	t.Run("statuses_error_is_wrapped", func(t *testing.T) {
		repo := &fakeSearchRepo{searchStatusesErr: stderrors.New("boom")}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		_, err := svc.Search(ctx, &Query{Query: "a", Type: "statuses"})
		require.ErrorIs(t, err, pkgerrors.ErrSearchStatuses)
	})

	t.Run("default_type_ignores_sub_errors_but_emits_event", func(t *testing.T) {
		repo := &fakeSearchRepo{
			searchAccountsErr: stderrors.New("accounts fail"),
			searchHashtagsErr: stderrors.New("hashtags fail"),
			searchStatusesErr: stderrors.New("statuses fail"),
		}
		pub := &fakePublisher{err: stderrors.New("ignored")}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, pub, zap.NewNop(), "example.com")

		result, err := svc.Search(ctx, &Query{Query: "a"})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, pub.streamCalls, 1)
	})

	t.Run("success_sets_default_limit_and_builds_account_result", func(t *testing.T) {
		lastStatusAt := time.Now()
		repo := &fakeSearchRepo{
			searchAccountsResp: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "https://example.com/@alice"}, PreferredUsername: "alice", LastStatusAt: &lastStatusAt},
			},
		}
		relRepo := &fakeRelationshipRepo{followersCount: 10, followingCount: 20}
		statusRepo := &fakeStatusRepo{statusesByAuthor: 3}
		pub := &fakePublisher{}
		svc := NewService(repo, &fakeActorRepo{}, relRepo, statusRepo, &fakeHashtagRepo{}, pub, zap.NewNop(), "example.com")

		q := &Query{Query: "ali", Type: "accounts", Limit: 0}
		result, err := svc.Search(ctx, q)
		require.NoError(t, err)
		assert.Equal(t, 20, q.Limit)
		require.Len(t, result.Accounts, 1)
		assert.Equal(t, 10, result.Accounts[0].FollowersCount)
		assert.Equal(t, 20, result.Accounts[0].FollowingCount)
		assert.Equal(t, 3, result.Accounts[0].StatusesCount)
		assert.True(t, result.Accounts[0].IsLocal)
		assert.NotEmpty(t, result.Accounts[0].LastStatusAt)
	})

	t.Run("statuses_uses_counts_when_id_available", func(t *testing.T) {
		repo := &fakeSearchRepo{
			searchStatusesResp: []*storage.StatusSearchResult{
				{ID: "s1"},
				{ID: "s2"},
			},
		}
		statusRepo := &fakeStatusRepo{
			statusCountsByID: map[string][3]int{
				"s1": {1, 2, 3},
				"s2": {4, 5, 6},
			},
		}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, statusRepo, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		result, err := svc.Search(ctx, &Query{Query: "cats", Type: "statuses", Limit: 2})
		require.NoError(t, err)
		require.Len(t, result.Statuses, 2)
		assert.Equal(t, 1, result.Statuses[0].LikesCount)
		assert.Equal(t, []string{"s1", "s2"}, statusRepo.calls)
	})

	t.Run("hashtags_strips_prefix_and_checks_following", func(t *testing.T) {
		repo := &fakeSearchRepo{
			searchHashtagsResp: []*storage.Hashtag{
				{Name: "go", UsageCount: 5, Accounts: 2},
				{Name: "cats", UsageCount: 10, Accounts: 4},
			},
		}
		hashtagRepo := &fakeHashtagRepo{
			followingByUserHashtag: map[string]bool{"u1#go": true, "u1#cats": false},
		}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, hashtagRepo, nil, zap.NewNop(), "example.com")

		result, err := svc.Search(ctx, &Query{Query: "#go", Type: "hashtags", AccountID: "u1", Limit: 10})
		require.NoError(t, err)
		require.Len(t, repo.searchHashtagsCalls, 1)
		assert.Equal(t, "go", repo.searchHashtagsCalls[0].query)
		require.Len(t, result.Hashtags, 2)
		assert.True(t, result.Hashtags[0].History[0].Uses >= result.Hashtags[1].History[0].Uses)
	})

	t.Run("hashtags_following_check_error_is_non_fatal", func(t *testing.T) {
		repo := &fakeSearchRepo{
			searchHashtagsResp: []*storage.Hashtag{{Name: "go", UsageCount: 5, Accounts: 2}},
		}
		hashtagRepo := &fakeHashtagRepo{
			errByUserHashtag: map[string]error{"u1#go": stderrors.New("boom")},
		}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, hashtagRepo, nil, zap.NewNop(), "example.com")

		result, err := svc.Search(ctx, &Query{Query: "#go", Type: "hashtags", AccountID: "u1", Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Hashtags, 1)
		assert.False(t, result.Hashtags[0].Following)
	})
}

func TestService_GetDirectory_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("error_is_wrapped", func(t *testing.T) {
		repo := &fakeSearchRepo{searchAccountsErr: stderrors.New("boom")}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		_, err := svc.GetDirectory(ctx, &DirectoryQuery{})
		require.ErrorIs(t, err, pkgerrors.ErrGetDirectory)
	})

	t.Run("local_filter_and_default_limit", func(t *testing.T) {
		repo := &fakeSearchRepo{
			searchAccountsResp: []*activitypub.Actor{
				{BaseObject: activitypub.BaseObject{ID: "https://example.com/@alice"}, PreferredUsername: "alice"},
				{BaseObject: activitypub.BaseObject{ID: "https://remote/@bob"}, PreferredUsername: "bob"},
			},
		}
		svc := NewService(repo, &fakeActorRepo{}, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		q := &DirectoryQuery{Local: true, Limit: 0, Offset: 5}
		result, err := svc.GetDirectory(ctx, q)
		require.NoError(t, err)
		require.Len(t, repo.searchAccountsCalls, 1)
		assert.Equal(t, 80, repo.searchAccountsCalls[0].limit) // limit*2
		require.Len(t, result.Accounts, 1)
		assert.Equal(t, "alice", result.Accounts[0].Actor.PreferredUsername)
	})
}

func TestService_SuggestionsAndRemoval_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("GetSuggestions_error_is_wrapped", func(t *testing.T) {
		actorRepo := &fakeActorRepo{getSuggestionsErr: stderrors.New("boom")}
		svc := NewService(&fakeSearchRepo{}, actorRepo, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		_, err := svc.GetSuggestions(ctx, &SuggestionsQuery{Username: "alice"})
		require.ErrorIs(t, err, pkgerrors.ErrGetSuggestions)
	})

	t.Run("GetSuggestions_v2_sets_source", func(t *testing.T) {
		actorRepo := &fakeActorRepo{getSuggestionsResp: []*activitypub.Actor{{BaseObject: activitypub.BaseObject{ID: "https://example.com/@bob"}, PreferredUsername: "bob"}}}
		svc := NewService(&fakeSearchRepo{}, actorRepo, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		result, err := svc.GetSuggestions(ctx, &SuggestionsQuery{Username: "alice", Version: 2, Limit: 0})
		require.NoError(t, err)
		require.Len(t, result.Suggestions, 1)
		assert.Equal(t, "global", result.Suggestions[0].Source)
		require.Len(t, actorRepo.getSuggestionsCalls, 1)
		assert.Equal(t, 40, actorRepo.getSuggestionsCalls[0].limit)
	})

	t.Run("RemoveSuggestion_error_is_wrapped", func(t *testing.T) {
		actorRepo := &fakeActorRepo{removeErr: stderrors.New("boom")}
		svc := NewService(&fakeSearchRepo{}, actorRepo, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, nil, zap.NewNop(), "example.com")

		err := svc.RemoveSuggestion(ctx, &RemoveSuggestionCommand{Username: "alice", AccountID: "bob"})
		require.ErrorIs(t, err, pkgerrors.ErrRemoveSuggestion)
	})

	t.Run("RemoveSuggestion_publishes_event", func(t *testing.T) {
		actorRepo := &fakeActorRepo{}
		pub := &fakePublisher{err: stderrors.New("ignored")}
		svc := NewService(&fakeSearchRepo{}, actorRepo, &fakeRelationshipRepo{}, &fakeStatusRepo{}, &fakeHashtagRepo{}, pub, zap.NewNop(), "example.com")

		err := svc.RemoveSuggestion(ctx, &RemoveSuggestionCommand{Username: "alice", AccountID: "bob"})
		require.NoError(t, err)
		require.Len(t, actorRepo.removeCalls, 1)
		require.Len(t, pub.userCalls, 1)
		assert.Equal(t, "alice", pub.userCalls[0].user)
	})
}
