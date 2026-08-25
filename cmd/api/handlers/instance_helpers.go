package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

func (h *Handler) instanceLocked(ctx context.Context) bool {
	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return false
	}

	state, err := h.repos.Instance().GetInstanceState(ctx)
	if err != nil || state == nil {
		return true
	}
	return state.Locked
}

func (h *Handler) instanceRules(ctx context.Context) []storage.InstanceRule {
	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return []storage.InstanceRule{}
	}

	rules, err := h.repos.Instance().GetInstanceRules(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to get instance rules", zap.Error(err))
		}
		return []storage.InstanceRule{}
	}
	return rules
}

func (h *Handler) instanceExtendedDescription(ctx context.Context) string {
	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return ""
	}

	extendedDescription, _, err := h.repos.Instance().GetExtendedDescription(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to get extended description", zap.Error(err))
		}
		return ""
	}
	return extendedDescription
}

func (h *Handler) resolveVAPIDPublicKey(ctx *apptheory.Context, generateIfMissing bool) (string, *apptheory.Response, error) {
	if h == nil || h.repos == nil || h.repos.PushSubscription() == nil {
		return "", nil, nil
	}

	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context())
	if err != nil {
		env := ""
		if h.cfg != nil {
			env = h.cfg.Stage
		}
		if naming.IsLiveEnvironment(env) {
			if h.logger != nil {
				h.logger.Error("VAPID keys are required in production but not found", zap.Error(err))
			}
			resp, respErr := common.RespondInternalServerError(ctx, "VAPID keys not configured - push notifications unavailable")
			return "", resp, respErr
		}

		if !generateIfMissing {
			if h.logger != nil {
				h.logger.Warn("failed to get VAPID keys", zap.Error(err))
			}
			return "", nil, nil
		}

		if h.logger != nil {
			h.logger.Info("VAPID keys not found in non-production environment, generating new keys")
		}
		vapidKeys, err = h.generateAndStoreVAPIDKeys(ctx.Context())
		if err != nil {
			if h.logger != nil {
				h.logger.Error("failed to generate VAPID keys", zap.Error(err))
			}
			return "", nil, nil
		}
	}

	return vapidKeys.PublicKey, nil, nil
}

func (h *Handler) instanceCounts(ctx context.Context) (int, int64, int64) {
	if h == nil || h.repos == nil || h.repos.Analytics() == nil || h.repos.Instance() == nil {
		return 0, 0, 0
	}

	userCount, err := h.repos.Analytics().GetTotalUserCount(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to get user count", zap.Error(err))
		}
		userCount = 0
	}

	statusCount, err := h.repos.Instance().GetTotalStatusCount(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to get status count", zap.Error(err))
		}
		statusCount = 0
	}

	domainCount, err := h.repos.Instance().GetTotalDomainCount(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to get domain count", zap.Error(err))
		}
		domainCount = 0
	}

	return userCount, statusCount, domainCount
}

func (h *Handler) instanceContactAccount(ctx context.Context) map[string]any {
	if h == nil || h.cfg == nil || h.repos == nil || h.repos.Instance() == nil {
		return nil
	}

	adminActor, err := h.repos.Instance().GetContactAccount(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to get contact account", zap.Error(err))
		}
		return nil
	}
	if adminActor == nil {
		return nil
	}

	return map[string]any{
		"id":              adminActor.ID,
		"username":        adminActor.Username,
		"acct":            adminActor.Username,
		"display_name":    adminActor.DisplayName,
		"locked":          false, // ActorRecord doesn't have this field
		"bot":             false, // ActorRecord doesn't have Bot field
		"discoverable":    true,  // ActorRecord doesn't have this field
		"group":           adminActor.ActorType == actorTypeGroup,
		"created_at":      adminActor.CreatedAt.Format(time.RFC3339),
		"note":            "", // ActorRecord doesn't have summary
		"url":             fmt.Sprintf("https://%s/@%s", h.cfg.Domain, adminActor.Username),
		"uri":             adminActor.ID,
		"avatar":          adminActor.Avatar,
		"avatar_static":   adminActor.Avatar,
		"header":          "", // ActorRecord doesn't have header
		"header_static":   "", // ActorRecord doesn't have header
		"followers_count": h.getAccountFollowersCountLift(ctx, adminActor.Username),
		"following_count": h.getAccountFollowingCountLift(ctx, adminActor.Username),
		"statuses_count":  h.getAccountStatusesCountLift(ctx, adminActor.Username),
		"emojis":          []any{},
		"fields":          []any{}, // ActorRecord doesn't have fields
	}
}

func (h *Handler) instanceTipsConfig(ctx context.Context) map[string]any {
	enabled := h != nil && h.cfg != nil && h.cfg.TipEnabled
	chainID := 0
	contractAddress := ""

	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return map[string]any{"enabled": false}
	}

	exists, err := h.repos.Instance().TipsConfigExists(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("failed to check persisted tips config; falling back to legacy config", zap.Error(err))
		}
	} else if exists {
		effective, err := h.repos.Instance().EffectiveTipsConfig(ctx)
		if err != nil {
			if h.logger != nil {
				h.logger.Warn("failed to resolve effective tips config; falling back to legacy config", zap.Error(err))
			}
		} else if effective != nil {
			enabled = effective.Enabled
			chainID = effective.ChainID
			contractAddress = strings.TrimSpace(effective.ContractAddress)
		}
	}

	if !exists {
		h.warnLegacyTipsConfig()
	}

	if !exists && h.cfg != nil {
		chainID = h.cfg.TipChainID
		contractAddress = strings.TrimSpace(h.cfg.TipContractAddress)
	}

	if enabled && (chainID == 0 || contractAddress == "") {
		if h.logger != nil {
			h.logger.Warn("tips enabled but missing chain ID or contract address; disabling tips in instance config",
				zap.Int("chain_id", chainID),
				zap.String("contract_address", contractAddress))
		}
		enabled = false
	}

	out := map[string]any{
		"enabled": enabled,
	}
	if enabled {
		out["chain_id"] = chainID
		out["contract_address"] = contractAddress
	}

	return out
}
