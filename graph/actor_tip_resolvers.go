package graph

import (
	"context"
	neturl "net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

func (r *actorResolver) TipAddress(ctx context.Context, obj *activitypub.Actor) (*string, error) {
	if obj == nil || r.Config == nil || r.Storage == nil || r.Storage.Account() == nil {
		return nil, nil
	}

	// Tips are instance-scoped: only expose a tip recipient when tipping is configured/enabled.
	if !r.Config.TipEnabled || r.Config.TipChainID == 0 || strings.TrimSpace(r.Config.TipContractAddress) == "" {
		return nil, nil
	}

	// Only local actors have instance-managed tip recipients.
	if obj.ID != "" && strings.Contains(obj.ID, "://") {
		parsed, err := neturl.Parse(obj.ID)
		if err == nil && parsed.Host != "" && !strings.EqualFold(parsed.Host, r.Config.Domain) {
			return nil, nil
		}
	}

	username := strings.TrimSpace(obj.PreferredUsername)
	if username == "" {
		return nil, nil
	}

	wallets, err := r.Storage.Account().GetUserWallets(ctx, username)
	if err != nil {
		if r.Logger != nil {
			r.Logger.Warn("failed to resolve tip address from wallets",
				zap.String("username", username),
				zap.Error(err))
		}
		return nil, nil
	}

	var bestAddress string
	var bestLastUsed int64
	for _, w := range wallets {
		if w == nil {
			continue
		}
		if w.Type != "" && w.Type != "ethereum" {
			continue
		}
		addr := strings.TrimSpace(w.Address)
		if addr == "" {
			continue
		}
		lastUsed := w.LastUsed.UnixNano()
		if bestAddress == "" || lastUsed > bestLastUsed {
			bestAddress = addr
			bestLastUsed = lastUsed
		}
	}

	if bestAddress == "" {
		return nil, nil
	}
	return &bestAddress, nil
}

func (r *actorResolver) TipChainID(_ context.Context, _ *activitypub.Actor) (*int, error) {
	if r.Config == nil {
		return nil, nil
	}
	if !r.Config.TipEnabled || r.Config.TipChainID == 0 || strings.TrimSpace(r.Config.TipContractAddress) == "" {
		return nil, nil
	}
	chainID := r.Config.TipChainID
	return &chainID, nil
}
