package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

const (
	agentMaxTagsPerPost = 5
	agentMaxPostChars   = 500

	agentRapidFireMaxPosts = 10
	agentRapidFireWindow   = 30 * time.Second

	agentRepetitionMax    = 5
	agentRepetitionWindow = 30 * time.Minute

	agentDefaultMaxPostsPerHour         = 50
	agentVerifiedDefaultMaxPostsPerHour = 200
	agentLockoutDuration                = 10 * time.Minute
)

func (h *Handler) enforceAgentStatusCreateRails(ctx *apptheory.Context, claims *auth.Claims, req *models.CreateStatusRequest) (*apptheory.Response, error) {
	if h == nil || ctx == nil || claims == nil || req == nil || !claims.IsAgent {
		return nil, nil
	}
	if h.repos == nil || h.repos.User() == nil {
		return apptheory.JSON(http.StatusServiceUnavailable, map[string]any{
			"error":             "service_unavailable",
			"error_description": "storage is not initialized",
		})
	}

	uniqueTags := uniqueHashtags(req.Status)
	if len(uniqueTags) > agentMaxTagsPerPost {
		return common.RespondValidationError(ctx, common.ValidationError{
			Field:   "status",
			Message: "agents may include at most 5 hashtags per post",
		})
	}

	if utf8.RuneCountInString(req.Status) > agentMaxPostChars {
		return common.RespondValidationError(ctx, common.ValidationError{
			Field:   "status",
			Message: "agents may include at most 500 characters per post",
		})
	}

	agentUser, _ := h.repos.User().GetUser(ctx.Context(), claims.Username)
	if agentUser == nil || !agentUser.IsAgent {
		return apptheory.JSON(http.StatusForbidden, map[string]any{
			"error":             "agent_required",
			"error_description": "agent safety rails are only available for agent accounts",
		})
	}
	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, claims.Username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
	}

	if quarantined, until := agentQuarantineActive(governance); quarantined {
		if strings.TrimSpace(req.Visibility) != VisibilityPrivate {
			h.imposeAgentLockout(ctx.Context(), claims.Username, "quarantine_violation")
			return apptheory.JSON(http.StatusForbidden, map[string]any{
				"error":             "agent_quarantined",
				"error_description": "agent is quarantined: only followers-only (private) posts are allowed",
				"quarantine_until":  until.Format(time.RFC3339),
			})
		}
	}

	if h.repos != nil && h.repos.RateLimit() != nil {
		// Enforce per-hour posting cap (capability-driven, conservative default).
		maxPerHour := h.agentMaxPostsPerHourLimit(ctx.Context(), agentUser, governance)
		if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_posts_per_hour", maxPerHour, time.Hour); err != nil {
			return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
				"error":             "too_many_requests",
				"error_description": "agent post limit exceeded",
			})
		}

		// Rapid-fire breaker: sustained bursts beyond the short testing envelope trigger a temporary lockout.
		if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_posts_10s", agentRapidFireMaxPosts, agentRapidFireWindow); err != nil {
			h.imposeAgentLockout(ctx.Context(), claims.Username, "rapid_fire")
			return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
				"error":             "agent_locked",
				"error_description": "agent locked due to rapid posting",
			})
		}

		// Repetition breaker: repeated identical content within the short window triggers a temporary lockout.
		hash := hashAgentContent(req.Status)
		if hash != "" {
			if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_repeat:"+hash, agentRepetitionMax, agentRepetitionWindow); err != nil {
				h.imposeAgentLockout(ctx.Context(), claims.Username, "repetition")
				return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
					"error":             "agent_locked",
					"error_description": "agent locked due to repeated content",
				})
			}
		}
	}

	return nil, nil
}

func (h *Handler) agentMaxPostsPerHourLimit(ctx context.Context, user *storage.User, governance *storage.AgentGovernanceState) int {
	limit := agentDefaultMaxPostsPerHour
	verifiedLimit := agentVerifiedDefaultMaxPostsPerHour

	if h != nil && h.repos != nil && h.repos.Instance() != nil {
		if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx); err == nil && policy != nil {
			if policy.AgentMaxPostsPerHour > 0 {
				limit = policy.AgentMaxPostsPerHour
			}
			if policy.VerifiedAgentMaxPostsPerHour > 0 {
				verifiedLimit = policy.VerifiedAgentMaxPostsPerHour
			}
		}
	}

	allowed := limit
	if agentVerifiedState(governance) {
		allowed = verifiedLimit
	}

	if user != nil && user.AgentCapabilities != nil && user.AgentCapabilities.MaxPostsPerHour > 0 {
		requested := user.AgentCapabilities.MaxPostsPerHour
		if requested < allowed {
			return requested
		}
	}

	return allowed
}

func uniqueHashtags(content string) []string {
	hashtags := mastodon.ExtractHashtagsWithCase(content)
	if len(hashtags) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(hashtags))
	out := make([]string, 0, len(hashtags))
	for _, tag := range hashtags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}

	return out
}

func agentQuarantineActive(governance *storage.AgentGovernanceState) (bool, time.Time) {
	if governance == nil {
		return false, time.Time{}
	}
	return governance.QuarantineActiveAt(time.Now().UTC())
}

func hashAgentContent(content string) string {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return ""
	}
	normalized = strings.Join(strings.Fields(normalized), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) imposeAgentLockout(ctx context.Context, username string, _ string) {
	if h == nil || h.repos == nil || h.repos.RateLimit() == nil {
		return
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return
	}

	_ = h.repos.RateLimit().ImposeLockout(ctx, agentLockoutIdentifier(username), agentLockoutDuration)
}

func agentLockoutIdentifier(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "agent:unknown"
	}
	return agentRateLimitUserID(username)
}

func agentRateLimitUserID(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "agent:unknown"
	}
	return "agent:" + strings.ToLower(username)
}

func agentRateLimitUserIDVariants(username string) []string {
	normalized := agentRateLimitUserID(username)
	rawUsername := strings.TrimSpace(username)
	if rawUsername == "" {
		return []string{normalized}
	}

	legacy := "agent:" + rawUsername
	if legacy == normalized {
		return []string{normalized}
	}

	return []string{normalized, legacy}
}
