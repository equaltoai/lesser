package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

const delegatedAgentClientID = "lesser-agent-delegation"

type delegationAgentInfo struct {
	AgentType         string               `json:"agent_type"`
	Version           string               `json:"version"`
	Capabilities      *agents.Capabilities `json:"capabilities,omitempty"`
	RestrictedDomains []string             `json:"restricted_domains,omitempty"`
}

func (h *Handler) ensureAgentsEnabled(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || h.cfg == nil || !h.cfg.AllowAgents {
		return common.RespondForbidden(ctx, "agents are disabled")
	}

	if h.repos != nil && h.repos.Instance() != nil {
		policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context())
		if err != nil {
			return common.RespondInternalServerError(ctx)
		}
		if policy == nil || !policy.AllowAgents {
			return common.RespondForbidden(ctx, "agents are disabled by instance policy")
		}
	}

	return nil, nil
}

func (h *Handler) ensureAgentRegistrationEnabled(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}
	if h.cfg == nil || !h.cfg.AllowAgentRegistration {
		return common.RespondForbidden(ctx, "agent registration is disabled")
	}

	if h.repos != nil && h.repos.Instance() != nil {
		policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context())
		if err != nil {
			return common.RespondInternalServerError(ctx)
		}
		if policy == nil || !policy.AllowAgentRegistration {
			return common.RespondForbidden(ctx, "agent registration is disabled by instance policy")
		}
	}

	return nil, nil
}

// HandleDelegateAgentLift handles POST /api/v1/agents/delegate.
//
// M2: delegated OAuth agent creation.
func (h *Handler) HandleDelegateAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentRegistrationEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	ownerClaims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.AgentDelegationRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	req.AgentUsername = strings.TrimSpace(req.AgentUsername)
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if err := common.ValidateUsernameParamID(req.AgentUsername); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("display_name", req.DisplayName); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	if h.mastodonLogic != nil {
		if err := h.mastodonLogic.ValidateDisplayName(req.DisplayName); err != nil {
			return common.RespondValidationError(ctx, err)
		}
		if err := h.mastodonLogic.ValidateBio(req.Bio); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	requestedScopes, resp, err := h.validateDelegationScopes(ctx, ownerClaims.Scopes, req.Scopes)
	if resp != nil || err != nil {
		return resp, err
	}

	accessTTL, resp, err := validateAgentAccessTokenTTL(ctx, req.ExpiresIn)
	if resp != nil || err != nil {
		return resp, err
	}

	info, err := parseDelegationAgentInfo(req.AgentInfo)
	if err != nil {
		return common.RespondBadRequest(ctx, "invalid agent_info")
	}

	capabilities := info.Capabilities
	if capabilities == nil {
		derived := deriveAgentCapabilitiesFromScopes(requestedScopes)
		capabilities = &derived
	}

	agentType := strings.TrimSpace(info.AgentType)
	if agentType == "" {
		agentType = "CUSTOM"
	}
	agentVersion := strings.TrimSpace(info.Version)
	if agentVersion == "" {
		agentVersion = "unknown"
	}

	ownerIdentifier := "@" + ownerClaims.Username

	now := time.Now().UTC()
	quarantineDays := 7
	maxPostsPerHourAllowed := agentDefaultMaxPostsPerHour
	if h.repos != nil && h.repos.Instance() != nil {
		if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context()); err == nil && policy != nil {
			if policy.DefaultQuarantineDays > 0 {
				quarantineDays = policy.DefaultQuarantineDays
			}
			if policy.AgentMaxPostsPerHour > 0 {
				maxPostsPerHourAllowed = policy.AgentMaxPostsPerHour
			}
		}
	}

	if maxPostsPerHourAllowed <= 0 {
		maxPostsPerHourAllowed = agentDefaultMaxPostsPerHour
	}
	if capabilities.MaxPostsPerHour <= 0 {
		capabilities.MaxPostsPerHour = maxPostsPerHourAllowed
	}
	if capabilities.MaxPostsPerHour > maxPostsPerHourAllowed {
		capabilities.MaxPostsPerHour = maxPostsPerHourAllowed
	}

	quarantineEnd := now.AddDate(0, 0, quarantineDays)
	user := &storage.User{
		Username:          req.AgentUsername,
		Email:             "",
		DisplayName:       req.DisplayName,
		Note:              req.Bio,
		Approved:          true,
		Suspended:         false,
		Silenced:          false,
		Role:              "user",
		Locale:            "",
		Locked:            false,
		Discoverable:      true,
		CreatedAt:         now,
		UpdatedAt:         now,
		IsAgent:           true,
		AgentType:         agentType,
		AgentVersion:      agentVersion,
		AgentOwner:        ownerIdentifier,
		AgentCreatedBy:    ownerClaims.Username,
		AgentCapabilities: capabilities,
		Metadata: map[string]interface{}{
			"agent_quarantine_status": "quarantined",
			"agent_quarantine_start":  now.Format(time.RFC3339),
			"agent_quarantine_end":    quarantineEnd.Format(time.RFC3339),
			"agent_delegated_scopes":  requestedScopes,
		},
	}

	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	publicKeyPEM, err := federation.EncodePublicKeyPEM(&privateKey.PublicKey)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	privateKeyPEM, err := federation.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	actorID := h.cfg.ActorURL(req.AgentUsername)
	actor := activitypub.NewActor(activitypub.ServiceType, actorID, req.AgentUsername)
	actor.Name = req.DisplayName
	actor.Summary = req.Bio
	actor.URL = fmt.Sprintf("%s/@%s", h.cfg.BaseURL(), req.AgentUsername)
	actor.CreatedAt = &now
	actor.PublicKey = &activitypub.PublicKey{
		ID:           actorID + "#main-key",
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}

	actor = activitypubutil.BuildLocalActor(req.AgentUsername, h.cfg.BaseURL(), user, actor)

	account := &storage.Account{
		User:       user,
		Actor:      actor,
		PrivateKey: string(privateKeyPEM),
	}

	if err := h.repos.Account().CreateAccount(ctx.Context(), account); err != nil {
		if common.IsConflict(err) {
			return common.RespondConflict(ctx, "username already taken")
		}
		return common.RespondInternalServerError(ctx)
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	accessToken, refreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTL(ctx.Context(), req.AgentUsername, delegatedAgentClientID, "", requestedScopes, accessTTL)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	refreshExpiry := time.Now().Add(accessTTL)
	oauthRefreshToken := &storage.RefreshToken{
		Token:     refreshToken,
		Username:  req.AgentUsername,
		ClientID:  delegatedAgentClientID,
		Scopes:    requestedScopes,
		CreatedAt: time.Now(),
		ExpiresAt: refreshExpiry,
	}
	_ = h.repos.Account().CreateRefreshToken(ctx.Context(), oauthRefreshToken)

	apiAccount := h.converter.ActorToAccount(actor)
	respPayload := apimodels.AgentDelegationResponse{
		Account: apiAccount,
		Token: apimodels.OAuthTokenResponse{
			AccessToken:  accessToken,
			TokenType:    "Bearer",
			Scope:        strings.Join(requestedScopes, " "),
			CreatedAt:    time.Now().Unix(),
			ExpiresIn:    int(accessTTL.Seconds()),
			RefreshToken: refreshToken,
		},
	}

	return okJSON(respPayload)
}

// HandleListAgentsLift handles GET /api/v1/agents.
//
// M2: agent directory.
func (h *Handler) HandleListAgentsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	limit := int32(20)
	users, _, err := h.repos.User().ListAgents(ctx.Context(), limit, "")
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	agentsOut := make([]apimodels.Agent, 0, len(users))
	for _, user := range users {
		if user == nil || !user.IsAgent {
			continue
		}
		if user.Suspended {
			continue
		}
		agentsOut = append(agentsOut, agentFromStorageUser(user))
	}

	return okJSON(agentsOut)
}

// HandleGetAgentLift handles GET /api/v1/agents/:username.
//
// M2: agent lookup.
func (h *Handler) HandleGetAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	user, err := h.repos.User().GetUser(ctx.Context(), username)
	if err != nil || user == nil || !user.IsAgent || user.Suspended {
		return common.RespondNotFound(ctx, "agent")
	}

	return okJSON(agentFromStorageUser(user))
}

// HandleUpdateAgentLift handles PATCH /api/v1/agents/:username.
//
// M2: agent metadata update (owner/admin only).
func (h *Handler) HandleUpdateAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}

	if !h.isAgentOwnerOrAdmin(claims, account.User) {
		return common.RespondForbidden(ctx, "not authorized to modify agent")
	}

	var req apimodels.UpdateAgentRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	if req.DisplayName != "" && h.mastodonLogic != nil {
		if err := h.mastodonLogic.ValidateDisplayName(req.DisplayName); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}
	if req.Bio != "" && h.mastodonLogic != nil {
		if err := h.mastodonLogic.ValidateBio(req.Bio); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	// Apply user updates.
	if strings.TrimSpace(req.DisplayName) != "" {
		account.User.DisplayName = strings.TrimSpace(req.DisplayName)
		if account.Actor != nil {
			account.Actor.Name = account.User.DisplayName
		}
	}
	if strings.TrimSpace(req.Bio) != "" {
		account.User.Note = strings.TrimSpace(req.Bio)
		if account.Actor != nil {
			account.Actor.Summary = account.User.Note
		}
	}
	if strings.TrimSpace(req.AgentType) != "" {
		account.User.AgentType = strings.TrimSpace(req.AgentType)
	}
	if strings.TrimSpace(req.AgentVersion) != "" {
		account.User.AgentVersion = strings.TrimSpace(req.AgentVersion)
	}
	if req.AgentCapabilities != nil {
		account.User.AgentCapabilities = storageAgentCapabilitiesFromAPI(*req.AgentCapabilities)

		allowed := agentDefaultMaxPostsPerHour
		verifiedAllowed := agentVerifiedDefaultMaxPostsPerHour
		if h.repos != nil && h.repos.Instance() != nil {
			if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context()); err == nil && policy != nil {
				if policy.AgentMaxPostsPerHour > 0 {
					allowed = policy.AgentMaxPostsPerHour
				}
				if policy.VerifiedAgentMaxPostsPerHour > 0 {
					verifiedAllowed = policy.VerifiedAgentMaxPostsPerHour
				}
			}
		}

		maxAllowed := allowed
		if agentMetadataBool(account.User, "agent_verified") {
			maxAllowed = verifiedAllowed
		}

		if account.User.AgentCapabilities != nil && account.User.AgentCapabilities.MaxPostsPerHour > maxAllowed {
			account.User.AgentCapabilities.MaxPostsPerHour = maxAllowed
		}
	}
	if req.ExitQuarantine {
		if account.User.Metadata == nil {
			account.User.Metadata = map[string]interface{}{}
		}
		now := time.Now().UTC()
		account.User.Metadata["agent_quarantine_status"] = "approved"
		account.User.Metadata["agent_quarantine_end"] = now.Format(time.RFC3339)
		account.User.Metadata["agent_quarantine_approved_by"] = claims.Username
		account.User.Metadata["agent_quarantine_approved_at"] = now.Format(time.RFC3339)
	}

	// Ensure actor reflects updated agent metadata.
	if account.Actor == nil {
		actorID := h.cfg.ActorURL(username)
		account.Actor = activitypub.NewActor(activitypub.ServiceType, actorID, username)
	}
	account.Actor.Type = activitypub.ServiceType
	account.User.IsAgent = true
	account.Actor = activitypubutil.BuildLocalActor(username, h.cfg.BaseURL(), account.User, account.Actor)

	account.User.UpdatedAt = time.Now().UTC()
	now := account.User.UpdatedAt
	account.Actor.Updated = &now

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentFromStorageUser(account.User))
}

// HandleDeleteAgentLift handles DELETE /api/v1/agents/:username.
//
// M2: deactivate agent (owner/admin only).
func (h *Handler) HandleDeleteAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}

	if !h.isAgentOwnerOrAdmin(claims, account.User) {
		return common.RespondForbidden(ctx, "not authorized to delete agent")
	}

	account.User.Suspended = true
	account.User.Discoverable = false
	account.User.UpdatedAt = time.Now().UTC()
	if account.Actor != nil {
		now := account.User.UpdatedAt
		account.Actor.Updated = &now
	}

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentFromStorageUser(account.User))
}

// HandleGetAgentActivityLift handles GET /api/v1/agents/:username/activity.
//
// M0: contract stub. Full implementation lands in M3.
func (h *Handler) HandleGetAgentActivityLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeRead)
		}
		return common.RespondUnauthorized(ctx)
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}

	// Owner/admin can view. Allow the agent itself to view its log as well.
	if !h.isAgentOwnerOrAdmin(claims, account.User) && !strings.EqualFold(claims.Username, username) {
		return common.RespondForbidden(ctx, "not authorized to view agent activity")
	}

	if h.repos == nil || h.repos.Audit() == nil {
		return common.RespondInternalServerError(ctx)
	}

	now := time.Now().UTC()
	start := now.Add(-30 * 24 * time.Hour)
	logs, err := h.repos.Audit().GetUserAuditLogs(ctx.Context(), username, 200, start, now)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	entries := make([]apimodels.AgentActivityLogEntry, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(log.EventType), "agent.") {
			continue
		}

		var (
			meta     any
			targetID string
		)
		if raw := strings.TrimSpace(log.Metadata); raw != "" {
			parsed := map[string]any{}
			if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
				meta = parsed
				if v, ok := parsed["target_id"].(string); ok {
					targetID = v
				}
			} else {
				meta = map[string]any{"raw": raw}
			}
		}

		entries = append(entries, apimodels.AgentActivityLogEntry{
			AgentUsername: username,
			Action:        log.EventType,
			TargetID:      targetID,
			Timestamp:     log.Timestamp,
			Metadata:      meta,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return okJSON(apimodels.AgentActivityLogList(entries))
}

// HandleSuspendAgentLift handles POST /api/v1/agents/:username/suspend.
//
// M2: admin suspend.
func (h *Handler) HandleSuspendAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	_, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
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

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}

	account.User.Suspended = true
	account.User.UpdatedAt = time.Now().UTC()
	if account.Actor != nil {
		now := account.User.UpdatedAt
		account.Actor.Updated = &now
	}

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentFromStorageUser(account.User))
}

func (h *Handler) authenticateAgentOwner(ctx *apptheory.Context) (*auth.Claims, *apptheory.Response, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return nil, resp, respErr
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return nil, resp, respErr
	}

	// Creating/managing local agents is an account write action; accept either broad or granular scope.
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		resp, respErr := common.RespondInsufficientScope(ctx, "write:accounts")
		return nil, resp, respErr
	}

	return claims, nil, nil
}

func (h *Handler) isAgentOwnerOrAdmin(claims *auth.Claims, agentUser *storage.User) bool {
	if claims == nil || agentUser == nil {
		return false
	}

	// Admin tokens can always manage agents.
	if claims.HasScope(auth.ScopeAdmin) || claims.HasScope("admin:write") || claims.HasScope("admin:all") {
		return true
	}

	owner := strings.TrimSpace(agentUser.AgentOwner)
	if owner == "" {
		return false
	}
	owner = strings.TrimPrefix(owner, "@")
	return strings.EqualFold(owner, claims.Username)
}

func (h *Handler) validateDelegationScopes(ctx *apptheory.Context, ownerScopes []string, requested []string) ([]string, *apptheory.Response, error) {
	if err := common.ValidateSliceNotEmpty("scopes", requested); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return nil, resp, respErr
	}

	clean := make([]string, 0, len(requested))
	for _, scope := range requested {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			resp, respErr := common.RespondValidationError(ctx, common.ValidationError{Field: "scopes", Message: "cannot contain empty values"})
			return nil, resp, respErr
		}

		base := strings.Split(scope, ":")[0]
		switch base {
		case "admin", "push":
			resp, respErr := common.RespondForbidden(ctx, "delegation cannot grant admin or push scopes")
			return nil, resp, respErr
		}

		clean = append(clean, scope)
	}

	if err := common.ValidateApplicationScopes(strings.Join(clean, " ")); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return nil, resp, respErr
	}

	if !scopesAreSubset(ownerScopes, clean) {
		resp, respErr := common.RespondForbidden(ctx, "requested scopes exceed delegator scopes")
		return nil, resp, respErr
	}

	return clean, nil, nil
}

func parseDelegationAgentInfo(raw any) (delegationAgentInfo, error) {
	if raw == nil {
		return delegationAgentInfo{}, nil
	}

	bytes, err := json.Marshal(raw)
	if err != nil {
		return delegationAgentInfo{}, err
	}

	var out delegationAgentInfo
	if err := json.Unmarshal(bytes, &out); err != nil {
		return delegationAgentInfo{}, err
	}
	return out, nil
}

func deriveAgentCapabilitiesFromScopes(scopes []string) agents.Capabilities {
	var caps agents.Capabilities

	for _, scope := range scopes {
		base := strings.Split(strings.TrimSpace(scope), ":")[0]
		switch base {
		case "write":
			caps.CanPost = true
			caps.CanReply = true
			caps.CanBoost = true
			caps.CanDM = true
		case "follow":
			caps.CanFollow = true
		}

		if scope == "write:statuses" {
			caps.CanPost = true
			caps.CanReply = true
			caps.CanBoost = true
			caps.CanDM = true
		}
		if scope == "write:follows" {
			caps.CanFollow = true
		}
	}

	return caps
}

func scopesAreSubset(ownerScopes, requested []string) bool {
	owned := map[string]struct{}{}
	for _, s := range ownerScopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		owned[s] = struct{}{}
	}

	for _, scope := range requested {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := owned[scope]; ok {
			continue
		}
		parts := strings.Split(scope, ":")
		if len(parts) == 2 {
			if _, ok := owned[parts[0]]; ok {
				continue
			}
		}
		return false
	}

	return true
}

func validateAgentAccessTokenTTL(ctx *apptheory.Context, expiresIn int) (time.Duration, *apptheory.Response, error) {
	if expiresIn == 0 {
		return auth.AccessTokenDuration, nil, nil
	}

	if expiresIn < 60 {
		resp, respErr := common.RespondValidationError(ctx, common.ValidationError{Field: "expires_in", Message: "must be at least 60 seconds"})
		return 0, resp, respErr
	}

	max := 7 * 24 * 60 * 60
	if expiresIn > max {
		resp, respErr := common.RespondValidationError(ctx, common.ValidationError{Field: "expires_in", Message: "cannot exceed 7 days"})
		return 0, resp, respErr
	}

	return time.Duration(expiresIn) * time.Second, nil, nil
}

func agentFromStorageUser(user *storage.User) apimodels.Agent {
	out := apimodels.Agent{
		Username:     user.Username,
		DisplayName:  user.DisplayName,
		Bio:          user.Note,
		AgentType:    user.AgentType,
		AgentVersion: user.AgentVersion,
		AgentOwner:   user.AgentOwner,
	}

	if !user.CreatedAt.IsZero() {
		created := user.CreatedAt
		out.CreatedAt = &created
	}

	out.Verified = agentMetadataBool(user, "agent_verified")
	if verifiedAt, ok := agentMetadataString(user, "agent_verified_at"); ok {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(verifiedAt)); err == nil {
			out.VerifiedAt = &parsed
		}
	}

	out.DelegatedScopes = agentDelegatedScopes(user)
	out.AgentCapabilities = apiAgentCapabilitiesFromStorage(user.AgentCapabilities)
	if strings.TrimSpace(out.AgentType) == "" {
		out.AgentType = "CUSTOM"
	}
	if strings.TrimSpace(out.AgentVersion) == "" {
		out.AgentVersion = "unknown"
	}

	return out
}

func agentMetadataBool(user *storage.User, key string) bool {
	if user == nil || user.Metadata == nil {
		return false
	}
	raw, ok := user.Metadata[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func agentDelegatedScopes(user *storage.User) []string {
	if user == nil || user.Metadata == nil {
		return nil
	}

	raw, ok := user.Metadata["agent_delegated_scopes"]
	if !ok || raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func apiAgentCapabilitiesFromStorage(caps *agents.Capabilities) apimodels.AgentCapabilities {
	if caps == nil {
		return apimodels.AgentCapabilities{
			MaxPostsPerHour:  0,
			RequiresApproval: false,
		}
	}

	return apimodels.AgentCapabilities{
		CanPost:           caps.CanPost,
		CanReply:          caps.CanReply,
		CanBoost:          caps.CanBoost,
		CanFollow:         caps.CanFollow,
		CanDM:             caps.CanDM,
		RestrictedDomains: append([]string(nil), caps.RestrictedDomains...),
		MaxPostsPerHour:   caps.MaxPostsPerHour,
		RequiresApproval:  caps.RequiresApproval,
	}
}

func storageAgentCapabilitiesFromAPI(caps apimodels.AgentCapabilities) *agents.Capabilities {
	return &agents.Capabilities{
		CanPost:           caps.CanPost,
		CanReply:          caps.CanReply,
		CanBoost:          caps.CanBoost,
		CanFollow:         caps.CanFollow,
		CanDM:             caps.CanDM,
		RestrictedDomains: append([]string(nil), caps.RestrictedDomains...),
		MaxPostsPerHour:   caps.MaxPostsPerHour,
		RequiresApproval:  caps.RequiresApproval,
	}
}
