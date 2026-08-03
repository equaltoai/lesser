package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

// HandleAdminGetAgentPolicyLift handles GET /api/v1/admin/agents/policy.
func (h *Handler) HandleAdminGetAgentPolicyLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	_, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	if h.repos == nil || h.repos.Instance() == nil {
		return common.RespondInternalServerError(ctx)
	}

	policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context())
	if err != nil || policy == nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentPolicyFromStorage(policy))
}

// HandleAdminUpdateAgentPolicyLift handles PUT /api/v1/admin/agents/policy.
func (h *Handler) HandleAdminUpdateAgentPolicyLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	if h.repos == nil || h.repos.Instance() == nil {
		return common.RespondInternalServerError(ctx)
	}

	var req apimodels.UpdateAdminAgentPolicyRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	if req.DefaultQuarantineDays < 0 || req.DefaultQuarantineDays > 365 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "default_quarantine_days", Message: "must be between 0 and 365"})
	}
	if req.MaxAgentsPerOwner < 0 || req.MaxAgentsPerOwner > 1000 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "max_agents_per_owner", Message: "must be between 0 and 1000"})
	}
	if req.RemoteQuarantineDays < 0 || req.RemoteQuarantineDays > 365 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "remote_quarantine_days", Message: "must be between 0 and 365"})
	}

	if req.AgentMaxPostsPerHour < 0 || req.AgentMaxPostsPerHour > 10000 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "agent_max_posts_per_hour", Message: "must be between 0 and 10000"})
	}
	if req.VerifiedAgentMaxPostsPerHour < 0 || req.VerifiedAgentMaxPostsPerHour > 10000 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "verified_agent_max_posts_per_hour", Message: "must be between 0 and 10000"})
	}
	if req.AgentMaxFollowsPerHour < 0 || req.AgentMaxFollowsPerHour > 10000 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "agent_max_follows_per_hour", Message: "must be between 0 and 10000"})
	}
	if req.VerifiedAgentMaxFollowsPerHour < 0 || req.VerifiedAgentMaxFollowsPerHour > 10000 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "verified_agent_max_follows_per_hour", Message: "must be between 0 and 10000"})
	}
	if req.HybridRetrievalMaxCandidates < 0 || req.HybridRetrievalMaxCandidates > 5000 {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "hybrid_retrieval_max_candidates", Message: "must be between 0 and 5000"})
	}

	normalizedBlocked := normalizeDomainList(req.BlockedAgentDomains)
	normalizedTrusted := normalizeDomainList(req.TrustedAgentDomains)

	cfg := &storageModels.AgentInstanceConfig{
		AllowAgents:                    req.AllowAgents,
		AllowAgentRegistration:         req.AllowAgentRegistration,
		DefaultQuarantineDays:          req.DefaultQuarantineDays,
		MaxAgentsPerOwner:              req.MaxAgentsPerOwner,
		AllowRemoteAgents:              req.AllowRemoteAgents,
		RemoteQuarantineDays:           req.RemoteQuarantineDays,
		BlockedAgentDomains:            normalizedBlocked,
		TrustedAgentDomains:            normalizedTrusted,
		AgentMaxPostsPerHour:           req.AgentMaxPostsPerHour,
		VerifiedAgentMaxPostsPerHour:   req.VerifiedAgentMaxPostsPerHour,
		AgentMaxFollowsPerHour:         req.AgentMaxFollowsPerHour,
		VerifiedAgentMaxFollowsPerHour: req.VerifiedAgentMaxFollowsPerHour,
		HybridRetrievalEnabled:         req.HybridRetrievalEnabled,
		HybridRetrievalMaxCandidates:   req.HybridRetrievalMaxCandidates,
		UpdatedAt:                      time.Now().UTC(),
	}

	if err := cfg.UpdateKeys(); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	if err := h.repos.Instance().SetAgentInstanceConfig(ctx.Context(), cfg); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	h.recordAgentGovernanceEvent(ctx, claims.Username, "agent.policy_updated", map[string]any{
		"allow_agents":             req.AllowAgents,
		"allow_agent_registration": req.AllowAgentRegistration,
	})

	return okJSON(agentPolicyFromStorage(cfg))
}

// HandleAdminVerifyAgentLift handles POST /api/v1/admin/agents/:username/verify.
func (h *Handler) HandleAdminVerifyAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	var req apimodels.AdminVerifyAgentRequest
	_ = common.ParseRequestWithFallback(ctx, &req)

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}
	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
	}

	now := time.Now().UTC()
	governance.Verified = true
	governance.VerifiedAt = cloneAgentGovernanceHandlerTime(&now)
	governance.VerifiedBy = claims.Username
	governance.VerifiedReason = ""
	governance.UnverifiedAt = nil
	governance.UnverifiedBy = ""
	governance.UnverifiedReason = ""
	if strings.TrimSpace(req.Reason) != "" {
		governance.VerifiedReason = strings.TrimSpace(req.Reason)
	}

	if req.ExitQuarantine {
		governance = applyAgentQuarantineExit(governance, claims, true, now)
	}
	governance.UpdatedAt = now

	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}
	if err := h.repos.Account().PutAgentGovernanceState(ctx.Context(), governance); err != nil {
		return respondAgentGovernanceWriteError(ctx, err)
	}

	h.recordAgentGovernanceEvent(ctx, username, "agent.verified", map[string]any{
		"verified_by": claims.Username,
		"reason":      strings.TrimSpace(req.Reason),
	})

	return okJSON(agentFromStorageUserWithBaseURL(account.User, governance, handlerBaseURL(h)))
}

// HandleAdminUnverifyAgentLift handles POST /api/v1/admin/agents/:username/unverify.
func (h *Handler) HandleAdminUnverifyAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	var req apimodels.AdminVerifyAgentRequest
	_ = common.ParseRequestWithFallback(ctx, &req)

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}
	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
	}

	now := time.Now().UTC()
	governance.Verified = false
	governance.VerifiedAt = nil
	governance.VerifiedBy = ""
	governance.VerifiedReason = ""
	governance.UnverifiedAt = cloneAgentGovernanceHandlerTime(&now)
	governance.UnverifiedBy = claims.Username
	governance.UnverifiedReason = ""
	if strings.TrimSpace(req.Reason) != "" {
		governance.UnverifiedReason = strings.TrimSpace(req.Reason)
	}
	governance.UpdatedAt = now

	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}
	if err := h.repos.Account().PutAgentGovernanceState(ctx.Context(), governance); err != nil {
		return respondAgentGovernanceWriteError(ctx, err)
	}

	h.recordAgentGovernanceEvent(ctx, username, "agent.unverified", map[string]any{
		"unverified_by": claims.Username,
		"reason":        strings.TrimSpace(req.Reason),
	})

	return okJSON(agentFromStorageUserWithBaseURL(account.User, governance, handlerBaseURL(h)))
}

// HandleAdminUnlockAgentLift handles POST /api/v1/admin/agents/:username/unlock.
func (h *Handler) HandleAdminUnlockAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	if h.repos == nil || h.repos.Account() == nil || h.repos.RateLimit() == nil {
		return common.RespondInternalServerError(ctx)
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	var req apimodels.AdminUnlockAgentRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}

	if err := h.clearAgentSafetyState(ctx.Context(), account.User.Username); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	reason := strings.TrimSpace(req.Reason)
	h.recordAgentGovernanceEvent(ctx, account.User.Username, "agent.unlocked", map[string]any{
		"unlocked_by":         claims.Username,
		"reason":              reason,
		"rate_limit_subjects": agentRateLimitUserIDVariants(account.User.Username),
	})

	return okJSON(apimodels.AdminUnlockAgentResponse{
		Username:   strings.TrimSpace(account.User.Username),
		Unlocked:   true,
		UnlockedBy: claims.Username,
		Reason:     reason,
		UnlockedAt: time.Now().UTC(),
	})
}

func (h *Handler) clearAgentSafetyState(ctx context.Context, username string) error {
	if h == nil || h.repos == nil || h.repos.RateLimit() == nil {
		return nil
	}

	for _, subject := range agentRateLimitUserIDVariants(username) {
		if err := h.repos.RateLimit().ClearLockout(ctx, subject); err != nil {
			return err
		}
		if err := h.repos.RateLimit().ClearAPIRateLimitsForUser(ctx, subject); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) recordAgentGovernanceEvent(ctx *apptheory.Context, username string, eventType string, metadata map[string]any) {
	if h == nil || h.repos == nil || h.repos.Audit() == nil {
		return
	}

	username = strings.TrimSpace(username)
	eventType = strings.TrimSpace(eventType)
	if username == "" || eventType == "" {
		return
	}
	if !strings.HasPrefix(eventType, "agent.") {
		eventType = "agent." + eventType
	}

	now := time.Now().UTC()
	entry := &storageModels.AuthAuditLog{
		ID:        common.GenerateOperationIDULID(),
		Timestamp: now,
		EventType: eventType,
		Severity:  "INFO",
		Username:  username,
		SessionID: "",
		IPAddress: headerValue(ctx, "x-forwarded-for"),
		UserAgent: headerValue(ctx, "user-agent"),
		Success:   true,
		CreatedAt: now,
	}

	if metadata != nil {
		if raw, err := json.Marshal(metadata); err == nil {
			entry.Metadata = string(raw)
		}
	}

	_ = h.repos.Audit().StoreAuditLog(ctx.Context(), entry)
}

func agentPolicyFromStorage(cfg *storageModels.AgentInstanceConfig) apimodels.AdminAgentPolicy {
	if cfg == nil {
		return apimodels.AdminAgentPolicy{}
	}

	return apimodels.AdminAgentPolicy{
		AllowAgents:                    cfg.AllowAgents,
		AllowAgentRegistration:         cfg.AllowAgentRegistration,
		DefaultQuarantineDays:          cfg.DefaultQuarantineDays,
		MaxAgentsPerOwner:              cfg.MaxAgentsPerOwner,
		AllowRemoteAgents:              cfg.AllowRemoteAgents,
		RemoteQuarantineDays:           cfg.RemoteQuarantineDays,
		BlockedAgentDomains:            append([]string(nil), cfg.BlockedAgentDomains...),
		TrustedAgentDomains:            append([]string(nil), cfg.TrustedAgentDomains...),
		AgentMaxPostsPerHour:           cfg.AgentMaxPostsPerHour,
		VerifiedAgentMaxPostsPerHour:   cfg.VerifiedAgentMaxPostsPerHour,
		AgentMaxFollowsPerHour:         cfg.AgentMaxFollowsPerHour,
		VerifiedAgentMaxFollowsPerHour: cfg.VerifiedAgentMaxFollowsPerHour,
		HybridRetrievalEnabled:         cfg.HybridRetrievalEnabled,
		HybridRetrievalMaxCandidates:   cfg.HybridRetrievalMaxCandidates,
		UpdatedAt:                      cfg.UpdatedAt,
	}
}

func normalizeDomainList(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		clean := strings.ToLower(strings.TrimSpace(domain))
		clean = strings.TrimPrefix(clean, "https://")
		clean = strings.TrimPrefix(clean, "http://")
		clean = strings.TrimSuffix(clean, "/")
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	return out
}
