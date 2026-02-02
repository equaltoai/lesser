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
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

const (
	agentMaxTagsPerPost = 5
	agentMaxPostChars   = 500

	agentRapidFireMaxPosts = 5
	agentRapidFireWindow   = 10 * time.Second

	agentRepetitionMax    = 3
	agentRepetitionWindow = time.Hour

	agentDefaultMaxPostsPerHour = 50
	agentLockoutDuration        = time.Hour
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

	if quarantined, until := agentQuarantineActive(agentUser); quarantined {
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
		maxPerHour := agentDefaultMaxPostsPerHour
		if agentUser.AgentCapabilities != nil && agentUser.AgentCapabilities.MaxPostsPerHour > 0 {
			maxPerHour = agentUser.AgentCapabilities.MaxPostsPerHour
		}
		if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), "agent:"+claims.Username, "agent_posts_per_hour", maxPerHour, time.Hour); err != nil {
			return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
				"error":             "too_many_requests",
				"error_description": "agent post limit exceeded",
			})
		}

		// Rapid-fire breaker: >5 posts in 10s => lockout.
		if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), "agent:"+claims.Username, "agent_posts_10s", agentRapidFireMaxPosts, agentRapidFireWindow); err != nil {
			h.imposeAgentLockout(ctx.Context(), claims.Username, "rapid_fire")
			return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
				"error":             "agent_locked",
				"error_description": "agent locked due to rapid posting",
			})
		}

		// Repetition breaker: identical content >3 times => lockout.
		hash := hashAgentContent(req.Status)
		if hash != "" {
			if err := h.repos.RateLimit().CheckAPIRateLimit(ctx.Context(), "agent:"+claims.Username, "agent_repeat:"+hash, agentRepetitionMax, agentRepetitionWindow); err != nil {
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

func agentQuarantineActive(user *storage.User) (bool, time.Time) {
	if user == nil || !user.IsAgent {
		return false, time.Time{}
	}

	status, _ := agentMetadataString(user, "agent_quarantine_status")
	if strings.EqualFold(status, "approved") {
		return false, time.Time{}
	}

	endStr, ok := agentMetadataString(user, "agent_quarantine_end")
	if !ok || strings.TrimSpace(endStr) == "" {
		return false, time.Time{}
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return false, time.Time{}
	}

	if time.Now().Before(end) {
		return true, end
	}

	return false, end
}

func agentMetadataString(user *storage.User, key string) (string, bool) {
	if user == nil || user.Metadata == nil {
		return "", false
	}
	raw, ok := user.Metadata[key]
	if !ok || raw == nil {
		return "", false
	}
	if s, ok := raw.(string); ok {
		return s, true
	}
	return "", false
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
	return "agent:" + strings.ToLower(username)
}
