package handlers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

const actorUsersPathSegment = "users"

func (h *Handler) shouldHideRemoteAgentActor(ctx context.Context, actorID string) bool {
	if h == nil || h.cfg == nil || h.repos == nil || h.repos.Instance() == nil {
		return false
	}

	// Remote agent policy is part of the agent feature surface; keep it inert unless agents are enabled.
	if !h.cfg.AllowAgents {
		return false
	}

	policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx)
	if err != nil || policy == nil || !policy.AllowAgents {
		return false
	}

	domain := extractDomainFromActorID(actorID)
	if domain == "" || isLocalDomain(domain, h.cfg.Domain) {
		return false
	}

	if domainInList(domain, policy.TrustedAgentDomains) {
		return false
	}
	if domainInList(domain, policy.BlockedAgentDomains) {
		return true
	}

	if policy.AllowRemoteAgents {
		return h.remoteAgentQuarantineActive(ctx, actorID, policy)
	}

	return true
}

func (h *Handler) remoteAgentQuarantineActive(ctx context.Context, actorID string, policy *storageModels.AgentInstanceConfig) bool {
	if h == nil || h.repos == nil || h.repos.GetDB() == nil || policy == nil {
		return false
	}
	if policy.RemoteQuarantineDays <= 0 {
		return false
	}

	handle := extractHandleFromActorID(actorID)
	if handle == "" {
		// Conservative default: without a stable handle we cannot look up first-seen time.
		return true
	}

	pk := fmt.Sprintf("REMOTE_ACTOR#%s", handle)
	var cached storageModels.RemoteActor
	err := h.repos.GetDB().WithContext(ctx).
		Model(&storageModels.RemoteActor{}).
		Where("PK", "=", pk).
		Where("SK", "=", "PROFILE").
		First(&cached)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return true
		}
		// If cache retrieval fails, avoid enforcing quarantine to prevent outages from hiding content.
		return false
	}

	if !cached.ExpiresAt.IsZero() && time.Now().After(cached.ExpiresAt) {
		return true
	}

	firstSeen := cached.CachedAt
	if firstSeen.IsZero() {
		firstSeen = cached.UpdatedAt
	}
	if firstSeen.IsZero() {
		return true
	}

	quarantineUntil := firstSeen.Add(time.Duration(policy.RemoteQuarantineDays) * 24 * time.Hour)
	return time.Now().Before(quarantineUntil)
}

func extractDomainFromActorID(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}

	parsed, err := url.Parse(actorID)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func extractHandleFromActorID(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}

	parsed, err := url.Parse(actorID)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return ""
	}

	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}

	// Common AP patterns:
	// - /users/<username>
	// - /@<username>
	segments := strings.Split(path, "/")
	for idx, seg := range segments {
		if seg == actorUsersPathSegment && idx+1 < len(segments) {
			username := strings.TrimSpace(segments[idx+1])
			username = strings.TrimPrefix(username, "@")
			if username != "" {
				return fmt.Sprintf("%s@%s", username, host)
			}
		}
	}

	for _, seg := range segments {
		if strings.HasPrefix(seg, "@") {
			username := strings.TrimSpace(strings.TrimPrefix(seg, "@"))
			if username != "" {
				return fmt.Sprintf("%s@%s", username, host)
			}
		}
	}

	// Fallback to last segment.
	username := strings.TrimSpace(segments[len(segments)-1])
	username = strings.TrimPrefix(username, "@")
	if username == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", username, host)
}

func isLocalDomain(candidate string, local string) bool {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	local = strings.ToLower(strings.TrimSpace(local))
	if candidate == "" || local == "" {
		return false
	}
	return candidate == local
}

func domainInList(domain string, domains []string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || len(domains) == 0 {
		return false
	}

	for _, entry := range domains {
		if strings.ToLower(strings.TrimSpace(entry)) == domain {
			return true
		}
	}
	return false
}
