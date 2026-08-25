package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

const (
	agentAccessLeaseClientID             = "lesser-agent-wallet-lease"
	agentAccessLeaseChallengeTTL         = 5 * time.Minute
	agentAccessLeaseActionPrincipal      = "principal_approve"
	agentAccessLeaseActionAgent          = "agent_accept"
	agentAccessLeaseActionRenewWallet    = "renew_wallet"
	agentAccessLeaseActionRenewSession   = "renew_session"
	agentAccessLeaseActionSessionKeyAuth = "session_key_authorize"
	agentAccessLeaseStatusActive         = "active"
	agentAccessLeaseStatusRevoked        = "revoked"
	agentAccessLeaseDefaultIdleHrs       = 7 * 24
	agentAccessLeaseMaxIdleHrs           = 30 * 24
	agentAccessLeaseDefaultAbsHrs        = 90 * 24
	agentAccessLeaseMaxAbsHrs            = 365 * 24
	agentAccessLeaseSessionKeyType       = "ed25519"
	agentAccessLeaseTypedDataVersion     = "1"
	agentAccessLeaseStatusExpired        = "expired"
)

type agentAccessLeaseOptions struct {
	LeaseID           string
	Username          string
	PrincipalUsername string
	PrincipalWallet   string
	AgentWallet       string
	SessionPublicKey  string
	SessionKeyType    string
	Scopes            []string
	DeviceLabel       string
	IdleTimeoutHours  int
	AbsoluteTTLHours  int
	TokenTTLHours     int
}

func (h *Handler) agentAccessLeaseRepo() *storageRepos.AgentAccessLeaseRepository {
	if h == nil || h.repos == nil {
		return nil
	}

	return storageRepos.NewAgentAccessLeaseRepository(h.repos.GetDB(), h.repos.GetTableName(), h.logger)
}

// HandleCreateAgentAccessLeasePrincipalChallengeLift issues the owner approval challenge.
func (h *Handler) HandleCreateAgentAccessLeasePrincipalChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleCreateAgentAccessLeaseChallenge(ctx, agentAccessLeaseActionPrincipal)
}

// HandleCreateAgentAccessLeaseAgentChallengeLift issues the agent acceptance challenge.
func (h *Handler) HandleCreateAgentAccessLeaseAgentChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleCreateAgentAccessLeaseChallenge(ctx, agentAccessLeaseActionAgent)
}

// HandleCreateAgentAccessLeaseLift finalizes a wallet-backed access lease.
//
//nolint:gocognit,gocyclo // Enrollment verifies two challenges, two signers, and lease bounds in one request path.
func (h *Handler) HandleCreateAgentAccessLeaseLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, account, resp, err := h.requireOwnedAgentAccount(ctx, username)
	if resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.CreateAgentAccessLeaseRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	req.PrincipalChallengeID = strings.TrimSpace(req.PrincipalChallengeID)
	req.PrincipalSignature = strings.TrimSpace(req.PrincipalSignature)
	req.AgentChallengeID = strings.TrimSpace(req.AgentChallengeID)
	req.AgentSignature = strings.TrimSpace(req.AgentSignature)

	if err := common.ValidateRequiredParam("principal_challenge_id", req.PrincipalChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("principal_signature", req.PrincipalSignature); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("agent_challenge_id", req.AgentChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("agent_signature", req.AgentSignature); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	principalChallenge, resp, err := h.loadAgentAccessLeaseChallenge(ctx, req.PrincipalChallengeID)
	if resp != nil || err != nil {
		return resp, err
	}
	agentChallenge, resp, err := h.loadAgentAccessLeaseChallenge(ctx, req.AgentChallengeID)
	if resp != nil || err != nil {
		return resp, err
	}

	if !agentAccessLeaseChallengesMatch(principalChallenge, agentChallenge) {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge pair does not match",
		})
	}
	if principalChallenge.Action != agentAccessLeaseActionPrincipal || agentChallenge.Action != agentAccessLeaseActionAgent {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge actions do not match enrollment flow",
		})
	}
	if !strings.EqualFold(principalChallenge.PrincipalUsername, claims.Username) {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge principal does not match authenticated owner",
		})
	}

	if usedBoundSoul, principalOK, agentOK, walletErr := h.validateBoundAgentAccessLeaseWallets(ctx, username, principalChallenge.PrincipalWallet, principalChallenge.AgentWallet); walletErr != nil {
		return common.RespondInternalServerError(ctx)
	} else if usedBoundSoul {
		if !principalOK {
			return common.RespondForbidden(ctx, "principal wallet does not match the bound soul principal")
		}
		if !agentOK {
			return common.RespondForbidden(ctx, "agent wallet does not match the bound soul body wallet")
		}
	} else {
		if ok, walletErr := h.userHasWallet(ctx, claims.Username, principalChallenge.PrincipalWallet); walletErr != nil {
			return common.RespondInternalServerError(ctx)
		} else if !ok {
			return common.RespondForbidden(ctx, "principal wallet is not linked to the owner account")
		}
		if ok, walletErr := h.userHasWallet(ctx, username, principalChallenge.AgentWallet); walletErr != nil {
			return common.RespondInternalServerError(ctx)
		} else if !ok {
			return common.RespondForbidden(ctx, "agent wallet is not linked to the agent account")
		}
	}

	if err := h.verifyLeaseChallengeSignature(ctx, principalChallenge, req.PrincipalSignature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": err.Error(),
		})
	}
	if err := h.verifyLeaseChallengeSignature(ctx, agentChallenge, req.AgentSignature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": err.Error(),
		})
	}

	now := time.Now().UTC()
	idleExpiresAt, absoluteExpiresAt := computeLeaseExpiries(now, principalChallenge.IdleTimeoutHours, principalChallenge.AbsoluteTTLHours)
	model := &storageModels.AgentAccessLease{
		ID:                principalChallenge.LeaseID,
		Username:          username,
		PrincipalUsername: claims.Username,
		PrincipalWallet:   principalChallenge.PrincipalWallet,
		AgentWallet:       principalChallenge.AgentWallet,
		Scopes:            append([]string(nil), principalChallenge.Scopes...),
		DeviceLabel:       principalChallenge.DeviceLabel,
		Status:            agentAccessLeaseStatusActive,
		IdleTimeoutHours:  principalChallenge.IdleTimeoutHours,
		TokenTTLHours:     principalChallenge.EffectiveTokenTTLHours(),
		IdleExpiresAt:     idleExpiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		LastUsedAt:        now,
		LeaseVersion:      1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := h.markAgentAccessLeaseChallengeUsed(ctx, principalChallenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "principal challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}
	if err := h.markAgentAccessLeaseChallengeUsed(ctx, agentChallenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "agent challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}

	repo := h.agentAccessLeaseRepo()
	if repo == nil {
		return common.RespondInternalServerError(ctx)
	}
	if err := repo.CreateLease(ctx.Context(), model); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return common.RespondConflict(ctx, "lease already exists")
		}
		return common.RespondInternalServerError(ctx)
	}

	h.recordAgentGovernanceEvent(ctx, username, "agent.access_lease.created", map[string]any{
		"lease_id":         model.ID,
		"principal_wallet": model.PrincipalWallet,
		"agent_wallet":     model.AgentWallet,
		"scopes":           model.Scopes,
		"idle_hours":       model.IdleTimeoutHours,
		"token_ttl_hours":  model.EffectiveTokenTTLHours(),
		"absolute_expires": model.AbsoluteExpiresAt.Format(time.RFC3339),
	})
	h.ensureAgentActor(username, account)
	return okJSON(agentAccessLeaseResponse(model, now))
}

// HandleListAgentAccessLeasesLift lists access leases for an agent.
func (h *Handler) HandleListAgentAccessLeasesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	if _, _, resp, err := h.requireManagedAgentAccount(ctx, username); resp != nil || err != nil {
		return resp, err
	}

	leases, err := h.listAgentAccessLeases(ctx, username)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	now := time.Now().UTC()
	out := make([]apimodels.AgentAccessLease, 0, len(leases))
	for i := range leases {
		out = append(out, agentAccessLeaseResponse(&leases[i], now))
	}
	return okJSON(apimodels.AgentAccessLeaseListResponse{Leases: out})
}

// HandleRevokeAgentAccessLeaseLift revokes a lease.
func (h *Handler) HandleRevokeAgentAccessLeaseLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	leaseID := strings.TrimSpace(ctx.Param("leaseID"))
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, _, resp, err := h.requireManagedAgentAccount(ctx, username)
	if resp != nil || err != nil {
		return resp, err
	}

	lease, resp, err := h.loadAgentAccessLease(ctx, username, leaseID)
	if resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.RevokeAgentAccessLeaseRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil && strings.TrimSpace(string(ctx.Request.Body)) != "" {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	now := time.Now().UTC()
	if effectiveAgentAccessLeaseStatus(lease, now) == agentAccessLeaseStatusRevoked {
		return okJSON(agentAccessLeaseResponse(lease, now))
	}
	reason := strings.TrimSpace(req.Reason)
	repo := h.agentAccessLeaseRepo()
	if err := repo.RevokeLease(ctx.Context(), lease, claims.Username, reason, now); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	lease.Status = agentAccessLeaseStatusRevoked
	lease.UpdatedAt = now
	lease.RevokedAt = now
	lease.RevokedBy = claims.Username
	lease.RevokedReason = reason
	h.recordAgentGovernanceEvent(ctx, username, "agent.access_lease.revoked", map[string]any{
		"lease_id": lease.ID,
		"reason":   reason,
	})
	return okJSON(agentAccessLeaseResponse(lease, now))
}

// HandleCreateAgentAccessLeaseSessionKeyChallengeLift issues a wallet-signed challenge that authorizes a session key.
func (h *Handler) HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	leaseID := strings.TrimSpace(ctx.Param("leaseID"))
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	lease, resp, err := h.loadAgentAccessLease(ctx, username, leaseID)
	if resp != nil || err != nil {
		return resp, err
	}
	if resp, err := h.ensureActiveAgentLeaseAccount(ctx, username); resp != nil || err != nil {
		return resp, err
	}
	now := time.Now().UTC()
	if effectiveAgentAccessLeaseStatus(lease, now) != agentAccessLeaseStatusActive {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_lease",
			"error_description": "lease is not active",
		})
	}

	var req apimodels.AgentAccessLeaseSessionKeyChallengeRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	sessionPublicKey, err := normalizeAgentAccessSessionPublicKey(req.SessionPublicKey)
	if err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	challenge, err := h.createAgentAccessLeaseChallenge(ctx, agentAccessLeaseOptions{
		LeaseID:           lease.ID,
		Username:          lease.Username,
		PrincipalUsername: lease.PrincipalUsername,
		PrincipalWallet:   lease.PrincipalWallet,
		AgentWallet:       lease.AgentWallet,
		SessionPublicKey:  sessionPublicKey,
		SessionKeyType:    agentAccessLeaseSessionKeyType,
		Scopes:            append([]string(nil), lease.Scopes...),
		DeviceLabel:       lease.DeviceLabel,
		IdleTimeoutHours:  lease.IdleTimeoutHours,
		AbsoluteTTLHours:  max(1, int(time.Until(lease.AbsoluteExpiresAt).Hours())),
	}, agentAccessLeaseActionSessionKeyAuth)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentAccessLeaseChallengeResponse(challenge))
}

// HandleAuthorizeAgentAccessLeaseSessionKeyLift authorizes a session key using the agent wallet.
func (h *Handler) HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	leaseID := strings.TrimSpace(ctx.Param("leaseID"))
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	lease, resp, err := h.loadAgentAccessLease(ctx, username, leaseID)
	if resp != nil || err != nil {
		return resp, err
	}
	if resp, err := h.ensureActiveAgentLeaseAccount(ctx, username); resp != nil || err != nil {
		return resp, err
	}
	now := time.Now().UTC()
	if effectiveAgentAccessLeaseStatus(lease, now) != agentAccessLeaseStatusActive {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_lease",
			"error_description": "lease is not active",
		})
	}

	var req apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	req.ChallengeID = strings.TrimSpace(req.ChallengeID)
	req.Signature = strings.TrimSpace(req.Signature)
	if err := common.ValidateRequiredParam("challenge_id", req.ChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	challenge, resp, err := h.loadAgentAccessLeaseChallenge(ctx, req.ChallengeID)
	if resp != nil || err != nil {
		return resp, err
	}
	if challenge.Action != agentAccessLeaseActionSessionKeyAuth || !strings.EqualFold(challenge.LeaseID, leaseID) || !strings.EqualFold(challenge.Username, username) {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge does not match lease",
		})
	}

	if err := h.verifyLeaseChallengeSignature(ctx, challenge, req.Signature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": err.Error(),
		})
	}
	if err := h.markAgentAccessLeaseChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}
	repo := h.agentAccessLeaseRepo()
	if err := repo.AuthorizeSessionKey(ctx.Context(), lease, challenge.SessionPublicKey, agentAccessLeaseSessionKeyType, now); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	lease.SessionPublicKey = challenge.SessionPublicKey
	lease.SessionKeyType = agentAccessLeaseSessionKeyType
	lease.SessionKeyCreatedAt = now
	lease.UpdatedAt = now
	h.recordAgentGovernanceEvent(ctx, username, "agent.access_lease.session_key_authorized", map[string]any{
		"lease_id":           lease.ID,
		"session_key_type":   agentAccessLeaseSessionKeyType,
		"session_public_key": challenge.SessionPublicKey,
	})
	return okJSON(agentAccessLeaseResponse(lease, now))
}

// HandleCreateAgentAccessLeaseRenewChallengeLift issues a renewal challenge for an active lease.
func (h *Handler) HandleCreateAgentAccessLeaseRenewChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	leaseID := strings.TrimSpace(ctx.Param("leaseID"))
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	lease, resp, err := h.loadAgentAccessLease(ctx, username, leaseID)
	if resp != nil || err != nil {
		return resp, err
	}
	if resp, err := h.ensureActiveAgentLeaseAccount(ctx, username); resp != nil || err != nil {
		return resp, err
	}

	now := time.Now().UTC()
	if effectiveAgentAccessLeaseStatus(lease, now) != agentAccessLeaseStatusActive {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_lease",
			"error_description": "lease is not active",
		})
	}

	action := agentAccessLeaseActionRenewWallet
	sessionPublicKey := ""
	sessionKeyType := ""
	if strings.TrimSpace(lease.SessionPublicKey) != "" && strings.EqualFold(strings.TrimSpace(lease.SessionKeyType), agentAccessLeaseSessionKeyType) {
		action = agentAccessLeaseActionRenewSession
		sessionPublicKey = lease.SessionPublicKey
		sessionKeyType = lease.SessionKeyType
	}

	challenge, err := h.createAgentAccessLeaseChallenge(ctx, agentAccessLeaseOptions{
		LeaseID:           lease.ID,
		Username:          lease.Username,
		PrincipalUsername: lease.PrincipalUsername,
		PrincipalWallet:   lease.PrincipalWallet,
		AgentWallet:       lease.AgentWallet,
		SessionPublicKey:  sessionPublicKey,
		SessionKeyType:    sessionKeyType,
		Scopes:            append([]string(nil), lease.Scopes...),
		DeviceLabel:       lease.DeviceLabel,
		IdleTimeoutHours:  lease.IdleTimeoutHours,
		AbsoluteTTLHours:  int(time.Until(lease.AbsoluteExpiresAt).Hours()),
	}, action)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(agentAccessLeaseChallengeResponse(challenge))
}

// HandleExchangeAgentAccessLeaseTokenLift exchanges a renewal proof for a short-lived token.
func (h *Handler) HandleExchangeAgentAccessLeaseTokenLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	leaseID := strings.TrimSpace(ctx.Param("leaseID"))
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	var req apimodels.RenewAgentAccessLeaseTokenRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	req.ChallengeID = strings.TrimSpace(req.ChallengeID)
	req.Signature = strings.TrimSpace(req.Signature)
	if err := common.ValidateRequiredParam("challenge_id", req.ChallengeID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	lease, resp, err := h.loadAgentAccessLease(ctx, username, leaseID)
	if resp != nil || err != nil {
		return resp, err
	}
	if resp, err := h.ensureActiveAgentLeaseAccount(ctx, username); resp != nil || err != nil {
		return resp, err
	}
	now := time.Now().UTC()
	if effectiveAgentAccessLeaseStatus(lease, now) != agentAccessLeaseStatusActive {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_lease",
			"error_description": "lease is not active",
		})
	}

	challenge, resp, err := h.loadAgentAccessLeaseChallenge(ctx, req.ChallengeID)
	if resp != nil || err != nil {
		return resp, err
	}
	if !strings.EqualFold(challenge.LeaseID, leaseID) || !strings.EqualFold(challenge.Username, username) {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge does not match lease",
		})
	}
	if challenge.Action != agentAccessLeaseActionRenewWallet && challenge.Action != agentAccessLeaseActionRenewSession {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_challenge",
			"error_description": "challenge action does not match renewal flow",
		})
	}
	if err := h.verifyLeaseChallengeSignature(ctx, challenge, req.Signature); err != nil {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_signature",
			"error_description": err.Error(),
		})
	}
	if err := h.markAgentAccessLeaseChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return apptheory.JSON(http.StatusUnauthorized, map[string]any{
				"error":             "invalid_challenge",
				"error_description": "challenge already used or expired",
			})
		}
		return common.RespondInternalServerError(ctx)
	}

	newIdleExpiresAt := now.Add(time.Duration(lease.IdleTimeoutHours) * time.Hour)
	if newIdleExpiresAt.After(lease.AbsoluteExpiresAt) {
		newIdleExpiresAt = lease.AbsoluteExpiresAt
	}
	tokenExpiresAt := storageModels.AgentAccessLeaseTokenExpiresAt(now, lease, newIdleExpiresAt)
	remaining := time.Until(tokenExpiresAt)
	if remaining <= 0 {
		return apptheory.JSON(http.StatusUnauthorized, map[string]any{
			"error":             "invalid_lease",
			"error_description": "lease expired",
		})
	}
	accessTTL := remaining

	accessToken, err := h.mintAgentAccessLeaseToken(ctx, lease, accessTTL)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	repo := h.agentAccessLeaseRepo()
	if err := repo.RecordLeaseUse(ctx.Context(), lease, newIdleExpiresAt, now, challenge.Action == agentAccessLeaseActionRenewSession); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	lease.LastUsedAt = now
	lease.IdleExpiresAt = newIdleExpiresAt
	lease.UpdatedAt = now
	if challenge.Action == agentAccessLeaseActionRenewSession {
		lease.SessionKeyLastUsedAt = now
	}
	h.recordAgentGovernanceEvent(ctx, username, "agent.access_lease.renewed", map[string]any{
		"lease_id":    lease.ID,
		"signer_kind": challenge.Action,
		"expires_in":  int(accessTTL.Seconds()),
		"token_hours": lease.EffectiveTokenTTLHours(),
	})
	return okJSON(apimodels.AgentAccessLeaseTokenResponse{
		LeaseID: lease.ID,
		Token: apimodels.OAuthTokenResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   int(accessTTL.Seconds()),
			Scope:       strings.Join(lease.Scopes, " "),
			CreatedAt:   now.Unix(),
		},
	})
}

func (h *Handler) mintAgentAccessLeaseToken(ctx *apptheory.Context, lease *storageModels.AgentAccessLease, accessTTL time.Duration) (string, error) {
	if h.cfg == nil || lease == nil || (strings.TrimSpace(h.cfg.JWTSecret) == "" && strings.TrimSpace(h.cfg.JWTSecretARN) == "") {
		return "", errors.New("agent access lease token configuration unavailable")
	}
	oauthSvc, err := createOAuthService("", h.cfg, h.repos, h.logger)
	if err != nil {
		return "", err
	}
	accessToken, _, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContext(
		ctx.Context(),
		lease.Username,
		agentAccessLeaseClientID,
		"",
		lease.Scopes,
		accessTTL,
		"",
		"",
	)
	if err != nil {
		return "", err
	}
	return accessToken, nil
}

func (h *Handler) handleCreateAgentAccessLeaseChallenge(ctx *apptheory.Context, action string) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, _, resp, err := h.requireOwnedAgentAccount(ctx, username)
	if resp != nil || err != nil {
		return resp, err
	}

	var req apimodels.AgentAccessLeaseChallengeRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	requestedScopes, resp, err := h.validateDelegationScopes(ctx, claims.Scopes, req.Scopes)
	if resp != nil || err != nil {
		return resp, err
	}
	governance, err := requireAgentGovernanceState(ctx.Context(), h.repos, username)
	if err != nil {
		return respondAgentGovernanceUnavailable(ctx)
	}
	if err := validateDelegationAgainstAgentEnvelope(governance, requestedScopes); err != nil {
		return common.RespondForbidden(ctx, err.Error())
	}

	opts, err := normalizeAgentAccessLeaseOptions(
		req.LeaseID,
		username,
		claims.Username,
		req.PrincipalWallet,
		req.AgentWallet,
		req.SessionPublicKey,
		requestedScopes,
		req.DeviceLabel,
		req.IdleTimeoutHours,
		req.AbsoluteTTLHours,
		req.TokenTTLHours,
		action == agentAccessLeaseActionAgent,
	)
	if err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	if usedBoundSoul, principalOK, agentOK, walletErr := h.validateBoundAgentAccessLeaseWallets(ctx, username, opts.PrincipalWallet, opts.AgentWallet); walletErr != nil {
		return common.RespondInternalServerError(ctx)
	} else if usedBoundSoul {
		if !principalOK {
			return common.RespondForbidden(ctx, "principal wallet does not match the bound soul principal")
		}
		if !agentOK {
			return common.RespondForbidden(ctx, "agent wallet does not match the bound soul body wallet")
		}
	} else {
		if ok, walletErr := h.userHasWallet(ctx, claims.Username, opts.PrincipalWallet); walletErr != nil {
			return common.RespondInternalServerError(ctx)
		} else if !ok {
			return common.RespondForbidden(ctx, "principal wallet is not linked to the owner account")
		}
		if ok, walletErr := h.userHasWallet(ctx, username, opts.AgentWallet); walletErr != nil {
			return common.RespondInternalServerError(ctx)
		} else if !ok {
			return common.RespondForbidden(ctx, "agent wallet is not linked to the agent account")
		}
	}

	challenge, err := h.createAgentAccessLeaseChallenge(ctx, opts, action)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	return okJSON(agentAccessLeaseChallengeResponse(challenge))
}

func (h *Handler) requireOwnedAgentAccount(ctx *apptheory.Context, username string) (*auth.Claims, *storage.Account, *apptheory.Response, error) {
	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return nil, nil, resp, err
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil {
		if common.IsNotFound(err) {
			resp, respErr := common.RespondNotFound(ctx, "agent")
			return nil, nil, resp, respErr
		}
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, nil, resp, respErr
	}
	if account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		resp, respErr := common.RespondNotFound(ctx, "agent")
		return nil, nil, resp, respErr
	}

	owner := strings.TrimPrefix(strings.TrimSpace(account.User.AgentOwner), "@")
	if !strings.EqualFold(owner, claims.Username) {
		resp, respErr := common.RespondForbidden(ctx, "not authorized to manage agent lease enrollment")
		return nil, nil, resp, respErr
	}
	return claims, account, nil, nil
}

func (h *Handler) requireManagedAgentAccount(ctx *apptheory.Context, username string) (*auth.Claims, *storage.Account, *apptheory.Response, error) {
	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return nil, nil, resp, err
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil {
		if common.IsNotFound(err) {
			resp, respErr := common.RespondNotFound(ctx, "agent")
			return nil, nil, resp, respErr
		}
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, nil, resp, respErr
	}
	if account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		resp, respErr := common.RespondNotFound(ctx, "agent")
		return nil, nil, resp, respErr
	}
	if !h.isAgentOwnerOrAdmin(claims, account.User) {
		resp, respErr := common.RespondForbidden(ctx, "not authorized to manage agent leases")
		return nil, nil, resp, respErr
	}
	return claims, account, nil, nil
}

func (h *Handler) ensureActiveAgentLeaseAccount(ctx *apptheory.Context, username string) (*apptheory.Response, error) {
	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil {
		if common.IsNotFound(err) {
			return common.RespondNotFound(ctx, "agent")
		}
		return common.RespondInternalServerError(ctx)
	}
	if account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return common.RespondNotFound(ctx, "agent")
	}
	return nil, nil
}

func (h *Handler) createAgentAccessLeaseChallenge(ctx *apptheory.Context, opts agentAccessLeaseOptions, action string) (*storageModels.AgentAccessLeaseChallenge, error) {
	repo := h.agentAccessLeaseRepo()
	if repo == nil {
		return nil, errors.New("storage not initialized")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(agentAccessLeaseChallengeTTL)

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	id := common.GenerateOperationIDULID()
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)

	address := opts.PrincipalWallet
	if action == agentAccessLeaseActionAgent || action == agentAccessLeaseActionRenewWallet || action == agentAccessLeaseActionSessionKeyAuth {
		address = opts.AgentWallet
	}
	if action == agentAccessLeaseActionRenewSession {
		address = ""
	}

	message := buildAgentAccessLeaseChallengeMessage(id, opts, action, nonce, now, expiresAt)
	model := &storageModels.AgentAccessLeaseChallenge{
		ID:                id,
		LeaseID:           opts.LeaseID,
		Username:          opts.Username,
		Action:            action,
		Address:           address,
		PrincipalUsername: opts.PrincipalUsername,
		PrincipalWallet:   opts.PrincipalWallet,
		AgentWallet:       opts.AgentWallet,
		SessionPublicKey:  opts.SessionPublicKey,
		SessionKeyType:    opts.SessionKeyType,
		Scopes:            append([]string(nil), opts.Scopes...),
		DeviceLabel:       opts.DeviceLabel,
		IdleTimeoutHours:  opts.IdleTimeoutHours,
		AbsoluteTTLHours:  opts.AbsoluteTTLHours,
		TokenTTLHours:     opts.TokenTTLHours,
		Nonce:             nonce,
		Message:           message,
		IssuedAt:          now,
		ExpiresAt:         expiresAt,
		Used:              false,
	}

	if err := repo.CreateChallenge(ctx.Context(), model); err != nil {
		return nil, err
	}
	return model, nil
}

func (h *Handler) loadAgentAccessLeaseChallenge(ctx *apptheory.Context, challengeID string) (*storageModels.AgentAccessLeaseChallenge, *apptheory.Response, error) {
	repo := h.agentAccessLeaseRepo()
	if repo == nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}

	challenge, err := repo.GetChallenge(ctx.Context(), challengeID)
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

	now := time.Now().UTC()
	if challenge.ExpiresAt.IsZero() || challenge.TTL <= now.Unix() || now.After(challenge.ExpiresAt) {
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

func (h *Handler) markAgentAccessLeaseChallengeUsed(ctx *apptheory.Context, challengeID string) error {
	repo := h.agentAccessLeaseRepo()
	if repo == nil {
		return errors.New("storage not initialized")
	}

	return repo.MarkChallengeUsed(ctx.Context(), challengeID, time.Now().UTC())
}

func (h *Handler) verifyLeaseChallengeSignature(_ *apptheory.Context, challenge *storageModels.AgentAccessLeaseChallenge, signature string) error {
	if challenge == nil {
		return errors.New("signature verification unavailable")
	}

	switch challenge.Action {
	case agentAccessLeaseActionPrincipal, agentAccessLeaseActionAgent, agentAccessLeaseActionSessionKeyAuth, agentAccessLeaseActionRenewWallet:
		return verifyAgentAccessLeaseTypedDataSignature(challenge.Address, buildAgentAccessLeaseTypedData(challenge), signature)
	case agentAccessLeaseActionRenewSession:
		return verifyAgentAccessLeaseSessionSignature(challenge.SessionPublicKey, challenge.Message, signature)
	default:
		return errors.New("unsupported challenge action")
	}
}

func (h *Handler) loadAgentAccessLease(ctx *apptheory.Context, username, leaseID string) (*storageModels.AgentAccessLease, *apptheory.Response, error) {
	repo := h.agentAccessLeaseRepo()
	if repo == nil {
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}

	lease, err := repo.GetLease(ctx.Context(), username, leaseID)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			resp, respErr := common.RespondNotFound(ctx, "agent access lease")
			return nil, resp, respErr
		}
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}
	return lease, nil, nil
}

func (h *Handler) listAgentAccessLeases(ctx *apptheory.Context, username string) ([]storageModels.AgentAccessLease, error) {
	repo := h.agentAccessLeaseRepo()
	if repo == nil {
		return nil, errors.New("storage not initialized")
	}

	return repo.ListLeases(ctx.Context(), username)
}

func (h *Handler) userHasWallet(ctx *apptheory.Context, username, address string) (bool, error) {
	if h == nil || h.repos == nil || h.repos.Account() == nil {
		return false, errors.New("account repository unavailable")
	}

	address = strings.TrimSpace(strings.ToLower(address))
	if address == "" {
		return false, nil
	}

	wallets, err := h.repos.Account().GetUserWalletCredentials(ctx.Context(), username)
	if err != nil {
		return false, err
	}
	for _, wallet := range wallets {
		if wallet != nil && strings.EqualFold(strings.TrimSpace(wallet.Address), address) {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) validateBoundAgentAccessLeaseWallets(ctx *apptheory.Context, agentUsername, principalWallet, agentWallet string) (bool, bool, bool, error) {
	service := h.getSoulService()
	if service == nil {
		return false, false, false, nil
	}

	soul, err := service.ResolveBoundAgent(ctx.Context(), agentUsername)
	if err != nil {
		return false, false, false, err
	}
	if soul == nil || !soul.Bound {
		return false, false, false, nil
	}

	expectedPrincipal := boundSoulLeasePrincipalWallet(soul)
	expectedAgent := strings.TrimSpace(strings.ToLower(soul.Wallet))
	normalizedPrincipal := strings.TrimSpace(strings.ToLower(principalWallet))
	normalizedAgent := strings.TrimSpace(strings.ToLower(agentWallet))

	return true,
		expectedPrincipal != "" && strings.EqualFold(expectedPrincipal, normalizedPrincipal),
		expectedAgent != "" && strings.EqualFold(expectedAgent, normalizedAgent),
		nil
}

func boundSoulLeasePrincipalWallet(soul *soulservice.Soul) string {
	if soul == nil {
		return ""
	}

	if principal := strings.TrimSpace(strings.ToLower(soul.BoundPrincipalAddress)); principal != "" {
		return principal
	}
	if principal := strings.TrimSpace(strings.ToLower(soul.PrincipalAddress)); principal != "" {
		return principal
	}
	return strings.TrimSpace(strings.ToLower(soul.Wallet))
}

func normalizeAgentAccessLeaseOptions(
	leaseID string,
	username string,
	principalUsername string,
	principalWallet string,
	agentWallet string,
	sessionPublicKey string,
	scopes []string,
	deviceLabel string,
	idleTimeoutHours int,
	absoluteTTLHours int,
	tokenTTLHours int,
	requireLeaseID bool,
) (agentAccessLeaseOptions, error) {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		if requireLeaseID {
			return agentAccessLeaseOptions{}, errors.New("lease_id is required")
		}
		leaseID = common.GenerateOperationIDULID()
	}

	principalWallet, err := normalizeEthLeaseAddress(principalWallet)
	if err != nil {
		return agentAccessLeaseOptions{}, fmt.Errorf("principal_wallet %w", err)
	}
	agentWallet, err = normalizeEthLeaseAddress(agentWallet)
	if err != nil {
		return agentAccessLeaseOptions{}, fmt.Errorf("agent_wallet %w", err)
	}
	sessionKeyType := ""
	if strings.TrimSpace(sessionPublicKey) != "" {
		sessionPublicKey, err = normalizeAgentAccessSessionPublicKey(sessionPublicKey)
		if err != nil {
			return agentAccessLeaseOptions{}, err
		}
		sessionKeyType = agentAccessLeaseSessionKeyType
	}
	if deviceLabel = strings.TrimSpace(deviceLabel); deviceLabel == "" {
		deviceLabel = "local-agent"
	}
	if idleTimeoutHours <= 0 {
		idleTimeoutHours = agentAccessLeaseDefaultIdleHrs
	}
	if idleTimeoutHours > agentAccessLeaseMaxIdleHrs {
		idleTimeoutHours = agentAccessLeaseMaxIdleHrs
	}
	if absoluteTTLHours <= 0 {
		absoluteTTLHours = agentAccessLeaseDefaultAbsHrs
	}
	if absoluteTTLHours > agentAccessLeaseMaxAbsHrs {
		absoluteTTLHours = agentAccessLeaseMaxAbsHrs
	}
	if absoluteTTLHours < idleTimeoutHours {
		absoluteTTLHours = idleTimeoutHours
	}
	tokenTTLHours = storageModels.NormalizeAgentAccessLeaseTokenTTLHours(idleTimeoutHours, absoluteTTLHours, tokenTTLHours)
	normalizedScopes := append([]string(nil), scopes...)
	sort.Strings(normalizedScopes)

	return agentAccessLeaseOptions{
		LeaseID:           leaseID,
		Username:          strings.TrimSpace(username),
		PrincipalUsername: strings.TrimSpace(principalUsername),
		PrincipalWallet:   principalWallet,
		AgentWallet:       agentWallet,
		SessionPublicKey:  sessionPublicKey,
		SessionKeyType:    sessionKeyType,
		Scopes:            normalizedScopes,
		DeviceLabel:       deviceLabel,
		IdleTimeoutHours:  idleTimeoutHours,
		AbsoluteTTLHours:  absoluteTTLHours,
		TokenTTLHours:     tokenTTLHours,
	}, nil
}

func normalizeEthLeaseAddress(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", errors.New("is required")
	}
	if !strings.HasPrefix(raw, "0x") || len(raw) != 42 {
		return "", errors.New("must be a 0x-prefixed 20-byte ethereum address")
	}
	for _, ch := range raw[2:] {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		default:
			return "", errors.New("must be hex encoded")
		}
	}
	return raw, nil
}

func normalizeAgentAccessSessionPublicKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("session_public_key is required")
	}
	if _, err := auth.ParseAgentPublicKey(agentAccessLeaseSessionKeyType, raw); err != nil {
		return "", errors.New("session_public_key must be a valid ed25519 public key")
	}
	return raw, nil
}

func buildAgentAccessLeaseChallengeMessage(id string, opts agentAccessLeaseOptions, action string, nonce string, issuedAt, expiresAt time.Time) string {
	return fmt.Sprintf(
		"LESSER AGENT ACCESS LEASE\nid: %s\nlease_id: %s\naction: %s\ndomain: %s\nprincipal_username: %s\nagent_username: %s\nprincipal_wallet: %s\nagent_wallet: %s\nsession_public_key: %s\nscopes: %s\ndevice_label: %s\nidle_timeout_hours: %d\nabsolute_ttl_hours: %d\ntoken_ttl_hours: %d\nnonce: %s\nissued_at: %s\nexpires_at: %s",
		strings.TrimSpace(id),
		strings.TrimSpace(opts.LeaseID),
		strings.TrimSpace(action),
		strings.TrimSpace(humanReadableAccessLeaseDomain()),
		strings.TrimSpace(opts.PrincipalUsername),
		strings.TrimSpace(opts.Username),
		strings.TrimSpace(opts.PrincipalWallet),
		strings.TrimSpace(opts.AgentWallet),
		strings.TrimSpace(opts.SessionPublicKey),
		strings.Join(opts.Scopes, " "),
		strings.TrimSpace(opts.DeviceLabel),
		opts.IdleTimeoutHours,
		opts.AbsoluteTTLHours,
		opts.TokenTTLHours,
		strings.TrimSpace(nonce),
		issuedAt.UTC().Format(time.RFC3339),
		expiresAt.UTC().Format(time.RFC3339),
	)
}

func humanReadableAccessLeaseDomain() string {
	cfg := config.Get()
	if cfg == nil {
		return "lesser"
	}
	domain := strings.TrimSpace(cfg.Domain)
	if domain == "" {
		domain = strings.TrimSpace(cfg.BaseURL())
	}
	if domain == "" {
		return "lesser"
	}
	return domain
}

func buildAgentAccessLeaseTypedData(challenge *storageModels.AgentAccessLeaseChallenge) apitypes.TypedData {
	domain := apitypes.TypedDataDomain{
		Name:    "Lesser Agent Access Lease",
		Version: agentAccessLeaseTypedDataVersion,
	}
	message := apitypes.TypedDataMessage{
		"id":                strings.TrimSpace(challenge.ID),
		"leaseId":           strings.TrimSpace(challenge.LeaseID),
		"action":            strings.TrimSpace(challenge.Action),
		"instanceDomain":    humanReadableAccessLeaseDomain(),
		"principalUsername": strings.TrimSpace(challenge.PrincipalUsername),
		"agentUsername":     strings.TrimSpace(challenge.Username),
		"principalWallet":   strings.TrimSpace(challenge.PrincipalWallet),
		"agentWallet":       strings.TrimSpace(challenge.AgentWallet),
		"sessionPublicKey":  strings.TrimSpace(challenge.SessionPublicKey),
		"scopes":            strings.Join(challenge.Scopes, " "),
		"deviceLabel":       strings.TrimSpace(challenge.DeviceLabel),
		"idleTimeoutHours":  fmt.Sprintf("%d", challenge.IdleTimeoutHours),
		"absoluteTTLHours":  fmt.Sprintf("%d", challenge.AbsoluteTTLHours),
		"tokenTTLHours":     fmt.Sprintf("%d", challenge.EffectiveTokenTTLHours()),
		"nonce":             strings.TrimSpace(challenge.Nonce),
		"issuedAt":          challenge.IssuedAt.UTC().Format(time.RFC3339),
		"expiresAt":         challenge.ExpiresAt.UTC().Format(time.RFC3339),
	}
	return apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
			},
			"AgentAccessLeaseChallenge": {
				{Name: "id", Type: "string"},
				{Name: "leaseId", Type: "string"},
				{Name: "action", Type: "string"},
				{Name: "instanceDomain", Type: "string"},
				{Name: "principalUsername", Type: "string"},
				{Name: "agentUsername", Type: "string"},
				{Name: "principalWallet", Type: "string"},
				{Name: "agentWallet", Type: "string"},
				{Name: "sessionPublicKey", Type: "string"},
				{Name: "scopes", Type: "string"},
				{Name: "deviceLabel", Type: "string"},
				{Name: "idleTimeoutHours", Type: "string"},
				{Name: "absoluteTTLHours", Type: "string"},
				{Name: "tokenTTLHours", Type: "string"},
				{Name: "nonce", Type: "string"},
				{Name: "issuedAt", Type: "string"},
				{Name: "expiresAt", Type: "string"},
			},
		},
		PrimaryType: "AgentAccessLeaseChallenge",
		Domain:      domain,
		Message:     message,
	}
}

func verifyAgentAccessLeaseTypedDataSignature(expectedAddress string, typedData apitypes.TypedData, signature string) error {
	digest, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return errors.New("failed to hash typed data")
	}
	sig, err := hexutil.Decode(signature)
	if err != nil {
		return errors.New("invalid signature format")
	}
	if len(sig) != 65 {
		return errors.New("invalid signature length")
	}
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}
	pubKey, err := crypto.SigToPub(digest, sig)
	if err != nil {
		return errors.New("failed to recover signer")
	}
	recovered := strings.ToLower(strings.TrimPrefix(crypto.PubkeyToAddress(*pubKey).Hex(), "0x"))
	expected := strings.ToLower(strings.TrimPrefix(expectedAddress, "0x"))
	if recovered != expected {
		return errors.New("signature address mismatch")
	}
	return nil
}

func verifyAgentAccessLeaseSessionSignature(publicKey string, message string, signature string) error {
	if strings.TrimSpace(publicKey) == "" {
		return errors.New("session key is not configured")
	}
	pub, err := auth.ParseAgentPublicKey(agentAccessLeaseSessionKeyType, publicKey)
	if err != nil {
		return errors.New("invalid session key")
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		return errors.New("invalid session key type")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		sigBytes, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(signature))
	}
	if err != nil {
		sigBytes, err = base64.RawURLEncoding.DecodeString(strings.TrimSpace(signature))
	}
	if err != nil {
		return errors.New("invalid session signature format")
	}
	if !ed25519.Verify(ed, []byte(message), sigBytes) {
		return errors.New("invalid session signature")
	}
	return nil
}

func computeLeaseExpiries(now time.Time, idleTimeoutHours, absoluteTTLHours int) (time.Time, time.Time) {
	absoluteExpiresAt := now.Add(time.Duration(absoluteTTLHours) * time.Hour)
	idleExpiresAt := now.Add(time.Duration(idleTimeoutHours) * time.Hour)
	if idleExpiresAt.After(absoluteExpiresAt) {
		idleExpiresAt = absoluteExpiresAt
	}
	return idleExpiresAt, absoluteExpiresAt
}

func effectiveAgentAccessLeaseStatus(lease *storageModels.AgentAccessLease, now time.Time) string {
	if lease == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(lease.Status), agentAccessLeaseStatusRevoked) {
		return agentAccessLeaseStatusRevoked
	}
	if !lease.AbsoluteExpiresAt.IsZero() && now.After(lease.AbsoluteExpiresAt) {
		return agentAccessLeaseStatusExpired
	}
	if !lease.IdleExpiresAt.IsZero() && now.After(lease.IdleExpiresAt) {
		return agentAccessLeaseStatusExpired
	}
	return agentAccessLeaseStatusActive
}

func agentAccessLeaseChallengesMatch(a, b *storageModels.AgentAccessLeaseChallenge) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.LeaseID, b.LeaseID) &&
		strings.EqualFold(a.Username, b.Username) &&
		strings.EqualFold(a.PrincipalUsername, b.PrincipalUsername) &&
		strings.EqualFold(a.PrincipalWallet, b.PrincipalWallet) &&
		strings.EqualFold(a.AgentWallet, b.AgentWallet) &&
		strings.EqualFold(strings.Join(a.Scopes, " "), strings.Join(b.Scopes, " ")) &&
		a.DeviceLabel == b.DeviceLabel &&
		a.IdleTimeoutHours == b.IdleTimeoutHours &&
		a.AbsoluteTTLHours == b.AbsoluteTTLHours &&
		a.EffectiveTokenTTLHours() == b.EffectiveTokenTTLHours()
}

func agentAccessLeaseResponse(model *storageModels.AgentAccessLease, now time.Time) apimodels.AgentAccessLease {
	if model == nil {
		return apimodels.AgentAccessLease{}
	}

	var revokedAt *time.Time
	if !model.RevokedAt.IsZero() {
		value := model.RevokedAt
		revokedAt = &value
	}
	var sessionKeyCreatedAt *time.Time
	if !model.SessionKeyCreatedAt.IsZero() {
		value := model.SessionKeyCreatedAt
		sessionKeyCreatedAt = &value
	}
	var sessionKeyLastUsedAt *time.Time
	if !model.SessionKeyLastUsedAt.IsZero() {
		value := model.SessionKeyLastUsedAt
		sessionKeyLastUsedAt = &value
	}

	return apimodels.AgentAccessLease{
		ID:                   strings.TrimSpace(model.ID),
		Username:             strings.TrimSpace(model.Username),
		PrincipalUsername:    strings.TrimSpace(model.PrincipalUsername),
		PrincipalWallet:      strings.TrimSpace(model.PrincipalWallet),
		AgentWallet:          strings.TrimSpace(model.AgentWallet),
		Scopes:               append([]string(nil), model.Scopes...),
		DeviceLabel:          strings.TrimSpace(model.DeviceLabel),
		Status:               effectiveAgentAccessLeaseStatus(model, now),
		IdleTimeoutHours:     model.IdleTimeoutHours,
		TokenTTLHours:        model.EffectiveTokenTTLHours(),
		IdleExpiresAt:        model.IdleExpiresAt,
		AbsoluteExpiresAt:    model.AbsoluteExpiresAt,
		LastUsedAt:           model.LastUsedAt,
		LeaseVersion:         model.LeaseVersion,
		SessionPublicKey:     strings.TrimSpace(model.SessionPublicKey),
		SessionKeyType:       strings.TrimSpace(model.SessionKeyType),
		SessionKeyCreatedAt:  sessionKeyCreatedAt,
		SessionKeyLastUsedAt: sessionKeyLastUsedAt,
		CreatedAt:            model.CreatedAt,
		UpdatedAt:            model.UpdatedAt,
		RevokedAt:            revokedAt,
		RevokedBy:            strings.TrimSpace(model.RevokedBy),
		RevokedReason:        strings.TrimSpace(model.RevokedReason),
	}
}

func agentAccessLeaseChallengeResponse(model *storageModels.AgentAccessLeaseChallenge) apimodels.AgentAccessLeaseChallengeResponse {
	if model == nil {
		return apimodels.AgentAccessLeaseChallengeResponse{}
	}

	return apimodels.AgentAccessLeaseChallengeResponse{
		ID:               strings.TrimSpace(model.ID),
		LeaseID:          strings.TrimSpace(model.LeaseID),
		Username:         strings.TrimSpace(model.Username),
		Action:           strings.TrimSpace(model.Action),
		WalletAddress:    strings.TrimSpace(model.Address),
		PrincipalWallet:  strings.TrimSpace(model.PrincipalWallet),
		AgentWallet:      strings.TrimSpace(model.AgentWallet),
		SessionPublicKey: strings.TrimSpace(model.SessionPublicKey),
		SessionKeyType:   strings.TrimSpace(model.SessionKeyType),
		Scopes:           append([]string(nil), model.Scopes...),
		DeviceLabel:      strings.TrimSpace(model.DeviceLabel),
		IdleTimeoutHours: model.IdleTimeoutHours,
		AbsoluteTTLHours: model.AbsoluteTTLHours,
		TokenTTLHours:    model.EffectiveTokenTTLHours(),
		Message:          model.Message,
		TypedData:        challengeTypedDataResponse(model),
		IssuedAt:         model.IssuedAt,
		ExpiresAt:        model.ExpiresAt,
	}
}

func challengeTypedDataResponse(model *storageModels.AgentAccessLeaseChallenge) any {
	if model == nil {
		return nil
	}
	switch model.Action {
	case agentAccessLeaseActionPrincipal, agentAccessLeaseActionAgent, agentAccessLeaseActionSessionKeyAuth, agentAccessLeaseActionRenewWallet:
		return buildAgentAccessLeaseTypedData(model)
	default:
		return nil
	}
}
