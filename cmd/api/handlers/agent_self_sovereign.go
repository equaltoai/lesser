package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

const (
	selfSovereignAgentClientID = auth.SelfSovereignAgentRuntimeClientID

	agentKeyChallengeTTL = 5 * time.Minute

	agentKeyActionRegister  = "register"
	agentKeyActionAuth      = "auth"
	agentKeyActionRotateKey = "rotate_key"
)

func (h *Handler) agentKeyChallengeRepo() *storageRepos.AgentKeyChallengeRepository {
	if h == nil || h.repos == nil {
		return nil
	}

	return storageRepos.NewAgentKeyChallengeRepository(h.repos.GetDB(), h.repos.GetTableName(), h.logger)
}

// HandleAgentRegisterChallengeLift issues a one-time challenge for self-sovereign agent registration.
func (h *Handler) HandleAgentRegisterChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentRegistrationEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.AgentKeyChallengeRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	username := strings.TrimSpace(req.Username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	challenge, err := h.createAgentKeyChallenge(ctx, username, agentKeyActionRegister, agentKeyChallengeTTL)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentKeyChallengeResponse(challenge))
}

// HandleAgentRegisterLift registers a self-sovereign agent by verifying a signed registration challenge and
// minting OAuth tokens for the new agent.
func (h *Handler) HandleAgentRegisterLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentRegistrationEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	req, resp, err := parseAgentSelfRegistrationRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	if resp, err := h.validateAgentSelfRegistrationRequest(ctx, req); resp != nil || err != nil {
		return resp, err
	}

	if resp, err := h.verifyAndConsumeSelfRegistrationChallenge(ctx, req); resp != nil || err != nil {
		return resp, err
	}

	now := time.Now().UTC()
	user, requestedScopes, resp, err := h.buildSelfSovereignAgentUser(ctx, req, now)
	if resp != nil || err != nil {
		return resp, err
	}

	account, resp, err := h.createSelfSovereignAgentAccount(ctx, user, req.Username, req.DisplayName, req.Bio, requestedScopes, now)
	if resp != nil || err != nil {
		return resp, err
	}

	userAgent, _ := h.getDeviceInfo(ctx)
	token, err := h.mintSelfSovereignTokens(ctx, req.Username, requestedScopes, req.DeviceLabel, userAgent)
	if err != nil {
		return oauthTemporarilyUnavailable("Agent token mint is temporarily unavailable")
	}

	apiAccount := h.converter.ActorToAccount(account.Actor)
	respPayload := apimodels.AgentSelfRegistrationResponse{
		Account: apiAccount,
		Token:   token,
	}

	return okJSON(respPayload)
}

func parseAgentSelfRegistrationRequest(ctx *apptheory.Context) (*apimodels.AgentSelfRegistrationRequest, *apptheory.Response, error) {
	var req apimodels.AgentSelfRegistrationRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return nil, resp, respErr
	}

	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.PublicKey = strings.TrimSpace(req.PublicKey)
	req.KeyType = strings.TrimSpace(req.KeyType)
	req.ChallengeID = strings.TrimSpace(req.ChallengeID)
	req.Signature = strings.TrimSpace(req.Signature)
	req.DeviceLabel = strings.TrimSpace(req.DeviceLabel)

	return &req, nil, nil
}

func (h *Handler) validateAgentSelfRegistrationRequest(ctx *apptheory.Context, req *apimodels.AgentSelfRegistrationRequest) (*apptheory.Response, error) {
	if err := common.ValidateUsernameParamID(req.Username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("display_name", req.DisplayName); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("public_key", req.PublicKey); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("key_type", req.KeyType); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("challenge_id", req.ChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
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

func (h *Handler) verifyAndConsumeSelfRegistrationChallenge(ctx *apptheory.Context, req *apimodels.AgentSelfRegistrationRequest) (*apptheory.Response, error) {
	challenge, resp, err := h.loadAndValidateAgentKeyChallenge(ctx, req.ChallengeID, req.Username, agentKeyActionRegister)
	if resp != nil || err != nil {
		return resp, err
	}

	if err := auth.VerifyAgentChallengeSignature(req.KeyType, req.PublicKey, challenge.Message, req.Signature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": "challenge signature verification failed",
		})
	}

	if err := h.markAgentKeyChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}

	return nil, nil
}

func (h *Handler) buildSelfSovereignAgentUser(ctx *apptheory.Context, req *apimodels.AgentSelfRegistrationRequest, now time.Time) (*storage.User, []string, *apptheory.Response, error) {
	instanceInfo, err := parseDelegationAgentInfo(req.AgentInfo)
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid agent_info")
		return nil, nil, resp, respErr
	}

	requestedScopes := normalizeSelfSovereignScopes(req.Scopes)
	capabilities := instanceInfo.Capabilities
	if capabilities == nil {
		derived := deriveAgentCapabilitiesFromScopes(requestedScopes)
		capabilities = &derived
	}

	agentType := strings.TrimSpace(instanceInfo.AgentType)
	if agentType == "" {
		agentType = agentTypeCustom
	}
	agentVersion := strings.TrimSpace(instanceInfo.Version)
	if agentVersion == "" {
		agentVersion = agentVersionUnknown
	}

	maxPostsPerHourAllowed := agentDefaultMaxPostsPerHour
	if h.repos != nil && h.repos.Instance() != nil {
		if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context()); err == nil && policy != nil {
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

	user := &storage.User{
		Username:          req.Username,
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
		AgentOwner:        "",
		AgentCreatedBy:    req.Username,
		AgentCapabilities: capabilities,
		AgentPublicKey:    req.PublicKey,
		AgentKeyType:      strings.ToLower(strings.TrimSpace(req.KeyType)),
	}

	return user, requestedScopes, nil, nil
}

func (h *Handler) createSelfSovereignAgentAccount(ctx *apptheory.Context, user *storage.User, username, displayName, bio string, requestedScopes []string, now time.Time) (*storage.Account, *apptheory.Response, error) {
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

	actorID := h.cfg.ActorURL(username)
	actor := activitypub.NewActor(activitypub.ServiceType, actorID, username)
	actor.Name = displayName
	actor.Summary = bio
	actor.URL = fmt.Sprintf("%s/@%s", h.cfg.BaseURL(), username)
	actor.CreatedAt = &now
	actor.PublicKey = &activitypub.PublicKey{
		ID:           actorID + "#main-key",
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}

	actor = activitypubutil.BuildLocalActor(username, h.cfg.BaseURL(), user, actor)
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
	quarantineDays := 7
	if h.repos != nil && h.repos.Instance() != nil {
		if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context()); err == nil && policy != nil && policy.DefaultQuarantineDays > 0 {
			quarantineDays = policy.DefaultQuarantineDays
		}
	}
	quarantineEnd := now.AddDate(0, 0, quarantineDays)
	governance := &storage.AgentGovernanceState{
		Username:         username,
		QuarantineStatus: storage.AgentQuarantineStatusQuarantined,
		QuarantineStart:  cloneAgentGovernanceHandlerTime(&now),
		QuarantineEnd:    cloneAgentGovernanceHandlerTime(&quarantineEnd),
		SelfScopes:       normalizeSelfSovereignScopes(requestedScopes),
		SelfSovereign:    true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := h.repos.Account().PutAgentGovernanceState(ctx.Context(), governance); err != nil {
		_ = h.repos.Account().DeleteAccount(ctx.Context(), username)
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}

	return account, nil, nil
}

func (h *Handler) mintSelfSovereignTokens(ctx *apptheory.Context, username string, requestedScopes []string, deviceLabel, userAgent string) (apimodels.OAuthTokenResponse, error) {
	accessTTL := auth.AgentAccessTokenTTL(h.cfg)
	if strings.TrimSpace(deviceLabel) == "" {
		deviceLabel = userAgent
	}
	bundle, err := auth.IssueAgentRuntimeTokens(ctx.Context(), h.cfg, h.repos, auth.AgentRuntimeTokenIssueParams{
		Username:    username,
		ClientID:    selfSovereignAgentClientID,
		Scopes:      requestedScopes,
		AccessTTL:   accessTTL,
		DeviceLabel: deviceLabel,
	})
	if err != nil {
		return apimodels.OAuthTokenResponse{}, err
	}

	now := bundle.Session.CreatedAt

	return apimodels.OAuthTokenResponse{
		AccessToken:  bundle.AccessToken,
		TokenType:    "Bearer",
		Scope:        strings.Join(requestedScopes, " "),
		CreatedAt:    now.Unix(),
		ExpiresIn:    int(accessTTL.Seconds()),
		RefreshToken: bundle.RefreshToken,
	}, nil
}

// HandleAgentAuthChallengeLift issues a one-time challenge for self-sovereign token minting.
func (h *Handler) HandleAgentAuthChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.AgentKeyChallengeRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	username := strings.TrimSpace(req.Username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	agentUser, _ := h.repos.User().GetUser(ctx.Context(), username)
	if agentUser == nil || !agentUser.IsAgent || agentUser.Suspended {
		return common.RespondNotFound(ctx, "agent")
	}
	if strings.TrimSpace(agentUser.AgentPublicKey) == "" || strings.TrimSpace(agentUser.AgentKeyType) == "" {
		return common.RespondForbidden(ctx, "agent does not have a self-sovereign key configured")
	}

	challenge, err := h.createAgentKeyChallenge(ctx, username, agentKeyActionAuth, agentKeyChallengeTTL)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentKeyChallengeResponse(challenge))
}

// HandleAgentAuthTokenLift mints an OAuth token by verifying a signed challenge.
func (h *Handler) HandleAgentAuthTokenLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.AgentSelfAuthTokenRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	req.Username = strings.TrimSpace(req.Username)
	req.ChallengeID = strings.TrimSpace(req.ChallengeID)
	req.Signature = strings.TrimSpace(req.Signature)
	req.DeviceLabel = strings.TrimSpace(req.DeviceLabel)

	if err := common.ValidateUsernameParamID(req.Username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("challenge_id", req.ChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	agentUser, _ := h.repos.User().GetUser(ctx.Context(), req.Username)
	if agentUser == nil || !agentUser.IsAgent || agentUser.Suspended {
		return common.RespondNotFound(ctx, "agent")
	}
	if strings.TrimSpace(agentUser.AgentPublicKey) == "" || strings.TrimSpace(agentUser.AgentKeyType) == "" {
		return common.RespondForbidden(ctx, "agent does not have a self-sovereign key configured")
	}

	challenge, resp, err := h.loadAndValidateAgentKeyChallenge(ctx, req.ChallengeID, req.Username, agentKeyActionAuth)
	if resp != nil || err != nil {
		return resp, err
	}

	if err := auth.VerifyAgentChallengeSignature(agentUser.AgentKeyType, agentUser.AgentPublicKey, challenge.Message, req.Signature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": "challenge signature verification failed",
		})
	}

	if err := h.markAgentKeyChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}

	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, req.Username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
	}
	scopes := agentSelfSovereignScopes(governance)
	userAgent, _ := h.getDeviceInfo(ctx)
	tokenBundle, err := auth.IssueAgentRuntimeTokens(ctx.Context(), h.cfg, h.repos, auth.AgentRuntimeTokenIssueParams{
		Username:    req.Username,
		ClientID:    selfSovereignAgentClientID,
		Scopes:      scopes,
		AccessTTL:   auth.AgentAccessTokenTTL(h.cfg),
		DeviceLabel: auth.CoalesceAgentRuntimeLabel(req.DeviceLabel, userAgent),
	})
	if err != nil {
		return oauthTemporarilyUnavailable("Agent token mint is temporarily unavailable")
	}

	return okJSON(apimodels.OAuthTokenResponse{
		AccessToken:  tokenBundle.AccessToken,
		TokenType:    "Bearer",
		Scope:        strings.Join(scopes, " "),
		CreatedAt:    tokenBundle.Session.CreatedAt.Unix(),
		ExpiresIn:    tokenBundle.Session.AccessTTLSeconds,
		RefreshToken: tokenBundle.RefreshToken,
	})
}

// HandleAgentRotateKeyChallengeLift issues a one-time challenge for rotating a self-sovereign agent key.
func (h *Handler) HandleAgentRotateKeyChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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
	if !claims.IsAgent || !strings.EqualFold(claims.Username, username) {
		return common.RespondForbidden(ctx, "not authorized to rotate agent key")
	}

	agentUser, _ := h.repos.User().GetUser(ctx.Context(), username)
	if agentUser == nil || !agentUser.IsAgent || agentUser.Suspended {
		return common.RespondNotFound(ctx, "agent")
	}
	if strings.TrimSpace(agentUser.AgentPublicKey) == "" || strings.TrimSpace(agentUser.AgentKeyType) == "" {
		return common.RespondForbidden(ctx, "agent does not have a self-sovereign key configured")
	}

	challenge, err := h.createAgentKeyChallenge(ctx, username, agentKeyActionRotateKey, agentKeyChallengeTTL)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentKeyChallengeResponse(challenge))
}

// HandleAgentRotateKeyLift rotates a self-sovereign agent key after verifying a signed challenge.
func (h *Handler) HandleAgentRotateKeyLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeWrite)
		}
		return common.RespondUnauthorized(ctx)
	}
	if !claims.IsAgent || !strings.EqualFold(claims.Username, username) {
		return common.RespondForbidden(ctx, "not authorized to rotate agent key")
	}

	var req apimodels.AgentRotateKeyRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	req.PublicKey = strings.TrimSpace(req.PublicKey)
	req.KeyType = strings.TrimSpace(req.KeyType)
	req.ChallengeID = strings.TrimSpace(req.ChallengeID)
	req.Signature = strings.TrimSpace(req.Signature)

	if err := common.ValidateRequiredParam("public_key", req.PublicKey); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("key_type", req.KeyType); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("challenge_id", req.ChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}

	if strings.TrimSpace(account.User.AgentPublicKey) == "" || strings.TrimSpace(account.User.AgentKeyType) == "" {
		return common.RespondForbidden(ctx, "agent does not have a self-sovereign key configured")
	}

	challenge, resp, err := h.loadAndValidateAgentKeyChallenge(ctx, req.ChallengeID, username, agentKeyActionRotateKey)
	if resp != nil || err != nil {
		return resp, err
	}

	if err := auth.VerifyAgentChallengeSignature(account.User.AgentKeyType, account.User.AgentPublicKey, challenge.Message, req.Signature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": "challenge signature verification failed",
		})
	}

	if _, err := auth.ParseAgentPublicKey(req.KeyType, req.PublicKey); err != nil {
		return common.RespondValidationError(ctx, common.ValidationError{Field: "public_key", Message: "invalid public key"})
	}

	if err := h.markAgentKeyChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}

	now := time.Now().UTC()
	account.User.AgentPublicKey = req.PublicKey
	account.User.AgentKeyType = strings.ToLower(strings.TrimSpace(req.KeyType))
	account.User.UpdatedAt = now
	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, account.User.Username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
	}
	governance.KeyRotatedAt = cloneAgentGovernanceHandlerTime(&now)
	governance.UpdatedAt = now

	if account.Actor != nil {
		account.Actor.Updated = &now
	}

	if err := h.repos.Account().UpdateAccount(ctx.Context(), account); err != nil {
		return common.RespondInternalServerError(ctx)
	}
	if err := h.repos.Account().PutAgentGovernanceState(ctx.Context(), governance); err != nil {
		return respondAgentGovernanceWriteError(ctx, err)
	}

	h.recordAgentAuditEvent(ctx, claims, "agent.key_rotated", "", map[string]any{
		"key_type": account.User.AgentKeyType,
	})

	return okJSON(agentFromStorageUserWithBaseURL(account.User, governance, handlerBaseURL(h)))
}

func (h *Handler) createAgentKeyChallenge(ctx *apptheory.Context, username string, action string, ttl time.Duration) (*storageModels.AgentKeyChallenge, error) {
	repo := h.agentKeyChallengeRepo()
	if repo == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	username = strings.TrimSpace(username)
	action = strings.TrimSpace(action)
	if username == "" || action == "" {
		return nil, fmt.Errorf("missing fields")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)

	id := common.GenerateOperationIDULID()
	message := buildAgentKeyChallengeMessage(id, action, username, nonce, now, expiresAt)

	model := &storageModels.AgentKeyChallenge{
		ID:        id,
		Username:  username,
		Action:    action,
		Nonce:     nonce,
		Message:   message,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
		Used:      false,
	}

	if err := repo.Create(ctx.Context(), model); err != nil {
		return nil, err
	}

	return model, nil
}

func (h *Handler) loadAndValidateAgentKeyChallenge(ctx *apptheory.Context, challengeID string, username string, action string) (*storageModels.AgentKeyChallenge, *apptheory.Response, error) {
	repo := h.agentKeyChallengeRepo()
	if repo == nil {
		resp, respErr := apptheory.JSON(http.StatusServiceUnavailable, map[string]any{
			"error":             "service_unavailable",
			"error_description": "storage is not initialized",
		})
		return nil, resp, respErr
	}

	challengeID = strings.TrimSpace(challengeID)
	username = strings.TrimSpace(username)
	action = strings.TrimSpace(action)
	if challengeID == "" || username == "" || action == "" {
		resp, respErr := common.RespondBadRequest(ctx, "missing challenge fields")
		return nil, resp, respErr
	}

	challenge, err := repo.Get(ctx.Context(), challengeID)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			resp, respErr := apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "challenge not found",
			})
			return nil, resp, respErr
		}
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}

	if strings.TrimSpace(challenge.Username) != username || strings.TrimSpace(challenge.Action) != action {
		resp, respErr := apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge does not match request",
		})
		return nil, resp, respErr
	}

	now := time.Now().UTC()
	if challenge.ExpiresAt.IsZero() || now.After(challenge.ExpiresAt) || challenge.TTL <= now.Unix() {
		resp, respErr := apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge expired",
		})
		return nil, resp, respErr
	}

	if challenge.Used {
		resp, respErr := apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge already used",
		})
		return nil, resp, respErr
	}

	return challenge, nil, nil
}

func (h *Handler) markAgentKeyChallengeUsed(ctx *apptheory.Context, challengeID string) error {
	repo := h.agentKeyChallengeRepo()
	if repo == nil {
		return fmt.Errorf("storage not initialized")
	}

	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return fmt.Errorf("challenge id is empty")
	}

	return repo.MarkUsed(ctx.Context(), challengeID, time.Now().UTC())
}

func buildAgentKeyChallengeMessage(id string, action string, username string, nonce string, issuedAt time.Time, expiresAt time.Time) string {
	return fmt.Sprintf("LESSER AGENT CHALLENGE\nid: %s\naction: %s\nusername: %s\nnonce: %s\nissued_at: %s\nexpires_at: %s",
		strings.TrimSpace(id),
		strings.TrimSpace(action),
		strings.TrimSpace(username),
		strings.TrimSpace(nonce),
		issuedAt.UTC().Format(time.RFC3339),
		expiresAt.UTC().Format(time.RFC3339),
	)
}

func agentKeyChallengeResponse(challenge *storageModels.AgentKeyChallenge) apimodels.AgentKeyChallengeResponse {
	if challenge == nil {
		return apimodels.AgentKeyChallengeResponse{}
	}

	return apimodels.AgentKeyChallengeResponse{
		ID:        strings.TrimSpace(challenge.ID),
		Username:  strings.TrimSpace(challenge.Username),
		Action:    strings.TrimSpace(challenge.Action),
		Message:   challenge.Message,
		IssuedAt:  challenge.IssuedAt,
		ExpiresAt: challenge.ExpiresAt,
	}
}

func normalizeSelfSovereignScopes(scopes []string) []string {
	// Self-sovereign tokens should remain compatible with Lesser’s existing scope checks.
	// Preserve only the base scopes: read/write/follow, ignoring granular suffixes.
	if len(scopes) == 0 {
		return []string{auth.ScopeRead, auth.ScopeWrite, "follow"}
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 3)

	add := func(scope string) {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return
		}
		if _, ok := seen[scope]; ok {
			return
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}

	for _, raw := range scopes {
		base := strings.Split(strings.TrimSpace(raw), ":")[0]
		switch base {
		case auth.ScopeRead, auth.ScopeWrite, "follow":
			add(base)
		}
	}

	if len(out) == 0 {
		return []string{auth.ScopeRead, auth.ScopeWrite, "follow"}
	}
	return out
}
