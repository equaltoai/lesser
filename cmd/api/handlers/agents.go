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

const (
	delegatedAgentClientID = "lesser-agent-delegation"

	agentTypeCustom     = "CUSTOM"
	agentVersionUnknown = "unknown"
)

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

	req, resp, err := parseAgentDelegationRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	if resp, err := h.validateAgentDelegationRequest(ctx, req); resp != nil || err != nil {
		return resp, err
	}

	requestedScopes, resp, err := h.validateDelegationScopes(ctx, ownerClaims.Scopes, req.Scopes)
	if resp != nil || err != nil {
		return resp, err
	}

	accessTTL, resp, err := validateAgentAccessTokenTTL(ctx, req.ExpiresIn)
	if resp != nil || err != nil {
		return resp, err
	}

	now := time.Now().UTC()
	info, resp, err := parseAgentDelegationInfo(ctx, req.AgentInfo, requestedScopes)
	if resp != nil || err != nil {
		return resp, err
	}

	account, resp, err := h.createDelegatedAgentAccount(ctx, ownerClaims, req, info, requestedScopes, now)
	if resp != nil || err != nil {
		return resp, err
	}

	token, err := h.mintDelegatedAgentTokens(ctx, req.AgentUsername, requestedScopes, accessTTL)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	apiAccount := h.converter.ActorToAccount(account.Actor)
	respPayload := apimodels.AgentDelegationResponse{
		Account: apiAccount,
		Token:   token,
	}

	return okJSON(respPayload)
}

type delegationResolvedInfo struct {
	AgentType    string
	AgentVersion string
	Capabilities *agents.Capabilities
}

func parseAgentDelegationRequest(ctx *apptheory.Context) (*apimodels.AgentDelegationRequest, *apptheory.Response, error) {
	var req apimodels.AgentDelegationRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return nil, resp, respErr
	}

	req.AgentUsername = strings.TrimSpace(req.AgentUsername)
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	return &req, nil, nil
}

func (h *Handler) validateAgentDelegationRequest(ctx *apptheory.Context, req *apimodels.AgentDelegationRequest) (*apptheory.Response, error) {
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

	return nil, nil
}

func parseAgentDelegationInfo(ctx *apptheory.Context, raw any, requestedScopes []string) (delegationResolvedInfo, *apptheory.Response, error) {
	info, err := parseDelegationAgentInfo(raw)
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid agent_info")
		return delegationResolvedInfo{}, resp, respErr
	}

	capabilities := info.Capabilities
	if capabilities == nil {
		derived := deriveAgentCapabilitiesFromScopes(requestedScopes)
		capabilities = &derived
	}

	agentType := strings.TrimSpace(info.AgentType)
	if agentType == "" {
		agentType = agentTypeCustom
	}

	agentVersion := strings.TrimSpace(info.Version)
	if agentVersion == "" {
		agentVersion = agentVersionUnknown
	}

	return delegationResolvedInfo{
		AgentType:    agentType,
		AgentVersion: agentVersion,
		Capabilities: capabilities,
	}, nil, nil
}

func (h *Handler) agentRegistrationLimits(ctx *apptheory.Context) (quarantineDays int, maxPostsPerHourAllowed int) {
	quarantineDays = 7
	maxPostsPerHourAllowed = agentDefaultMaxPostsPerHour

	if h != nil && h.repos != nil && h.repos.Instance() != nil {
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

	return quarantineDays, maxPostsPerHourAllowed
}

func clampMaxPostsPerHour(capabilities *agents.Capabilities, maxPostsPerHourAllowed int) {
	if capabilities == nil {
		return
	}
	if capabilities.MaxPostsPerHour <= 0 {
		capabilities.MaxPostsPerHour = maxPostsPerHourAllowed
	}
	if capabilities.MaxPostsPerHour > maxPostsPerHourAllowed {
		capabilities.MaxPostsPerHour = maxPostsPerHourAllowed
	}
}

func (h *Handler) createDelegatedAgentAccount(
	ctx *apptheory.Context,
	ownerClaims *auth.Claims,
	req *apimodels.AgentDelegationRequest,
	info delegationResolvedInfo,
	requestedScopes []string,
	now time.Time,
) (*storage.Account, *apptheory.Response, error) {
	quarantineDays, maxPostsPerHourAllowed := h.agentRegistrationLimits(ctx)
	clampMaxPostsPerHour(info.Capabilities, maxPostsPerHourAllowed)
	quarantineEnd := now.AddDate(0, 0, quarantineDays)

	ownerIdentifier := "@" + ownerClaims.Username
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
		AgentType:         info.AgentType,
		AgentVersion:      info.AgentVersion,
		AgentOwner:        ownerIdentifier,
		AgentCreatedBy:    ownerClaims.Username,
		AgentCapabilities: info.Capabilities,
		Metadata: map[string]interface{}{
			"agent_quarantine_status": "quarantined",
			"agent_quarantine_start":  now.Format(time.RFC3339),
			"agent_quarantine_end":    quarantineEnd.Format(time.RFC3339),
			"agent_delegated_scopes":  requestedScopes,
		},
	}

	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}
	publicKeyPEM, err := federation.EncodePublicKeyPEM(&privateKey.PublicKey)
	if err != nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}
	privateKeyPEM, err := federation.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
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
			resp, respErr := common.RespondConflict(ctx, "username already taken")
			return nil, resp, respErr
		}
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}

	return account, nil, nil
}

func (h *Handler) mintDelegatedAgentTokens(ctx *apptheory.Context, agentUsername string, requestedScopes []string, accessTTL time.Duration) (apimodels.OAuthTokenResponse, error) {
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	accessToken, refreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTL(ctx.Context(), agentUsername, delegatedAgentClientID, "", requestedScopes, accessTTL)
	if err != nil {
		return apimodels.OAuthTokenResponse{}, err
	}

	now := time.Now()
	refreshExpiry := now.Add(accessTTL)
	oauthRefreshToken := &storage.RefreshToken{
		Token:     refreshToken,
		Username:  agentUsername,
		ClientID:  delegatedAgentClientID,
		Scopes:    requestedScopes,
		CreatedAt: now,
		ExpiresAt: refreshExpiry,
	}
	_ = h.repos.Account().CreateRefreshToken(ctx.Context(), oauthRefreshToken)

	return apimodels.OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		Scope:        strings.Join(requestedScopes, " "),
		CreatedAt:    now.Unix(),
		ExpiresIn:    int(accessTTL.Seconds()),
		RefreshToken: refreshToken,
	}, nil
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

	req, resp, err := parseUpdateAgentRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	if resp, err := h.validateUpdateAgentRequest(ctx, req); resp != nil || err != nil {
		return resp, err
	}

	now := time.Now().UTC()
	h.applyAgentUpdateRequest(ctx, account, username, claims, req, now)

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentFromStorageUser(account.User))
}

func parseUpdateAgentRequest(ctx *apptheory.Context) (*apimodels.UpdateAgentRequest, *apptheory.Response, error) {
	var req apimodels.UpdateAgentRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return nil, resp, respErr
	}
	return &req, nil, nil
}

func (h *Handler) validateUpdateAgentRequest(ctx *apptheory.Context, req *apimodels.UpdateAgentRequest) (*apptheory.Response, error) {
	if h.mastodonLogic == nil {
		return nil, nil
	}
	if strings.TrimSpace(req.DisplayName) != "" {
		if err := h.mastodonLogic.ValidateDisplayName(req.DisplayName); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}
	if strings.TrimSpace(req.Bio) != "" {
		if err := h.mastodonLogic.ValidateBio(req.Bio); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}
	return nil, nil
}

func (h *Handler) applyAgentUpdateRequest(
	ctx *apptheory.Context,
	account *storage.Account,
	username string,
	claims *auth.Claims,
	req *apimodels.UpdateAgentRequest,
	now time.Time,
) {
	applyAgentProfileUpdates(account, req)
	applyAgentInfoUpdates(account.User, req)
	h.applyAgentCapabilitiesUpdate(ctx, account.User, req.AgentCapabilities)
	applyAgentQuarantineExit(account.User, claims, req.ExitQuarantine, now)

	h.ensureAgentActor(username, account)
	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}
}

func applyAgentProfileUpdates(account *storage.Account, req *apimodels.UpdateAgentRequest) {
	if account == nil || account.User == nil || req == nil {
		return
	}

	if v := strings.TrimSpace(req.DisplayName); v != "" {
		account.User.DisplayName = v
		if account.Actor != nil {
			account.Actor.Name = v
		}
	}
	if v := strings.TrimSpace(req.Bio); v != "" {
		account.User.Note = v
		if account.Actor != nil {
			account.Actor.Summary = v
		}
	}
}

func applyAgentInfoUpdates(user *storage.User, req *apimodels.UpdateAgentRequest) {
	if user == nil || req == nil {
		return
	}
	if v := strings.TrimSpace(req.AgentType); v != "" {
		user.AgentType = v
	}
	if v := strings.TrimSpace(req.AgentVersion); v != "" {
		user.AgentVersion = v
	}
}

func applyAgentQuarantineExit(user *storage.User, claims *auth.Claims, exitQuarantine bool, now time.Time) {
	if !exitQuarantine || user == nil {
		return
	}
	if user.Metadata == nil {
		user.Metadata = map[string]interface{}{}
	}

	approvedBy := ""
	if claims != nil {
		approvedBy = claims.Username
	}

	user.Metadata["agent_quarantine_status"] = "approved"
	user.Metadata["agent_quarantine_end"] = now.Format(time.RFC3339)
	user.Metadata["agent_quarantine_approved_by"] = approvedBy
	user.Metadata["agent_quarantine_approved_at"] = now.Format(time.RFC3339)
}

func (h *Handler) applyAgentCapabilitiesUpdate(ctx *apptheory.Context, user *storage.User, apiCaps *apimodels.AgentCapabilities) {
	if user == nil || apiCaps == nil {
		return
	}

	user.AgentCapabilities = storageAgentCapabilitiesFromAPI(*apiCaps)
	maxAllowed := h.maxPostsPerHourAllowedForUpdate(ctx, user)
	if user.AgentCapabilities != nil && user.AgentCapabilities.MaxPostsPerHour > maxAllowed {
		user.AgentCapabilities.MaxPostsPerHour = maxAllowed
	}
}

func (h *Handler) maxPostsPerHourAllowedForUpdate(ctx *apptheory.Context, user *storage.User) int {
	allowed := agentDefaultMaxPostsPerHour
	verifiedAllowed := agentVerifiedDefaultMaxPostsPerHour

	if h != nil && h.repos != nil && h.repos.Instance() != nil {
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
	if agentMetadataBool(user, "agent_verified") {
		maxAllowed = verifiedAllowed
	}
	return maxAllowed
}

func (h *Handler) ensureAgentActor(username string, account *storage.Account) {
	if h == nil || account == nil || account.User == nil {
		return
	}
	if account.Actor == nil {
		actorID := h.cfg.ActorURL(username)
		account.Actor = activitypub.NewActor(activitypub.ServiceType, actorID, username)
	}
	account.Actor.Type = activitypub.ServiceType
	account.User.IsAgent = true
	account.Actor = activitypubutil.BuildLocalActor(username, h.cfg.BaseURL(), account.User, account.Actor)
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
		case roleAdmin, "push":
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

	maxSeconds := 7 * 24 * 60 * 60
	if expiresIn > maxSeconds {
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
		out.AgentType = agentTypeCustom
	}
	if strings.TrimSpace(out.AgentVersion) == "" {
		out.AgentVersion = agentVersionUnknown
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
