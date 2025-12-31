package emoji

import (
	"context"
	stderrors "errors"
	"testing"

	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeEmojiRepo struct {
	customByShortcode map[string]*storage.CustomEmoji
	customErrByCode   map[string]error

	allEmojis    []*storage.CustomEmoji
	allEmojisErr error

	createErr error
	updateErr error
	deleteErr error

	remoteByKey   map[string]*storage.CustomEmoji
	remoteErrByKey map[string]error

	searchCalls []fakeEmojiSearchCall
	searchResp  []*storage.CustomEmoji
	searchErr   error

	popularCalls []fakeEmojiPopularCall
	popularResp  []*storage.CustomEmoji
	popularErr   error

	incrementCalls []string
	incrementErrByCode map[string]error
}

type fakeEmojiSearchCall struct {
	query string
	limit int
}

type fakeEmojiPopularCall struct {
	domain string
	limit  int
}

func (r *fakeEmojiRepo) GetCustomEmoji(_ context.Context, shortcode string) (*storage.CustomEmoji, error) {
	if err := r.customErrByCode[shortcode]; err != nil {
		return nil, err
	}
	if r.customByShortcode == nil {
		return nil, storage.ErrNotFound
	}
	emoji, ok := r.customByShortcode[shortcode]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return emoji, nil
}

func (r *fakeEmojiRepo) GetCustomEmojis(context.Context) ([]*storage.CustomEmoji, error) {
	if r.allEmojisErr != nil {
		return nil, r.allEmojisErr
	}
	return r.allEmojis, nil
}

func (r *fakeEmojiRepo) CreateCustomEmoji(_ context.Context, emoji *storage.CustomEmoji) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.customByShortcode == nil {
		r.customByShortcode = map[string]*storage.CustomEmoji{}
	}
	r.customByShortcode[emoji.Shortcode] = emoji
	return nil
}

func (r *fakeEmojiRepo) UpdateCustomEmoji(_ context.Context, emoji *storage.CustomEmoji) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.customByShortcode == nil {
		r.customByShortcode = map[string]*storage.CustomEmoji{}
	}
	r.customByShortcode[emoji.Shortcode] = emoji
	return nil
}

func (r *fakeEmojiRepo) DeleteCustomEmoji(_ context.Context, shortcode string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if r.customByShortcode != nil {
		delete(r.customByShortcode, shortcode)
	}
	return nil
}

func (r *fakeEmojiRepo) GetRemoteEmoji(_ context.Context, shortcode, domain string) (*storage.CustomEmoji, error) {
	key := domain + "#" + shortcode
	if err := r.remoteErrByKey[key]; err != nil {
		return nil, err
	}
	emoji, ok := r.remoteByKey[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return emoji, nil
}

func (r *fakeEmojiRepo) SearchEmojis(_ context.Context, query string, limit int) ([]*storage.CustomEmoji, error) {
	r.searchCalls = append(r.searchCalls, fakeEmojiSearchCall{query: query, limit: limit})
	if r.searchErr != nil {
		return nil, r.searchErr
	}
	return r.searchResp, nil
}

func (r *fakeEmojiRepo) GetPopularEmojis(_ context.Context, domain string, limit int) ([]*storage.CustomEmoji, error) {
	r.popularCalls = append(r.popularCalls, fakeEmojiPopularCall{domain: domain, limit: limit})
	if r.popularErr != nil {
		return nil, r.popularErr
	}
	return r.popularResp, nil
}

func (r *fakeEmojiRepo) IncrementEmojiUsage(_ context.Context, shortcode string) error {
	r.incrementCalls = append(r.incrementCalls, shortcode)
	if err := r.incrementErrByCode[shortcode]; err != nil {
		return err
	}
	return nil
}

func TestService_CRUD_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("GetEmoji_not_found", func(t *testing.T) {
		svc := NewService(&fakeEmojiRepo{}, nil, zap.NewNop(), "example.com")
		_, err := svc.GetEmoji(ctx, &GetEmojiQuery{Shortcode: "missing"})
		require.ErrorIs(t, err, pkgerrors.ErrEmojiNotFound)
	})

	t.Run("GetEmoji_other_error", func(t *testing.T) {
		svc := NewService(&fakeEmojiRepo{customErrByCode: map[string]error{"x": stderrors.New("boom")}}, nil, zap.NewNop(), "example.com")
		_, err := svc.GetEmoji(ctx, &GetEmojiQuery{Shortcode: "x"})
		require.ErrorIs(t, err, pkgerrors.ErrGetEmoji)
	})

	t.Run("ListEmojis_error", func(t *testing.T) {
		svc := NewService(&fakeEmojiRepo{allEmojisErr: stderrors.New("boom")}, nil, zap.NewNop(), "example.com")
		_, err := svc.ListEmojis(ctx, &ListEmojisQuery{})
		require.ErrorIs(t, err, pkgerrors.ErrListEmojis)
	})

	t.Run("ListEmojis_filters", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			allEmojis: []*storage.CustomEmoji{
				{Shortcode: "local", Domain: "", VisibleInPicker: true, Disabled: false, Category: "fun"},
				{Shortcode: "remote", Domain: "remote.example", VisibleInPicker: true, Disabled: false, Category: "fun"},
				{Shortcode: "disabled", Domain: "", VisibleInPicker: true, Disabled: true, Category: "fun"},
			},
		}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		result, err := svc.ListEmojis(ctx, &ListEmojisQuery{OnlyLocal: true, OnlyVisible: true})
		require.NoError(t, err)
		require.Len(t, result.Emojis, 1)
		assert.Equal(t, "local", result.Emojis[0].Shortcode)
	})

	t.Run("CreateEmoji_invalid_shortcode", func(t *testing.T) {
		svc := NewService(&fakeEmojiRepo{}, nil, zap.NewNop(), "example.com")
		_, err := svc.CreateEmoji(ctx, &CreateEmojiCommand{Shortcode: "a", ImageURL: "https://x"})
		require.ErrorIs(t, err, pkgerrors.ErrInvalidShortcode)
	})

	t.Run("CreateEmoji_already_exists", func(t *testing.T) {
		repo := &fakeEmojiRepo{customByShortcode: map[string]*storage.CustomEmoji{"party": {Shortcode: "party"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.CreateEmoji(ctx, &CreateEmojiCommand{Shortcode: "party", ImageURL: "https://x"})
		require.ErrorIs(t, err, pkgerrors.ErrEmojiAlreadyExists)
	})

	t.Run("CreateEmoji_repo_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{createErr: stderrors.New("boom")}
		svc := NewService(repo, nopPublisher{}, zap.NewNop(), "example.com")

		_, err := svc.CreateEmoji(ctx, &CreateEmojiCommand{Shortcode: "party", ImageURL: "https://x"})
		require.ErrorIs(t, err, pkgerrors.ErrCreateEmoji)
	})

	t.Run("CreateEmoji_success", func(t *testing.T) {
		repo := &fakeEmojiRepo{}
		svc := NewService(repo, nopPublisher{}, zap.NewNop(), "example.com")

		result, err := svc.CreateEmoji(ctx, &CreateEmojiCommand{
			Shortcode:       "party",
			ImageURL:        "https://example.com/party.png",
			Category:        "fun",
			VisibleInPicker: true,
		})
		require.NoError(t, err)
		require.NotNil(t, result.Emoji)
		assert.Equal(t, "party", result.Emoji.Shortcode)
		assert.Equal(t, "", result.Emoji.Domain)
		require.Len(t, result.Events, 1)
	})

	t.Run("UpdateEmoji_not_found", func(t *testing.T) {
		svc := NewService(&fakeEmojiRepo{}, nil, zap.NewNop(), "example.com")
		_, err := svc.UpdateEmoji(ctx, &UpdateEmojiCommand{Shortcode: "missing"})
		require.ErrorIs(t, err, pkgerrors.ErrEmojiNotFound)
	})

	t.Run("UpdateEmoji_remote_cannot_update", func(t *testing.T) {
		repo := &fakeEmojiRepo{customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x", Domain: "remote"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.UpdateEmoji(ctx, &UpdateEmojiCommand{Shortcode: "x"})
		require.ErrorIs(t, err, pkgerrors.ErrCannotUpdateRemoteEmoji)
	})

	t.Run("UpdateEmoji_no_changes", func(t *testing.T) {
		existing := &storage.CustomEmoji{Shortcode: "x", Domain: ""}
		repo := &fakeEmojiRepo{customByShortcode: map[string]*storage.CustomEmoji{"x": existing}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		result, err := svc.UpdateEmoji(ctx, &UpdateEmojiCommand{Shortcode: "x"})
		require.NoError(t, err)
		require.Same(t, existing, result.Emoji)
		assert.Nil(t, result.Events)
	})

	t.Run("UpdateEmoji_repo_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x", Domain: ""}},
			updateErr:         stderrors.New("boom"),
		}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		visible := true
		_, err := svc.UpdateEmoji(ctx, &UpdateEmojiCommand{Shortcode: "x", VisibleInPicker: &visible})
		require.ErrorIs(t, err, pkgerrors.ErrUpdateEmoji)
	})

	t.Run("UpdateEmoji_success", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x", Domain: ""}},
		}
		svc := NewService(repo, nopPublisher{}, zap.NewNop(), "example.com")

		category := "mods"
		disabled := true
		result, err := svc.UpdateEmoji(ctx, &UpdateEmojiCommand{Shortcode: "x", Category: &category, Disabled: &disabled})
		require.NoError(t, err)
		require.NotNil(t, result.Emoji)
		assert.Equal(t, "mods", result.Emoji.Category)
		assert.True(t, result.Emoji.Disabled)
		require.Len(t, result.Events, 1)
	})

	t.Run("DeleteEmoji_remote_cannot_delete", func(t *testing.T) {
		repo := &fakeEmojiRepo{customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x", Domain: "remote"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		err := svc.DeleteEmoji(ctx, &DeleteEmojiCommand{Shortcode: "x"})
		require.ErrorIs(t, err, pkgerrors.ErrCannotDeleteRemoteEmoji)
	})

	t.Run("DeleteEmoji_repo_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x", Domain: ""}},
			deleteErr:         stderrors.New("boom"),
		}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		err := svc.DeleteEmoji(ctx, &DeleteEmojiCommand{Shortcode: "x"})
		require.ErrorIs(t, err, pkgerrors.ErrDeleteEmoji)
	})

	t.Run("DeleteEmoji_success", func(t *testing.T) {
		repo := &fakeEmojiRepo{customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x", Domain: ""}}}
		svc := NewService(repo, nopPublisher{}, zap.NewNop(), "example.com")

		require.NoError(t, svc.DeleteEmoji(ctx, &DeleteEmojiCommand{Shortcode: "x"}))
	})
}

func TestService_CopySearchPopularIncrement_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("CopyRemoteEmoji_not_found", func(t *testing.T) {
		repo := &fakeEmojiRepo{}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.CopyRemoteEmoji(ctx, &CopyEmojiCommand{Shortcode: "party", Domain: "remote.example"})
		require.ErrorIs(t, err, pkgerrors.ErrRemoteEmojiNotFound)
	})

	t.Run("CopyRemoteEmoji_create_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			remoteByKey: map[string]*storage.CustomEmoji{
				"remote.example#party": {Shortcode: "party", Domain: "remote.example", URL: "https://remote/party.png", StaticURL: "https://remote/party.png", Category: "fun"},
			},
			createErr: stderrors.New("boom"),
		}
		svc := NewService(repo, nopPublisher{}, zap.NewNop(), "example.com")

		_, err := svc.CopyRemoteEmoji(ctx, &CopyEmojiCommand{Shortcode: "party", Domain: "remote.example"})
		require.ErrorIs(t, err, pkgerrors.ErrCreateLocalEmojiCopy)
	})

	t.Run("CopyRemoteEmoji_success_default_shortcode", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			remoteByKey: map[string]*storage.CustomEmoji{
				"remote.example#party": {Shortcode: "party", Domain: "remote.example", URL: "https://remote/party.png", StaticURL: "https://remote/party.png", Category: "fun"},
			},
		}
		svc := NewService(repo, nopPublisher{}, zap.NewNop(), "example.com")

		result, err := svc.CopyRemoteEmoji(ctx, &CopyEmojiCommand{Shortcode: "party", Domain: "remote.example"})
		require.NoError(t, err)
		require.NotNil(t, result.Emoji)
		assert.Equal(t, "party", result.Emoji.Shortcode)
		assert.Equal(t, "", result.Emoji.Domain)
		require.Len(t, result.Events, 1)
	})

	t.Run("SearchEmojis_default_and_cap_limit", func(t *testing.T) {
		repo := &fakeEmojiRepo{searchResp: []*storage.CustomEmoji{{Shortcode: "x"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.SearchEmojis(ctx, &SearchEmojisQuery{Query: "x"})
		require.NoError(t, err)
		require.Len(t, repo.searchCalls, 1)
		assert.Equal(t, 20, repo.searchCalls[0].limit)

		_, err = svc.SearchEmojis(ctx, &SearchEmojisQuery{Query: "x", Limit: 1000})
		require.NoError(t, err)
		require.Len(t, repo.searchCalls, 2)
		assert.Equal(t, 100, repo.searchCalls[1].limit)
	})

	t.Run("SearchEmojis_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{searchErr: stderrors.New("boom")}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.SearchEmojis(ctx, &SearchEmojisQuery{Query: "x", Limit: 1})
		require.ErrorIs(t, err, pkgerrors.ErrSearchEmojis)
	})

	t.Run("GetPopularEmojis_default_and_cap_limit", func(t *testing.T) {
		repo := &fakeEmojiRepo{popularResp: []*storage.CustomEmoji{{Shortcode: "x"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.GetPopularEmojis(ctx, &GetPopularEmojisQuery{})
		require.NoError(t, err)
		require.Len(t, repo.popularCalls, 1)
		assert.Equal(t, 20, repo.popularCalls[0].limit)

		_, err = svc.GetPopularEmojis(ctx, &GetPopularEmojisQuery{Limit: 1000})
		require.NoError(t, err)
		require.Len(t, repo.popularCalls, 2)
		assert.Equal(t, 100, repo.popularCalls[1].limit)
	})

	t.Run("GetPopularEmojis_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{popularErr: stderrors.New("boom")}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		_, err := svc.GetPopularEmojis(ctx, &GetPopularEmojisQuery{Limit: 1})
		require.ErrorIs(t, err, pkgerrors.ErrGetPopularEmojis)
	})

	t.Run("IncrementUsage_not_found_is_noop", func(t *testing.T) {
		repo := &fakeEmojiRepo{incrementErrByCode: map[string]error{"x": storage.ErrNotFound}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		require.NoError(t, svc.IncrementUsage(ctx, &IncrementUsageCommand{Shortcode: "x"}))
	})

	t.Run("IncrementUsage_other_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{incrementErrByCode: map[string]error{"x": stderrors.New("boom")}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")

		err := svc.IncrementUsage(ctx, &IncrementUsageCommand{Shortcode: "x"})
		require.ErrorIs(t, err, pkgerrors.ErrIncrementEmojiUsage)
	})

	t.Run("IncrementUsage_success", func(t *testing.T) {
		repo := &fakeEmojiRepo{}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		require.NoError(t, svc.IncrementUsage(ctx, &IncrementUsageCommand{Shortcode: "x"}))
		assert.Equal(t, []string{"x"}, repo.incrementCalls)
	})

	t.Run("NewService_nil_logger_defaults", func(t *testing.T) {
		repo := &fakeEmojiRepo{}
		svc := NewService(repo, nil, nil, "example.com")
		require.NotNil(t, svc)
		require.NotNil(t, svc.logger)
	})

	t.Run("UpdateEmoji_get_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{customErrByCode: map[string]error{"x": stderrors.New("boom")}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		_, err := svc.UpdateEmoji(ctx, &UpdateEmojiCommand{Shortcode: "x"})
		require.ErrorIs(t, err, pkgerrors.ErrGetEmoji)
	})

	t.Run("DeleteEmoji_not_found", func(t *testing.T) {
		svc := NewService(&fakeEmojiRepo{}, nil, zap.NewNop(), "example.com")
		err := svc.DeleteEmoji(ctx, &DeleteEmojiCommand{Shortcode: "missing"})
		require.ErrorIs(t, err, pkgerrors.ErrEmojiNotFound)
	})

	t.Run("CopyRemoteEmoji_get_error", func(t *testing.T) {
		repo := &fakeEmojiRepo{remoteErrByKey: map[string]error{"remote.example#party": stderrors.New("boom")}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		_, err := svc.CopyRemoteEmoji(ctx, &CopyEmojiCommand{Shortcode: "party", Domain: "remote.example"})
		require.ErrorIs(t, err, pkgerrors.ErrGetRemoteEmoji)
	})

	t.Run("CopyRemoteEmoji_existing_local_shortcode", func(t *testing.T) {
		repo := &fakeEmojiRepo{
			remoteByKey:       map[string]*storage.CustomEmoji{"remote.example#party": {Shortcode: "party", Domain: "remote.example", URL: "https://remote/party.png", StaticURL: "https://remote/party.png"}},
			customByShortcode: map[string]*storage.CustomEmoji{"party": {Shortcode: "party"}},
		}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		_, err := svc.CopyRemoteEmoji(ctx, &CopyEmojiCommand{Shortcode: "party", Domain: "remote.example"})
		require.ErrorIs(t, err, pkgerrors.ErrEmojiAlreadyExists)
	})

	t.Run("CopyRemoteEmoji_invalid_new_shortcode", func(t *testing.T) {
		repo := &fakeEmojiRepo{remoteByKey: map[string]*storage.CustomEmoji{"remote.example#party": {Shortcode: "party", Domain: "remote.example", URL: "https://remote/party.png", StaticURL: "https://remote/party.png"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		_, err := svc.CopyRemoteEmoji(ctx, &CopyEmojiCommand{Shortcode: "party", Domain: "remote.example", NewShortcode: "a"})
		require.ErrorIs(t, err, pkgerrors.ErrInvalidShortcode)
	})

	t.Run("GetEmoji_success", func(t *testing.T) {
		repo := &fakeEmojiRepo{customByShortcode: map[string]*storage.CustomEmoji{"x": {Shortcode: "x"}}}
		svc := NewService(repo, nil, zap.NewNop(), "example.com")
		emoji, err := svc.GetEmoji(ctx, &GetEmojiQuery{Shortcode: "x"})
		require.NoError(t, err)
		assert.Equal(t, "x", emoji.Shortcode)
	})
}
