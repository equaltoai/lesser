package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/services/search"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// QUERY RESOLVERS
// ====================================================================

// Viewer returns the current authenticated user's actor.
func (r *queryResolver) Viewer(ctx context.Context) (*activitypub.Actor, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	account, err := r.Registry.Accounts().GetAccount(ctx, username)
	if err != nil {
		r.Logger.Error("Failed to get viewer account", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get viewer account"), err)
	}

	return r.convertAccountToActor(account), nil
}

// Actor is the resolver for the actor field.
func (r *queryResolver) Actor(ctx context.Context, id *string, username *string) (*activitypub.Actor, error) {
	// Determine lookup method
	var query *accounts.GetAccountQuery
	if id != nil {
		if err := common.ValidateRequiredParam("id", *id); err != nil {
			return nil, err
		}
		query = &accounts.GetAccountQuery{
			Username: *id,
		}
	} else if username != nil {
		if err := common.ValidateRequiredParam("username", *username); err != nil {
			return nil, err
		}
		query = &accounts.GetAccountQuery{
			Username: *username,
		}
	} else {
		return nil, ErrEitherIDOrUsernameRequired
	}

	// Get account using service
	usernameToLookup := ""
	if err := common.ValidateRequiredParam("username", query.Username); err == nil {
		usernameToLookup = query.Username
	}

	account, err := r.Registry.Accounts().GetAccount(ctx, usernameToLookup)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.Logger.Error("Failed to get actor", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get actor"), err)
	}

	return r.convertAccountToActor(account), nil
}

// AccountQuotePermissions resolves quote permissions for the requested username.
func (r *queryResolver) AccountQuotePermissions(ctx context.Context, username string) (*model.AccountQuotePermissions, error) {
	resolved := deriveUsernameFromIRI(username)
	if err := common.ValidateRequiredParam("username", resolved); err != nil {
		return nil, err
	}

	storage := r.Registry.GetStorage()
	if storage == nil || storage.Quote() == nil {
		return nil, errors.New("quote repository is not available")
	}

	perms, err := storage.Quote().GetQuotePermissions(ctx, resolved)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			perms = &storageModels.QuotePermissions{
				Username: resolved,
			}
			perms.SetDefaults()
			_ = perms.UpdateKeys()
		} else {
			r.Logger.Error("Failed to get quote permissions",
				zap.String("username", resolved),
				zap.Error(err))
			return nil, errors.Join(errors.New("failed to get quote permissions"), err)
		}
	}

	blockList := perms.BlockList
	if blockList == nil {
		blockList = []string{}
	}

	return &model.AccountQuotePermissions{
		Username:       resolved,
		AllowPublic:    perms.AllowPublic,
		AllowFollowers: perms.AllowFollowers,
		AllowMentioned: perms.AllowMentioned,
		BlockList:      blockList,
	}, nil
}

// CustomEmojis is the resolver for the customEmojis field.
func (r *queryResolver) CustomEmojis(ctx context.Context) ([]*model.CustomEmoji, error) {
	result, err := r.Registry.Emoji().ListEmojis(ctx, &emoji.ListEmojisQuery{
		OnlyVisible: true,
	})
	if err != nil {
		r.Logger.Error("Failed to list emojis", zap.Error(err))
		return nil, errors.Join(errors.New("failed to list emojis"), err)
	}

	emojis := make([]*model.CustomEmoji, len(result.Emojis))
	for i, e := range result.Emojis {
		emojis[i] = r.convertEmojiToGraphQL(e)
	}

	return emojis, nil
}

// ProfileDirectory is the resolver for the profileDirectory field.
func (r *queryResolver) ProfileDirectory(ctx context.Context, filters *model.DirectoryFiltersInput, first *int, after *model.Cursor) (*model.ProfileDirectory, error) {
	query := &search.DirectoryQuery{
		Local:  true,
		Order:  "active",
		Limit:  40,
		Offset: 0,
	}

	if filters != nil {
		if filters.Local != nil {
			query.Local = *filters.Local
		}
		if filters.Order != nil {
			query.Order = string(*filters.Order)
		}
	}

	if first != nil && *first > 0 && *first <= 100 {
		query.Limit = *first
	}

	if after != nil {
		// Convert cursor to offset (simplified)
		query.Offset = 20
	}

	result, err := r.Registry.Search().GetDirectory(ctx, query)
	if err != nil {
		r.Logger.Error("Failed to get directory", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get directory"), err)
	}

	actors := make([]*activitypub.Actor, len(result.Accounts))
	for i, account := range result.Accounts {
		actors[i] = account.Actor
	}

	return &model.ProfileDirectory{
		Accounts:   actors,
		TotalCount: len(result.Accounts),
	}, nil
}

// Suggestions is the resolver for the suggestions field.
func (r *queryResolver) Suggestions(ctx context.Context, limit *int) ([]*model.AccountSuggestion, error) {
	username := r.optionalAuth(ctx)

	maxLimit := 40
	if limit != nil {
		limitStr := fmt.Sprintf("%d", *limit)
		if parsedLimit, err := common.ParseAdminLimit(limitStr); err == nil {
			maxLimit = parsedLimit
		}
	}

	result, err := r.Registry.Search().GetSuggestions(ctx, &search.SuggestionsQuery{
		Username: username,
		Limit:    maxLimit,
	})
	if err != nil {
		r.Logger.Error("Failed to get suggestions", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get suggestions"), err)
	}

	suggestions := make([]*model.AccountSuggestion, len(result.Suggestions))
	for i, s := range result.Suggestions {
		source := s.Source
		suggestions[i] = &model.AccountSuggestion{
			Account: s.Account.Actor,
			Source:  r.convertSuggestionSource(s.Source),
			Reason:  &source, // Use source as reason
		}
	}

	return suggestions, nil
}

// RemoveSuggestion is the resolver for the removeSuggestion field.
func (r *queryResolver) RemoveSuggestion(ctx context.Context, accountID string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	err = r.Registry.Search().RemoveSuggestion(ctx, &search.RemoveSuggestionCommand{
		Username:  username,
		AccountID: accountID,
	})
	if err != nil {
		r.Logger.Error("Failed to remove suggestion",
			zap.String("user", username),
			zap.String("account", accountID),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to remove suggestion"), err)
	}

	return true, nil
}

// Endorsements returns the accounts pinned by the current viewer.
func (r *queryResolver) Endorsements(ctx context.Context) ([]*activitypub.Actor, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Accounts().GetAccountPins(ctx, &accounts.GetAccountPinsQuery{
		Username: username,
	})
	if err != nil {
		r.Logger.Error("Failed to load endorsements",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load endorsements"), err)
	}

	actors := make([]*activitypub.Actor, 0, len(result.PinnedAccounts))
	for _, account := range result.PinnedAccounts {
		actor := r.convertAccountToActor(account)
		if actor != nil {
			actors = append(actors, actor)
		}
	}

	return actors, nil
}
