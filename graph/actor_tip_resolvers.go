package graph

import (
	"context"
	neturl "net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

type resolvedTipsConfig struct {
	enabled         bool
	chainID         int
	contractAddress string
}

func (r *actorResolver) effectiveTipsConfig(ctx context.Context) resolvedTipsConfig {
	if r == nil || r.Config == nil {
		return resolvedTipsConfig{}
	}

	out := resolvedTipsConfig{
		enabled:         r.Config.TipEnabled,
		chainID:         r.Config.TipChainID,
		contractAddress: strings.TrimSpace(r.Config.TipContractAddress),
	}

	if r.Storage == nil || r.Storage.Instance() == nil {
		return out
	}

	exists, err := r.Storage.Instance().TipsConfigExists(ctx)
	if err != nil || !exists {
		return out
	}

	effective, err := r.Storage.Instance().EffectiveTipsConfig(ctx)
	if err != nil || effective == nil {
		return out
	}

	out.enabled = effective.Enabled
	out.chainID = effective.ChainID
	out.contractAddress = strings.TrimSpace(effective.ContractAddress)
	return out
}

func (r *actorResolver) actorIDLocalToInstance(id string) bool {
	if id == "" || !strings.Contains(id, "://") {
		return true
	}

	parsed, err := neturl.Parse(id)
	if err != nil || parsed.Host == "" {
		return true
	}

	return r != nil && r.Config != nil && strings.EqualFold(parsed.Host, r.Config.Domain)
}

func bestEthereumWalletAddress(wallets []*storage.WalletCredential) string {
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

	return bestAddress
}

func (r *actorResolver) TipAddress(ctx context.Context, obj *activitypub.Actor) (*string, error) {
	if obj == nil || r.Config == nil || r.Storage == nil || r.Storage.Account() == nil {
		return nil, nil
	}

	// Tips are instance-scoped: only expose a tip recipient when tipping is configured/enabled.
	config := r.effectiveTipsConfig(ctx)
	if !config.enabled || config.chainID == 0 || config.contractAddress == "" {
		return nil, nil
	}

	// Only local actors have instance-managed tip recipients.
	if !r.actorIDLocalToInstance(obj.ID) {
		return nil, nil
	}

	username := strings.TrimSpace(obj.PreferredUsername)
	if username == "" {
		return nil, nil
	}
	if !r.canViewActorTipAddress(ctx, obj, username) {
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

	bestAddress := bestEthereumWalletAddress(wallets)
	if bestAddress == "" {
		return nil, nil
	}
	return &bestAddress, nil
}

func (r *actorResolver) canViewActorTipAddress(ctx context.Context, obj *activitypub.Actor, username string) bool {
	viewerUsername := strings.TrimSpace(r.optionalAuth(ctx))
	if viewerUsername == "" {
		return false
	}
	if r.isAdmin(ctx, viewerUsername) {
		return true
	}

	localUsername := r.localUsernameForLookup(obj.ID)
	if localUsername == "" && strings.TrimSpace(obj.ID) == "" {
		localUsername = strings.TrimSpace(username)
	}

	return localUsername != "" && strings.EqualFold(localUsername, viewerUsername)
}

func (r *actorResolver) TipChainID(ctx context.Context, _ *activitypub.Actor) (*int, error) {
	if r.Config == nil {
		return nil, nil
	}

	config := r.effectiveTipsConfig(ctx)
	if !config.enabled || config.chainID == 0 || config.contractAddress == "" {
		return nil, nil
	}

	cid := config.chainID
	return &cid, nil
}
