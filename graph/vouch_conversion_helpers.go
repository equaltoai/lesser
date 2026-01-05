package graph

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/reputation"
)

func (r *Resolver) convertVouchToGraphQL(ctx context.Context, vouch *reputation.Vouch) *model.Vouch {
	if vouch == nil {
		return nil
	}

	fromActor := r.resolveActorByID(ctx, vouch.From)
	if fromActor == nil {
		return nil
	}

	toActor := r.resolveActorByID(ctx, vouch.To)
	if toActor == nil {
		return nil
	}

	createdAt := vouch.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	expiresAt := vouch.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = createdAt.Add(365 * 24 * time.Hour)
	}

	var revokedAt *model.Time
	if vouch.RevokedAt != nil && !vouch.RevokedAt.IsZero() {
		t := model.Time(*vouch.RevokedAt)
		revokedAt = &t
	}

	return &model.Vouch{
		ID:                vouch.ID,
		From:              fromActor,
		To:                toActor,
		Confidence:        vouch.Confidence,
		Context:           vouch.Context,
		VoucherReputation: vouch.VoucherReputation,
		CreatedAt:         model.Time(createdAt),
		ExpiresAt:         model.Time(expiresAt),
		Active:            vouch.Active,
		Revoked:           vouch.Revoked,
		RevokedAt:         revokedAt,
	}
}
