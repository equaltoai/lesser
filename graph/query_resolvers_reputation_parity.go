package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/reputation"
	"go.uber.org/zap"
)

func convertReputationToGraphQL(rep *reputation.Reputation) *model.Reputation {
	if rep == nil {
		return nil
	}

	var signature *string
	if strings.TrimSpace(rep.Signature) != "" {
		signature = &rep.Signature
	}

	return &model.Reputation{
		ActorID:         rep.ActorID,
		Instance:        rep.InstanceURL,
		TotalScore:      rep.TotalScore,
		TrustScore:      rep.TrustScore,
		ActivityScore:   rep.ActivityScore,
		ModerationScore: rep.ModerationScore,
		CommunityScore:  rep.CommunityScore,
		CalculatedAt:    model.Time(rep.CalculatedAt),
		Version:         rep.Version,
		Evidence: &model.ReputationEvidence{
			TotalPosts:        rep.TotalPosts,
			TotalFollowers:    rep.TotalFollowers,
			AccountAge:        rep.AccountAge,
			VouchCount:        rep.VouchCount,
			TrustingActors:    rep.TrustingActors,
			AverageTrustScore: rep.AverageTrustScore,
		},
		Signature: signature,
	}
}

// Reputation covers GET /api/v1/reputation/{actor_id}.
func (r *queryResolver) Reputation(ctx context.Context, actorID string) (*model.Reputation, error) {
	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	actorID = strings.TrimSpace(actorID)
	if err := common.ValidateRequiredParam("actorId", actorID); err != nil {
		return nil, err
	}

	if r.Config != nil && !strings.Contains(actorID, "://") {
		actorID = fmt.Sprintf("https://%s/users/%s", r.Config.Domain, actorID)
	}

	service, err := r.getReputationService()
	if err != nil {
		return nil, err
	}

	rep, err := service.GetReputation(ctx, actorID)
	if err != nil {
		r.Logger.Error("failed to get reputation", zap.String("actor_id", actorID), zap.Error(err))
		return nil, errors.Join(errors.New("failed to get reputation"), err)
	}

	return convertReputationToGraphQL(rep), nil
}

// Vouches covers GET /api/v1/vouches/{actor_id}.
func (r *queryResolver) Vouches(ctx context.Context, actorID string) ([]*model.Vouch, error) {
	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	actorID = strings.TrimSpace(actorID)
	if err := common.ValidateRequiredParam("actorId", actorID); err != nil {
		return nil, err
	}

	if r.Config != nil && !strings.Contains(actorID, "://") {
		actorID = fmt.Sprintf("https://%s/users/%s", r.Config.Domain, actorID)
	}

	service, err := r.getReputationService()
	if err != nil {
		return nil, err
	}

	vouches, err := service.GetVouches(ctx, actorID)
	if err != nil {
		r.Logger.Error("failed to get vouches", zap.String("actor_id", actorID), zap.Error(err))
		return nil, errors.Join(errors.New("failed to get vouches"), err)
	}

	results := make([]*model.Vouch, 0, len(vouches))
	for _, vouch := range vouches {
		converted := r.convertVouchToGraphQL(ctx, &vouch)
		if converted != nil {
			results = append(results, converted)
		}
	}

	return results, nil
}
