package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Lists is the resolver for the lists field.
func (r *queryResolver) Lists(ctx context.Context) ([]*model.List, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Lists().ListUserLists(ctx, &lists.ListUserListsQuery{
		Username: username,
		ViewerID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to get lists",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get lists"), err)
	}

	gqlLists := make([]*model.List, len(result.Lists))
	for i, list := range result.Lists {
		gqlLists[i] = r.convertListToGraphQL(ctx, list)
	}

	return gqlLists, nil
}

// List is the resolver for the list field.
func (r *queryResolver) List(ctx context.Context, id string) (*model.List, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	list, err := r.Registry.Lists().GetList(ctx, &lists.GetListQuery{
		ListID:   id,
		ViewerID: username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.Logger.Error("Failed to get list",
			zap.String("user", username),
			zap.String("id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get list"), err)
	}

	return r.convertListToGraphQL(ctx, list), nil
}

// ListAccounts is the resolver for the listAccounts field.
func (r *queryResolver) ListAccounts(ctx context.Context, id string) ([]*activitypub.Actor, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Lists().GetListMembers(ctx, &lists.GetListMembersQuery{
		ViewerID: username,
		ListID:   id,
	})
	if err != nil {
		r.Logger.Error("Failed to get list members",
			zap.String("user", username),
			zap.String("listId", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get list members"), err)
	}

	actors := make([]*activitypub.Actor, len(result.Members))
	for i, member := range result.Members {
		actors[i] = r.convertAccountToActor(member)
	}

	return actors, nil
}
