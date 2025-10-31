package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"go.uber.org/zap"
)

// TimelineTypeList is referenced from constants.go
var _ = TimelineTypeList

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// CreateList is the resolver for the createList field.
func (r *mutationResolver) CreateList(ctx context.Context, input model.CreateListInput) (*model.List, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	cmd := &lists.CreateListCommand{
		Username:  username,
		Title:     input.Title,
		CreatorID: username,
	}

	if input.RepliesPolicy != nil {
		// Convert GraphQL enum (FOLLOWED, LIST, NONE) to lowercase (followed, list, none)
		// as expected by validation
		cmd.RepliesPolicy = strings.ToLower(string(*input.RepliesPolicy))
	} else {
		cmd.RepliesPolicy = TimelineTypeList // Default to "list" (lowercase)
	}

	// Note: Exclusive field doesn't exist in CreateListCommand

	result, err := r.Registry.Lists().CreateList(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to create list",
			zap.String("user", username),
			zap.String("title", input.Title),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create list"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertListToGraphQL(ctx, result.List), nil
}

// UpdateList is the resolver for the updateList field.
func (r *mutationResolver) UpdateList(ctx context.Context, id string, input model.UpdateListInput) (*model.List, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	cmd := &lists.UpdateListCommand{
		ListID:    id,
		UpdaterID: username,
	}

	if input.Title != nil {
		cmd.Title = *input.Title
	}
	if input.RepliesPolicy != nil {
		// Convert GraphQL enum (FOLLOWED, LIST, NONE) to lowercase (followed, list, none)
		// as expected by validation
		cmd.RepliesPolicy = strings.ToLower(string(*input.RepliesPolicy))
	}
	// Note: Exclusive field doesn't exist in UpdateListCommand

	result, err := r.Registry.Lists().UpdateList(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to update list",
			zap.String("user", username),
			zap.String("list", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update list"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertListToGraphQL(ctx, result.List), nil
}

// DeleteList is the resolver for the deleteList field.
func (r *mutationResolver) DeleteList(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	err = r.Registry.Lists().DeleteList(ctx, &lists.DeleteListCommand{
		ListID:    id,
		DeleterID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to delete list",
			zap.String("user", username),
			zap.String("list", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete list"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// AddAccountsToList is the resolver for the addAccountsToList field.
func (r *mutationResolver) AddAccountsToList(ctx context.Context, id string, accountIDs []string) (*model.List, error) {
	return r.executeListMembershipOperation(ctx, id, accountIDs, "add", func(ctx context.Context, listID, accountID, username string) (*lists.MembershipResult, error) {
		return r.Registry.Lists().AddToList(ctx, &lists.AddToListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			AdderID:        username,
		})
	})
}

// RemoveAccountsFromList is the resolver for the removeAccountsFromList field.
func (r *mutationResolver) RemoveAccountsFromList(ctx context.Context, id string, accountIDs []string) (*model.List, error) {
	return r.executeListMembershipOperation(ctx, id, accountIDs, "remove", func(ctx context.Context, listID, accountID, username string) (*lists.MembershipResult, error) {
		return r.Registry.Lists().RemoveFromList(ctx, &lists.RemoveFromListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			RemoverID:      username,
		})
	})
}
