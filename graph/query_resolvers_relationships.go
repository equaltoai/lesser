package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
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

	relationship, err := r.Registry.Relationships().GetRelationship(ctx, username, id)
	if err != nil {
		r.Logger.Error("Failed to get relationship",
			zap.String("user", username),
			zap.String("target", id),
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
			return "all"
		}()))

	// Validate inputs
	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		return nil, ErrActorIDRequired
	}

	trustRepo := r.Registry.GetStorage().Trust()

	// Get relationships where this actor is trusted (incoming trust)
	incomingRels, _, err := trustRepo.GetTrustedByRelationships(ctx, actorID, 100, "")
	if err != nil && err != storage.ErrNotFound {
		r.Logger.Error("Failed to get incoming trust relationships", zap.Error(err))
		return nil, errors.Join(errors.New("failed to fetch trust relationships"), err)
	}

	// Get relationships where this actor trusts others (outgoing trust)
	outgoingRels, _, err := trustRepo.GetTrustRelationships(ctx, actorID, 100, "")
	if err != nil && err != storage.ErrNotFound {
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
