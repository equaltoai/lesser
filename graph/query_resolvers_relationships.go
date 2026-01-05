package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Relationship is the resolver for the relationship field.
func (r *queryResolver) Relationship(ctx context.Context, id string) (*model.Relationship, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	targetID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", targetID); err != nil {
		return nil, err
	}

	relationship, err := r.Registry.Relationships().GetRelationship(ctx, username, targetID)
	if err != nil {
		var validationErr common.ValidationError
		if errors.As(err, &validationErr) && validationErr.Field == "target_id" {
			return nil, common.ValidationError{
				Field:   "id",
				Message: validationErr.Message,
			}
		}

		r.Logger.Error("Failed to get relationship",
			zap.String("user", username),
			zap.String("target", targetID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get relationship"), err)
	}

	return r.convertRelationshipToGraphQL(relationship), nil
}

// Relationships is the resolver for the relationships field.
func (r *queryResolver) Relationships(ctx context.Context, ids []string) ([]*model.Relationship, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	rels := make([]*model.Relationship, len(ids))
	for i, id := range ids {
		relationship, err := r.Registry.Relationships().GetRelationship(ctx, username, id)
		if err != nil {
			rels[i] = &model.Relationship{
				ID:                  id,
				Following:           false,
				FollowedBy:          false,
				Blocking:            false,
				BlockedBy:           false,
				Muting:              false,
				MutingNotifications: false,
				Requested:           false,
				DomainBlocking:      false,
				ShowingReblogs:      true,
				Notifying:           false,
			}
			continue
		}
		rels[i] = r.convertRelationshipToGraphQL(relationship)
	}

	return rels, nil
}

// Blocks returns the accounts blocked by the current viewer.
func (r *queryResolver) Blocks(ctx context.Context, first *int, after *model.Cursor) (*model.ActorListPage, error) {
	service := r.Registry.Relationships()
	if service == nil {
		return nil, errors.New("relationships service is not available")
	}

	return r.resolveViewerActorListPage(ctx, first, after, "blocks", func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
		result, err := service.GetBlockedUsers(ctx, &relationships.GetBlockedUsersQuery{
			UserID: username,
			Limit:  limit,
			Cursor: cursor,
		})
		if err != nil {
			return nil, "", err
		}
		return result.BlockedUsers, result.NextCursor, nil
	})
}

// Mutes returns the accounts muted by the current viewer.
func (r *queryResolver) Mutes(ctx context.Context, first *int, after *model.Cursor) (*model.ActorListPage, error) {
	service := r.Registry.Relationships()
	if service == nil {
		return nil, errors.New("relationships service is not available")
	}

	return r.resolveViewerActorListPage(ctx, first, after, "mutes", func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
		result, err := service.GetMutedUsers(ctx, &relationships.GetMutedUsersQuery{
			UserID: username,
			Limit:  limit,
			Cursor: cursor,
		})
		if err != nil {
			return nil, "", err
		}
		return result.MutedUsers, result.NextCursor, nil
	})
}

type viewerActorListFetcher func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error)

func (r *queryResolver) resolveViewerActorListPage(ctx context.Context, first *int, after *model.Cursor, label string, fetch viewerActorListFetcher) (*model.ActorListPage, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if fetch == nil {
		return nil, errors.New("relationships service is not available")
	}

	limit := clampLimit(first)
	cursor := cursorToString(after)

	accounts, nextCursor, err := fetch(ctx, username, limit, cursor)
	if err != nil {
		r.Logger.Error("Failed to list viewer actors",
			zap.String("type", label),
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(fmt.Errorf("failed to list %s", label), err)
	}

	actors := make([]*activitypub.Actor, 0, len(accounts))
	for _, account := range accounts {
		actor := r.convertAccountToActor(account)
		if actor != nil {
			actors = append(actors, actor)
		}
	}

	return &model.ActorListPage{
		Actors:     actors,
		NextCursor: stringToCursor(nextCursor),
		TotalCount: len(actors),
	}, nil
}

// FollowRequests returns pending follow requests for the current viewer (locked accounts only).
func (r *queryResolver) FollowRequests(ctx context.Context, first *int, after *model.Cursor) (*model.ActorListPage, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	service := r.Registry.Relationships()
	if service == nil {
		return nil, errors.New("relationships service is not available")
	}

	limit := 100
	if first != nil && *first > 0 {
		limit = *first
		if limit > 200 {
			limit = 200
		}
	}

	cursor := cursorToString(after)

	result, err := service.GetPendingFollowRequests(ctx, &relationships.GetFollowRequestsQuery{
		UserID: username,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		r.Logger.Error("Failed to list follow requests",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list follow requests"), err)
	}

	repoStorage := r.Registry.GetStorage()
	var actorRepo interface {
		GetActor(context.Context, string) (*activitypub.Actor, error)
	}
	if repoStorage != nil && repoStorage.Actor() != nil {
		actorRepo = repoStorage.Actor()
	}

	actors := make([]*activitypub.Actor, 0, len(result.FollowerIDs))
	for _, followerID := range result.FollowerIDs {
		if err := common.ValidateRequiredParam("follower_id", followerID); err != nil {
			continue
		}

		account, err := r.Registry.Accounts().GetAccount(ctx, followerID)
		if err != nil || account == nil {
			// Fallback to actor repository (remote or partial data)
			if actorRepo != nil {
				actor, actorErr := actorRepo.GetActor(ctx, followerID)
				if actorErr == nil && actor != nil {
					actors = append(actors, actor)
				}
			}
			continue
		}

		actor := r.convertAccountToActor(account)
		if actor != nil {
			actors = append(actors, actor)
		}
	}

	return &model.ActorListPage{
		Actors:     actors,
		NextCursor: stringToCursor(result.NextCursor),
		TotalCount: len(actors),
	}, nil
}

// DomainBlocks returns the domains blocked by the current viewer.
func (r *queryResolver) DomainBlocks(ctx context.Context, first *int, after *model.Cursor) (*model.DomainBlockPage, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	service := r.Registry.Relationships()
	if service == nil {
		return nil, errors.New("relationships service is not available")
	}

	limit := 100
	if first != nil && *first > 0 {
		limit = *first
		if limit > 200 {
			limit = 200
		}
	}

	cursor := cursorToString(after)

	result, err := service.GetDomainBlocks(ctx, &relationships.GetDomainBlocksQuery{
		UserID: username,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		r.Logger.Error("Failed to list domain blocks",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list domain blocks"), err)
	}

	domains := result.Domains
	if domains == nil {
		domains = []string{}
	}

	return &model.DomainBlockPage{
		Domains:    domains,
		NextCursor: stringToCursor(result.NextCursor),
		TotalCount: len(domains),
	}, nil
}

// Followers is the resolver for the followers field.
func (r *queryResolver) Followers(ctx context.Context, username string, limit *int, cursor *model.Cursor) (*model.ActorListPage, error) {
	service := r.Registry.Relationships()
	if service == nil {
		return nil, errors.New("relationships service is not available")
	}

	return r.resolveActorRelationshipPage(ctx, username, limit, cursor, service.GetFollowers, service.CountFollowers)
}

// Following is the resolver for the following field.
func (r *queryResolver) Following(ctx context.Context, username string, limit *int, cursor *model.Cursor) (*model.ActorListPage, error) {
	service := r.Registry.Relationships()
	if service == nil {
		return nil, errors.New("relationships service is not available")
	}

	return r.resolveActorRelationshipPage(ctx, username, limit, cursor, service.GetFollowing, service.CountFollowing)
}

type relationshipPageFetcher func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error)
type relationshipCountFetcher func(ctx context.Context, username string) (int64, error)

func (r *queryResolver) resolveActorRelationshipPage(
	ctx context.Context,
	username string,
	limit *int,
	cursor *model.Cursor,
	fetch relationshipPageFetcher,
	count relationshipCountFetcher,
) (*model.ActorListPage, error) {
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, err
	}

	if fetch == nil || count == nil {
		return nil, errors.New("relationships service is not available")
	}

	pageSize := clampLimit(limit)
	after := cursorToString(cursor)

	accounts, nextCursor, err := fetch(ctx, username, pageSize, after)
	if err != nil {
		r.Logger.Error("Failed to list relationships",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list relationships"), err)
	}

	actors := make([]*activitypub.Actor, 0, len(accounts))
	for _, account := range accounts {
		actor := r.convertAccountToActor(account)
		if actor != nil {
			actors = append(actors, actor)
		}
	}

	totalCount, err := count(ctx, username)
	if err != nil {
		r.Logger.Warn("Failed to count relationships",
			zap.String("username", username),
			zap.Error(err))
		totalCount = int64(len(actors))
	}

	return &model.ActorListPage{
		Actors:     actors,
		NextCursor: stringToCursor(nextCursor),
		TotalCount: int(totalCount),
	}, nil
}

func clampLimit(limit *int) int {
	pageSize := 40
	if limit != nil && *limit > 0 {
		if *limit > 80 {
			pageSize = 80
		} else {
			pageSize = *limit
		}
	}
	return pageSize
}

func cursorToString(cursor *model.Cursor) string {
	if cursor == nil {
		return ""
	}
	return string(*cursor)
}

func stringToCursor(value string) *model.Cursor {
	if value == "" {
		return nil
	}
	c := model.Cursor(value)
	return &c
}

// ====================================================================
// ADVANCED FEATURE RESOLVERS
// ====================================================================

// The following resolvers implement advanced features like trust graphs,
// moderation patterns, and community analytics

// TrustGraph is the resolver for the trustGraph field.
func (r *queryResolver) TrustGraph(ctx context.Context, actorID string, category *models.TrustCategory) ([]*trust.TrustEdge, error) {
	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	r.Logger.Info("Fetching trust graph",
		zap.String("actor", actorID),
		zap.String("category", func() string {
			if category != nil {
				return string(*category)
			}
			return QueryTypeAll
		}()))

	// Validate inputs
	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		return nil, ErrActorIDRequired
	}

	trustRepo := r.Registry.GetStorage().Trust()

	// Get relationships where this actor is trusted (incoming trust)
	incomingRels, _, err := trustRepo.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		r.Logger.Error("Failed to get incoming trust relationships", zap.Error(err))
		return nil, errors.Join(errors.New("failed to fetch trust relationships"), err)
	}

	// Get relationships where this actor trusts others (outgoing trust)
	outgoingRels, _, err := trustRepo.GetTrustRelationships(ctx, actorID, 100, "")
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		r.Logger.Error("Failed to get outgoing trust relationships", zap.Error(err))
		return nil, errors.Join(errors.New("failed to fetch trust relationships"), err)
	}

	// Combine all relationships
	allRels := append(incomingRels, outgoingRels...)

	// Filter by category if specified
	filteredRels := allRels
	if category != nil {
		filteredRels = make([]*storage.TrustRelationship, 0)
		for _, rel := range allRels {
			if rel.Category == storage.TrustCategory(*category) {
				filteredRels = append(filteredRels, rel)
			}
		}
	}

	// Convert to TrustEdge objects
	edges := make([]*trust.TrustEdge, 0, len(filteredRels))
	for _, rel := range filteredRels {
		edge := &trust.TrustEdge{
			From:       rel.TrusterID,
			To:         rel.TrusteeID,
			Category:   rel.Category,
			Score:      rel.Score,
			Confidence: rel.Confidence,
			Weight:     rel.Score * rel.Confidence,
		}
		edges = append(edges, edge)
	}

	r.Logger.Info("Trust graph fetched successfully",
		zap.String("actor", actorID),
		zap.Int("edge_count", len(edges)))

	return edges, nil
}
