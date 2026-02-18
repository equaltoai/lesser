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
	enabled := r.Config.TipEnabled
	chainID := r.Config.TipChainID
	contractAddress := strings.TrimSpace(r.Config.TipContractAddress)

	if r.Storage.Instance() != nil {
		exists, err := r.Storage.Instance().TipsConfigExists(ctx)
		if err == nil && exists {
			effective, err := r.Storage.Instance().EffectiveTipsConfig(ctx)
			if err == nil && effective != nil {
				enabled = effective.Enabled
				chainID = effective.ChainID
				contractAddress = strings.TrimSpace(effective.ContractAddress)
			}
		}
	}

	if !enabled || chainID == 0 || contractAddress == "" {
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

func (r *actorResolver) TipChainID(ctx context.Context, _ *activitypub.Actor) (*int, error) {
	if r.Config == nil {
		return nil, nil
	}

	enabled := r.Config.TipEnabled
	chainID := r.Config.TipChainID
	contractAddress := strings.TrimSpace(r.Config.TipContractAddress)

	if r.Storage != nil && r.Storage.Instance() != nil {
		exists, err := r.Storage.Instance().TipsConfigExists(ctx)
		if err == nil && exists {
			effective, err := r.Storage.Instance().EffectiveTipsConfig(ctx)
			if err == nil && effective != nil {
				enabled = effective.Enabled
				chainID = effective.ChainID
				contractAddress = strings.TrimSpace(effective.ContractAddress)
			}
		}
	}

	if !enabled || chainID == 0 || contractAddress == "" {
		return nil, nil
	}

	cid := chainID
	return &cid, nil
}
