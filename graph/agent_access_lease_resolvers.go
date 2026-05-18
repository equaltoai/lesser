package graph

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

const (
	graphAgentAccessLeaseClientID             = "lesser-agent-wallet-lease"
	graphAgentAccessLeaseChallengeTTL         = 5 * time.Minute
	graphAgentAccessLeaseActionPrincipal      = "principal_approve"
	graphAgentAccessLeaseActionAgent          = "agent_accept"
	graphAgentAccessLeaseActionRenewWallet    = "renew_wallet"
	graphAgentAccessLeaseActionRenewSession   = "renew_session"
	graphAgentAccessLeaseActionSessionKeyAuth = "session_key_authorize"
	graphAgentAccessLeaseStatusActive         = "active"
	graphAgentAccessLeaseStatusRevoked        = "revoked"
	graphAgentAccessLeaseDefaultIdleHrs       = 7 * 24
	graphAgentAccessLeaseMaxIdleHrs           = 30 * 24
	graphAgentAccessLeaseDefaultAbsHrs        = 90 * 24
	graphAgentAccessLeaseMaxAbsHrs            = 365 * 24
	graphAgentAccessLeaseSessionKeyType       = "ed25519"
	graphAgentAccessLeaseTypedDataVersion     = "1"
	graphAgentAccessLeaseStatusExpired        = "expired"
)

type graphAgentAccessLeaseOptions struct {
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

func (r *Resolver) agentAccessLeaseRepo() *storageRepos.AgentAccessLeaseRepository {
	if r == nil || r.Storage == nil {
		return nil
	}

	return storageRepos.NewAgentAccessLeaseRepository(r.Storage.GetDB(), r.Storage.GetTableName(), r.Logger)
}

// AgentAccessLeases is the resolver for the agentAccessLeases field.
func (r *queryResolver) AgentAccessLeases(ctx context.Context, username string) ([]*model.AgentAccessLease, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if _, _, err := r.requireManagedAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}

	leases, err := r.listGraphAgentAccessLeases(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to list agent access leases")
	}
	now := time.Now().UTC()
	out := make([]*model.AgentAccessLease, 0, len(leases))
	for i := range leases {
		item := graphAgentAccessLeaseModel(&leases[i], now)
		out = append(out, item)
	}
	return out, nil
}

// CreateAgentAccessLeasePrincipalChallenge is the resolver for the createAgentAccessLeasePrincipalChallenge field.
func (r *mutationResolver) CreateAgentAccessLeasePrincipalChallenge(ctx context.Context, username string, input model.AgentAccessLeaseChallengeInput) (*model.AgentAccessLeaseChallenge, error) {
	return r.createGraphAgentAccessLeaseChallenge(ctx, username, input, graphAgentAccessLeaseActionPrincipal)
}

// CreateAgentAccessLeaseAgentChallenge is the resolver for the createAgentAccessLeaseAgentChallenge field.
func (r *mutationResolver) CreateAgentAccessLeaseAgentChallenge(ctx context.Context, username string, input model.AgentAccessLeaseChallengeInput) (*model.AgentAccessLeaseChallenge, error) {
	return r.createGraphAgentAccessLeaseChallenge(ctx, username, input, graphAgentAccessLeaseActionAgent)
}

// CreateAgentAccessLease is the resolver for the createAgentAccessLease field.
func (r *mutationResolver) CreateAgentAccessLease(ctx context.Context, username string, input model.CreateAgentAccessLeaseInput) (*model.AgentAccessLease, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	claims, _, err := r.requireOwnedAgentLeaseAccount(ctx, username)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.GetDB() == nil {
		return nil, ErrStorageUnavailable
	}

	principalChallenge, err := r.loadGraphAgentAccessLeaseChallenge(ctx, input.PrincipalChallengeID)
	if err != nil {
		return nil, err
	}
	agentChallenge, err := r.loadGraphAgentAccessLeaseChallenge(ctx, input.AgentChallengeID)
	if err != nil {
		return nil, err
	}
	if !graphAgentAccessLeaseChallengesMatch(principalChallenge, agentChallenge) {
		return nil, apperrors.Unauthorized("challenge pair does not match")
	}
	if principalChallenge.Action != graphAgentAccessLeaseActionPrincipal || agentChallenge.Action != graphAgentAccessLeaseActionAgent {
		return nil, apperrors.Unauthorized("challenge actions do not match enrollment flow")
	}
	if !strings.EqualFold(principalChallenge.PrincipalUsername, claims.Username) {
		return nil, apperrors.Unauthorized("challenge principal does not match authenticated owner")
	}
	if err := r.authorizeGraphAgentAccessLeaseWallets(ctx, claims.Username, username, principalChallenge.PrincipalWallet, principalChallenge.AgentWallet); err != nil {
		return nil, err
	}

	if err := verifyGraphAgentAccessLeaseChallengeSignature(principalChallenge, input.PrincipalSignature); err != nil {
		return nil, apperrors.Unauthorized(err.Error())
	}
	if err := verifyGraphAgentAccessLeaseChallengeSignature(agentChallenge, input.AgentSignature); err != nil {
		return nil, apperrors.Unauthorized(err.Error())
	}

	now := time.Now().UTC()
	idleExpiresAt, absoluteExpiresAt := computeGraphAgentLeaseExpiries(now, principalChallenge.IdleTimeoutHours, principalChallenge.AbsoluteTTLHours)
	lease := &storageModels.AgentAccessLease{
		ID:                principalChallenge.LeaseID,
		Username:          username,
		PrincipalUsername: claims.Username,
		PrincipalWallet:   principalChallenge.PrincipalWallet,
		AgentWallet:       principalChallenge.AgentWallet,
		Scopes:            append([]string(nil), principalChallenge.Scopes...),
		DeviceLabel:       principalChallenge.DeviceLabel,
		Status:            graphAgentAccessLeaseStatusActive,
		IdleTimeoutHours:  principalChallenge.IdleTimeoutHours,
		TokenTTLHours:     principalChallenge.EffectiveTokenTTLHours(),
		IdleExpiresAt:     idleExpiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		LastUsedAt:        now,
		LeaseVersion:      1,
		SessionPublicKey:  principalChallenge.SessionPublicKey,
		SessionKeyType:    principalChallenge.SessionKeyType,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := r.markGraphAgentAccessLeaseChallengeUsed(ctx, principalChallenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return nil, apperrors.Unauthorized("principal challenge already used or expired")
		}
		return nil, apperrors.InternalWithCause(err, "failed to mark principal challenge used")
	}
	if err := r.markGraphAgentAccessLeaseChallengeUsed(ctx, agentChallenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return nil, apperrors.Unauthorized("agent challenge already used or expired")
		}
		return nil, apperrors.InternalWithCause(err, "failed to mark agent challenge used")
	}
	repo := r.agentAccessLeaseRepo()
	if repo == nil {
		return nil, ErrStorageUnavailable
	}
	if err := repo.CreateLease(ctx, lease); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return nil, apperrors.NewAppError(apperrors.CodeConflict, apperrors.CategoryBusiness, "lease already exists")
		}
		return nil, apperrors.InternalWithCause(err, "failed to create agent access lease")
	}
	return graphAgentAccessLeaseModel(lease, now), nil
}

// RevokeAgentAccessLease is the resolver for the revokeAgentAccessLease field.
func (r *mutationResolver) RevokeAgentAccessLease(ctx context.Context, username string, leaseID string, input *model.RevokeAgentAccessLeaseInput) (*model.AgentAccessLease, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	leaseID = strings.TrimSpace(leaseID)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return nil, err
	}
	claims, _, err := r.requireManagedAgentLeaseAccount(ctx, username)
	if err != nil {
		return nil, err
	}
	lease, err := r.loadGraphAgentAccessLease(ctx, username, leaseID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if graphEffectiveAgentAccessLeaseStatus(lease, now) == graphAgentAccessLeaseStatusRevoked {
		return graphAgentAccessLeaseModel(lease, now), nil
	}
	reason := ""
	if input != nil && input.Reason != nil {
		reason = strings.TrimSpace(*input.Reason)
	}
	repo := r.agentAccessLeaseRepo()
	if err := repo.RevokeLease(ctx, lease, claims.Username, reason, now); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to revoke agent access lease")
	}
	lease.Status = graphAgentAccessLeaseStatusRevoked
	lease.UpdatedAt = now
	lease.RevokedAt = now
	lease.RevokedBy = claims.Username
	lease.RevokedReason = reason
	return graphAgentAccessLeaseModel(lease, now), nil
}

// CreateAgentAccessLeaseSessionKeyChallenge is the resolver for the createAgentAccessLeaseSessionKeyChallenge field.
func (r *mutationResolver) CreateAgentAccessLeaseSessionKeyChallenge(ctx context.Context, username string, leaseID string, input model.AgentAccessLeaseSessionKeyChallengeInput) (*model.AgentAccessLeaseChallenge, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	leaseID = strings.TrimSpace(leaseID)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return nil, err
	}
	lease, err := r.loadGraphAgentAccessLease(ctx, username, leaseID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureActiveAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if graphEffectiveAgentAccessLeaseStatus(lease, now) != graphAgentAccessLeaseStatusActive {
		return nil, apperrors.Unauthorized("lease is not active")
	}
	sessionPublicKey, err := normalizeGraphAgentAccessSessionPublicKey(input.SessionPublicKey)
	if err != nil {
		return nil, err
	}
	challenge, err := r.createGraphAgentAccessLeaseChallengeRecord(ctx, graphAgentAccessLeaseOptions{
		LeaseID:           lease.ID,
		Username:          lease.Username,
		PrincipalUsername: lease.PrincipalUsername,
		PrincipalWallet:   lease.PrincipalWallet,
		AgentWallet:       lease.AgentWallet,
		SessionPublicKey:  sessionPublicKey,
		SessionKeyType:    graphAgentAccessLeaseSessionKeyType,
		Scopes:            append([]string(nil), lease.Scopes...),
		DeviceLabel:       lease.DeviceLabel,
		IdleTimeoutHours:  lease.IdleTimeoutHours,
		AbsoluteTTLHours:  graphLeaseRemainingAbsoluteTTLHours(now, lease.AbsoluteExpiresAt),
	}, graphAgentAccessLeaseActionSessionKeyAuth)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to create session key challenge")
	}
	return graphAgentAccessLeaseChallengeModel(challenge), nil
}

// AuthorizeAgentAccessLeaseSessionKey is the resolver for the authorizeAgentAccessLeaseSessionKey field.
func (r *mutationResolver) AuthorizeAgentAccessLeaseSessionKey(ctx context.Context, username string, leaseID string, input model.AuthorizeAgentAccessLeaseSessionKeyInput) (*model.AgentAccessLease, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	leaseID = strings.TrimSpace(leaseID)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return nil, err
	}
	lease, err := r.loadGraphAgentAccessLease(ctx, username, leaseID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureActiveAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if graphEffectiveAgentAccessLeaseStatus(lease, now) != graphAgentAccessLeaseStatusActive {
		return nil, apperrors.Unauthorized("lease is not active")
	}
	challenge, err := r.loadGraphAgentAccessLeaseChallenge(ctx, input.ChallengeID)
	if err != nil {
		return nil, err
	}
	if challenge.Action != graphAgentAccessLeaseActionSessionKeyAuth || !strings.EqualFold(challenge.LeaseID, leaseID) || !strings.EqualFold(challenge.Username, username) {
		return nil, apperrors.Unauthorized("challenge does not match lease")
	}
	if err := verifyGraphAgentAccessLeaseChallengeSignature(challenge, input.Signature); err != nil {
		return nil, apperrors.Unauthorized(err.Error())
	}
	if err := r.markGraphAgentAccessLeaseChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return nil, apperrors.Unauthorized("challenge already used or expired")
		}
		return nil, apperrors.InternalWithCause(err, "failed to mark session key challenge used")
	}
	repo := r.agentAccessLeaseRepo()
	if err := repo.AuthorizeSessionKey(ctx, lease, challenge.SessionPublicKey, graphAgentAccessLeaseSessionKeyType, now); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to authorize session key")
	}
	lease.SessionPublicKey = challenge.SessionPublicKey
	lease.SessionKeyType = graphAgentAccessLeaseSessionKeyType
	lease.SessionKeyCreatedAt = now
	lease.UpdatedAt = now
	return graphAgentAccessLeaseModel(lease, now), nil
}

// CreateAgentAccessLeaseRenewChallenge is the resolver for the createAgentAccessLeaseRenewChallenge field.
func (r *mutationResolver) CreateAgentAccessLeaseRenewChallenge(ctx context.Context, username string, leaseID string) (*model.AgentAccessLeaseChallenge, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	leaseID = strings.TrimSpace(leaseID)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return nil, err
	}
	lease, err := r.loadGraphAgentAccessLease(ctx, username, leaseID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureActiveAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if graphEffectiveAgentAccessLeaseStatus(lease, now) != graphAgentAccessLeaseStatusActive {
		return nil, apperrors.Unauthorized("lease is not active")
	}
	action := graphAgentAccessLeaseActionRenewWallet
	sessionPublicKey := ""
	sessionKeyType := ""
	if strings.TrimSpace(lease.SessionPublicKey) != "" && strings.EqualFold(strings.TrimSpace(lease.SessionKeyType), graphAgentAccessLeaseSessionKeyType) {
		action = graphAgentAccessLeaseActionRenewSession
		sessionPublicKey = lease.SessionPublicKey
		sessionKeyType = lease.SessionKeyType
	}
	challenge, err := r.createGraphAgentAccessLeaseChallengeRecord(ctx, graphAgentAccessLeaseOptions{
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
		AbsoluteTTLHours:  graphLeaseRemainingAbsoluteTTLHours(now, lease.AbsoluteExpiresAt),
	}, action)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to create renewal challenge")
	}
	return graphAgentAccessLeaseChallengeModel(challenge), nil
}

// ExchangeAgentAccessLeaseToken is the resolver for the exchangeAgentAccessLeaseToken field.
func (r *mutationResolver) ExchangeAgentAccessLeaseToken(ctx context.Context, username string, leaseID string, input model.ExchangeAgentAccessLeaseTokenInput) (*model.AgentAccessLeaseTokenPayload, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	leaseID = strings.TrimSpace(leaseID)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("leaseID", leaseID); err != nil {
		return nil, err
	}
	lease, err := r.loadGraphAgentAccessLease(ctx, username, leaseID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureActiveAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if graphEffectiveAgentAccessLeaseStatus(lease, now) != graphAgentAccessLeaseStatusActive {
		return nil, apperrors.Unauthorized("lease is not active")
	}
	challenge, err := r.loadGraphAgentAccessLeaseChallenge(ctx, input.ChallengeID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(challenge.LeaseID, leaseID) || !strings.EqualFold(challenge.Username, username) {
		return nil, apperrors.Unauthorized("challenge does not match lease")
	}
	if challenge.Action != graphAgentAccessLeaseActionRenewWallet && challenge.Action != graphAgentAccessLeaseActionRenewSession {
		return nil, apperrors.Unauthorized("challenge action does not match renewal flow")
	}
	if err := verifyGraphAgentAccessLeaseChallengeSignature(challenge, input.Signature); err != nil {
		return nil, apperrors.Unauthorized(err.Error())
	}
	if err := r.markGraphAgentAccessLeaseChallengeUsed(ctx, challenge.ID); err != nil {
		if dynamormErrors.IsConditionFailed(err) {
			return nil, apperrors.Unauthorized("challenge already used or expired")
		}
		return nil, apperrors.InternalWithCause(err, "failed to mark renewal challenge used")
	}
	newIdleExpiresAt := now.Add(time.Duration(lease.IdleTimeoutHours) * time.Hour)
	if newIdleExpiresAt.After(lease.AbsoluteExpiresAt) {
		newIdleExpiresAt = lease.AbsoluteExpiresAt
	}
	tokenExpiresAt := storageModels.AgentAccessLeaseTokenExpiresAt(now, lease, newIdleExpiresAt)
	remaining := time.Until(tokenExpiresAt)
	if remaining <= 0 {
		return nil, apperrors.Unauthorized("lease expired")
	}
	accessTTL := remaining
	if r.Config == nil {
		return nil, apperrors.Internal("jwt config not configured")
	}
	jwtSecret, err := r.Config.ResolveJWTSecret()
	if err == nil && strings.TrimSpace(jwtSecret) == "" {
		err = errors.New("JWT secret is empty")
	}
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "jwt secret not configured")
	}
	oauthSvc := auth.NewOAuthService(jwtSecret, r.Config, r.Storage, nil)
	accessToken, _, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContext(ctx, lease.Username, graphAgentAccessLeaseClientID, "", lease.Scopes, accessTTL, "", "")
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to mint lease access token")
	}
	repo := r.agentAccessLeaseRepo()
	if err := repo.RecordLeaseUse(ctx, lease, newIdleExpiresAt, now, challenge.Action == graphAgentAccessLeaseActionRenewSession); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to update lease activity")
	}
	return &model.AgentAccessLeaseTokenPayload{
		LeaseID:     lease.ID,
		AccessToken: accessToken,
		TokenType:   "Bearer",
		Scope:       strings.Join(lease.Scopes, " "),
		CreatedAt:   model.Time(now),
		ExpiresIn:   int(accessTTL.Seconds()),
	}, nil
}

func (r *mutationResolver) createGraphAgentAccessLeaseChallenge(ctx context.Context, username string, input model.AgentAccessLeaseChallengeInput, action string) (*model.AgentAccessLeaseChallenge, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	claims, _, err := r.requireOwnedAgentLeaseAccount(ctx, username)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.GetDB() == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}
	requestedScopes, err := validateDelegationScopes(claims.Scopes, input.Scopes)
	if err != nil {
		return nil, err
	}
	governance, err := r.requireAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, graphAgentGovernanceLoadError(err)
	}
	if err := validateDelegationAgainstAgentEnvelope(governance, requestedScopes); err != nil {
		return nil, err
	}
	sessionPublicKey := ""
	if input.SessionPublicKey != nil {
		sessionPublicKey = *input.SessionPublicKey
	}
	deviceLabel := ""
	if input.DeviceLabel != nil {
		deviceLabel = *input.DeviceLabel
	}
	leaseID := ""
	if input.LeaseID != nil {
		leaseID = *input.LeaseID
	}
	idleTimeoutHours := 0
	if input.IdleTimeoutHours != nil {
		idleTimeoutHours = *input.IdleTimeoutHours
	}
	absoluteTTLHours := 0
	if input.AbsoluteTTLHours != nil {
		absoluteTTLHours = *input.AbsoluteTTLHours
	}
	tokenTTLHours := 0
	if input.TokenTTLHours != nil {
		tokenTTLHours = *input.TokenTTLHours
	}
	opts, err := normalizeGraphAgentAccessLeaseOptions(
		leaseID,
		username,
		claims.Username,
		input.PrincipalWallet,
		input.AgentWallet,
		sessionPublicKey,
		requestedScopes,
		deviceLabel,
		idleTimeoutHours,
		absoluteTTLHours,
		tokenTTLHours,
		action == graphAgentAccessLeaseActionAgent,
	)
	if err != nil {
		return nil, err
	}
	if err := r.authorizeGraphAgentAccessLeaseWallets(ctx, claims.Username, username, opts.PrincipalWallet, opts.AgentWallet); err != nil {
		return nil, err
	}
	challenge, err := r.createGraphAgentAccessLeaseChallengeRecord(ctx, opts, action)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to create lease challenge")
	}
	return graphAgentAccessLeaseChallengeModel(challenge), nil
}

func (r *Resolver) requireOwnedAgentLeaseAccount(ctx context.Context, username string) (*auth.Claims, *storage.Account, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		return nil, nil, apperrors.InsufficientScope("write:accounts")
	}
	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, nil, ErrStorageUnavailable
	}
	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return nil, nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}
	owner := strings.TrimPrefix(strings.TrimSpace(account.User.AgentOwner), "@")
	if !strings.EqualFold(owner, claims.Username) {
		return nil, nil, apperrors.Forbidden("not authorized to manage agent lease enrollment")
	}
	return claims, account, nil
}

func (r *Resolver) requireManagedAgentLeaseAccount(ctx context.Context, username string) (*auth.Claims, *storage.Account, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		return nil, nil, apperrors.InsufficientScope("write:accounts")
	}
	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, nil, ErrStorageUnavailable
	}
	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return nil, nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}
	if !isAgentOwnerOrAdmin(claims, account.User, r.agentOwnerActorURL(claims.Username)) {
		return nil, nil, apperrors.Forbidden("not authorized to manage agent leases")
	}
	return claims, account, nil
}

func (r *Resolver) ensureActiveAgentLeaseAccount(ctx context.Context, username string) error {
	if r.Storage == nil || r.Storage.Account() == nil {
		return ErrStorageUnavailable
	}
	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}
	return nil
}

func (r *Resolver) graphUserHasWallet(ctx context.Context, username string, address string) (bool, error) {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil {
		return false, ErrStorageUnavailable
	}
	address = strings.TrimSpace(strings.ToLower(address))
	if address == "" {
		return false, nil
	}
	wallets, err := r.Storage.Account().GetUserWalletCredentials(ctx, username)
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

func (r *Resolver) validateGraphBoundAgentAccessLeaseWallets(ctx context.Context, agentUsername, principalWallet, agentWallet string) (bool, bool, bool, error) {
	service, err := r.getSoulService()
	if err != nil {
		return false, false, false, nil
	}

	soul, err := service.ResolveBoundAgent(ctx, agentUsername)
	if err != nil {
		return false, false, false, err
	}
	if soul == nil || !soul.Bound {
		return false, false, false, nil
	}

	expectedPrincipal := graphBoundSoulLeasePrincipalWallet(soul)
	expectedAgent := strings.TrimSpace(strings.ToLower(soul.Wallet))
	normalizedPrincipal := strings.TrimSpace(strings.ToLower(principalWallet))
	normalizedAgent := strings.TrimSpace(strings.ToLower(agentWallet))

	return true,
		expectedPrincipal != "" && strings.EqualFold(expectedPrincipal, normalizedPrincipal),
		expectedAgent != "" && strings.EqualFold(expectedAgent, normalizedAgent),
		nil
}

func (r *Resolver) authorizeGraphAgentAccessLeaseWallets(ctx context.Context, principalUsername, agentUsername, principalWallet, agentWallet string) error {
	if usedBoundSoul, principalOK, agentOK, walletErr := r.validateGraphBoundAgentAccessLeaseWallets(ctx, agentUsername, principalWallet, agentWallet); walletErr != nil {
		return apperrors.InternalWithCause(walletErr, "failed to verify bound soul wallets")
	} else if usedBoundSoul {
		if !principalOK {
			return apperrors.Forbidden("principal wallet does not match the bound soul principal")
		}
		if !agentOK {
			return apperrors.Forbidden("agent wallet does not match the bound soul body wallet")
		}
		return nil
	}

	if ok, walletErr := r.graphUserHasWallet(ctx, principalUsername, principalWallet); walletErr != nil {
		return apperrors.InternalWithCause(walletErr, "failed to verify principal wallet")
	} else if !ok {
		return apperrors.Forbidden("principal wallet is not linked to the owner account")
	}
	if ok, walletErr := r.graphUserHasWallet(ctx, agentUsername, agentWallet); walletErr != nil {
		return apperrors.InternalWithCause(walletErr, "failed to verify agent wallet")
	} else if !ok {
		return apperrors.Forbidden("agent wallet is not linked to the agent account")
	}

	return nil
}

func graphBoundSoulLeasePrincipalWallet(soul *soulservice.Soul) string {
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

func (r *Resolver) listGraphAgentAccessLeases(ctx context.Context, username string) ([]storageModels.AgentAccessLease, error) {
	repo := r.agentAccessLeaseRepo()
	if repo == nil {
		return nil, ErrStorageUnavailable
	}

	return repo.ListLeases(ctx, username)
}

func (r *Resolver) loadGraphAgentAccessLease(ctx context.Context, username, leaseID string) (*storageModels.AgentAccessLease, error) {
	repo := r.agentAccessLeaseRepo()
	if repo == nil {
		return nil, ErrStorageUnavailable
	}
	lease, err := repo.GetLease(ctx, username, leaseID)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent access lease not found")
		}
		return nil, apperrors.InternalWithCause(err, "failed to load agent access lease")
	}
	return lease, nil
}

func (r *Resolver) createGraphAgentAccessLeaseChallengeRecord(ctx context.Context, opts graphAgentAccessLeaseOptions, action string) (*storageModels.AgentAccessLeaseChallenge, error) {
	repo := r.agentAccessLeaseRepo()
	if repo == nil {
		return nil, ErrStorageUnavailable
	}
	now := time.Now().UTC()
	expiresAt := now.Add(graphAgentAccessLeaseChallengeTTL)
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	id := common.GenerateOperationIDULID()
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	address := opts.PrincipalWallet
	if action == graphAgentAccessLeaseActionAgent || action == graphAgentAccessLeaseActionRenewWallet || action == graphAgentAccessLeaseActionSessionKeyAuth {
		address = opts.AgentWallet
	}
	if action == graphAgentAccessLeaseActionRenewSession {
		address = ""
	}
	message := buildGraphAgentAccessLeaseChallengeMessage(id, opts, action, nonce, now, expiresAt)
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
	if err := repo.CreateChallenge(ctx, model); err != nil {
		return nil, err
	}
	return model, nil
}

func (r *Resolver) loadGraphAgentAccessLeaseChallenge(ctx context.Context, challengeID string) (*storageModels.AgentAccessLeaseChallenge, error) {
	repo := r.agentAccessLeaseRepo()
	if repo == nil {
		return nil, ErrStorageUnavailable
	}
	challenge, err := repo.GetChallenge(ctx, challengeID)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return nil, apperrors.Unauthorized("challenge not found")
		}
		return nil, apperrors.InternalWithCause(err, "failed to load access lease challenge")
	}
	now := time.Now().UTC()
	if challenge.ExpiresAt.IsZero() || challenge.TTL <= now.Unix() || now.After(challenge.ExpiresAt) {
		return nil, apperrors.Unauthorized("challenge expired")
	}
	if challenge.Used {
		return nil, apperrors.Unauthorized("challenge already used")
	}
	return challenge, nil
}

func (r *Resolver) markGraphAgentAccessLeaseChallengeUsed(ctx context.Context, challengeID string) error {
	repo := r.agentAccessLeaseRepo()
	if repo == nil {
		return ErrStorageUnavailable
	}
	return repo.MarkChallengeUsed(ctx, challengeID, time.Now().UTC())
}

func normalizeGraphAgentAccessLeaseOptions(
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
) (graphAgentAccessLeaseOptions, error) {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		if requireLeaseID {
			return graphAgentAccessLeaseOptions{}, common.ValidationError{Field: "leaseID", Message: "is required"}
		}
		leaseID = common.GenerateOperationIDULID()
	}
	principalWallet, err := normalizeGraphEthLeaseAddress(principalWallet)
	if err != nil {
		return graphAgentAccessLeaseOptions{}, common.ValidationError{Field: "principalWallet", Message: err.Error()}
	}
	agentWallet, err = normalizeGraphEthLeaseAddress(agentWallet)
	if err != nil {
		return graphAgentAccessLeaseOptions{}, common.ValidationError{Field: "agentWallet", Message: err.Error()}
	}
	sessionKeyType := ""
	if strings.TrimSpace(sessionPublicKey) != "" {
		sessionPublicKey, err = normalizeGraphAgentAccessSessionPublicKey(sessionPublicKey)
		if err != nil {
			return graphAgentAccessLeaseOptions{}, common.ValidationError{Field: "sessionPublicKey", Message: err.Error()}
		}
		sessionKeyType = graphAgentAccessLeaseSessionKeyType
	}
	deviceLabel = strings.TrimSpace(deviceLabel)
	if deviceLabel == "" {
		deviceLabel = "local-agent"
	}
	if idleTimeoutHours <= 0 {
		idleTimeoutHours = graphAgentAccessLeaseDefaultIdleHrs
	}
	if idleTimeoutHours > graphAgentAccessLeaseMaxIdleHrs {
		idleTimeoutHours = graphAgentAccessLeaseMaxIdleHrs
	}
	if absoluteTTLHours <= 0 {
		absoluteTTLHours = graphAgentAccessLeaseDefaultAbsHrs
	}
	if absoluteTTLHours > graphAgentAccessLeaseMaxAbsHrs {
		absoluteTTLHours = graphAgentAccessLeaseMaxAbsHrs
	}
	if absoluteTTLHours < idleTimeoutHours {
		absoluteTTLHours = idleTimeoutHours
	}
	tokenTTLHours = storageModels.NormalizeAgentAccessLeaseTokenTTLHours(idleTimeoutHours, absoluteTTLHours, tokenTTLHours)
	normalizedScopes := append([]string(nil), scopes...)
	sort.Strings(normalizedScopes)
	return graphAgentAccessLeaseOptions{
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

func normalizeGraphEthLeaseAddress(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", errors.New("must be provided")
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

func normalizeGraphAgentAccessSessionPublicKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("must be provided")
	}
	if _, err := auth.ParseAgentPublicKey(graphAgentAccessLeaseSessionKeyType, raw); err != nil {
		return "", errors.New("must be a valid ed25519 public key")
	}
	return raw, nil
}

func buildGraphAgentAccessLeaseChallengeMessage(id string, opts graphAgentAccessLeaseOptions, action string, nonce string, issuedAt time.Time, expiresAt time.Time) string {
	return fmt.Sprintf(
		"LESSER AGENT ACCESS LEASE\nid: %s\nlease_id: %s\naction: %s\ndomain: %s\nprincipal_username: %s\nagent_username: %s\nprincipal_wallet: %s\nagent_wallet: %s\nsession_public_key: %s\nscopes: %s\ndevice_label: %s\nidle_timeout_hours: %d\nabsolute_ttl_hours: %d\ntoken_ttl_hours: %d\nnonce: %s\nissued_at: %s\nexpires_at: %s",
		strings.TrimSpace(id),
		strings.TrimSpace(opts.LeaseID),
		strings.TrimSpace(action),
		strings.TrimSpace(graphAgentAccessLeaseDomain()),
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

func graphAgentAccessLeaseDomain() string {
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

func computeGraphAgentLeaseExpiries(now time.Time, idleTimeoutHours int, absoluteTTLHours int) (time.Time, time.Time) {
	absolute := now.Add(time.Duration(absoluteTTLHours) * time.Hour)
	idle := now.Add(time.Duration(idleTimeoutHours) * time.Hour)
	if idle.After(absolute) {
		idle = absolute
	}
	return idle, absolute
}

func graphLeaseRemainingAbsoluteTTLHours(now, absoluteExpiry time.Time) int {
	if absoluteExpiry.IsZero() {
		return graphAgentAccessLeaseDefaultAbsHrs
	}
	return max(1, int(absoluteExpiry.Sub(now).Hours()))
}

func graphEffectiveAgentAccessLeaseStatus(lease *storageModels.AgentAccessLease, now time.Time) string {
	if lease == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(lease.Status), graphAgentAccessLeaseStatusRevoked) {
		return graphAgentAccessLeaseStatusRevoked
	}
	if !lease.AbsoluteExpiresAt.IsZero() && now.After(lease.AbsoluteExpiresAt) {
		return graphAgentAccessLeaseStatusExpired
	}
	if !lease.IdleExpiresAt.IsZero() && now.After(lease.IdleExpiresAt) {
		return graphAgentAccessLeaseStatusExpired
	}
	return graphAgentAccessLeaseStatusActive
}

func graphAgentAccessLeaseChallengesMatch(a, b *storageModels.AgentAccessLeaseChallenge) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.LeaseID, b.LeaseID) &&
		strings.EqualFold(a.Username, b.Username) &&
		strings.EqualFold(a.PrincipalUsername, b.PrincipalUsername) &&
		strings.EqualFold(a.PrincipalWallet, b.PrincipalWallet) &&
		strings.EqualFold(a.AgentWallet, b.AgentWallet) &&
		strings.EqualFold(strings.TrimSpace(a.SessionPublicKey), strings.TrimSpace(b.SessionPublicKey)) &&
		strings.EqualFold(strings.Join(a.Scopes, " "), strings.Join(b.Scopes, " ")) &&
		a.DeviceLabel == b.DeviceLabel &&
		a.IdleTimeoutHours == b.IdleTimeoutHours &&
		a.AbsoluteTTLHours == b.AbsoluteTTLHours &&
		a.EffectiveTokenTTLHours() == b.EffectiveTokenTTLHours()
}

func buildGraphAgentAccessLeaseTypedData(challenge *storageModels.AgentAccessLeaseChallenge) apitypes.TypedData {
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
		Domain: apitypes.TypedDataDomain{
			Name:    "Lesser Agent Access Lease",
			Version: graphAgentAccessLeaseTypedDataVersion,
		},
		Message: apitypes.TypedDataMessage{
			"id":                strings.TrimSpace(challenge.ID),
			"leaseId":           strings.TrimSpace(challenge.LeaseID),
			"action":            strings.TrimSpace(challenge.Action),
			"instanceDomain":    graphAgentAccessLeaseDomain(),
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
		},
	}
}

func verifyGraphAgentAccessLeaseChallengeSignature(challenge *storageModels.AgentAccessLeaseChallenge, signature string) error {
	if challenge == nil {
		return errors.New("signature verification unavailable")
	}
	switch challenge.Action {
	case graphAgentAccessLeaseActionPrincipal, graphAgentAccessLeaseActionAgent, graphAgentAccessLeaseActionSessionKeyAuth, graphAgentAccessLeaseActionRenewWallet:
		return verifyGraphTypedDataSignature(challenge.Address, buildGraphAgentAccessLeaseTypedData(challenge), signature)
	case graphAgentAccessLeaseActionRenewSession:
		return verifyGraphSessionSignature(challenge.SessionPublicKey, challenge.Message, signature)
	default:
		return errors.New("unsupported challenge action")
	}
}

func verifyGraphTypedDataSignature(expectedAddress string, typedData apitypes.TypedData, signature string) error {
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

func verifyGraphSessionSignature(publicKey string, message string, signature string) error {
	pub, err := auth.ParseAgentPublicKey(graphAgentAccessLeaseSessionKeyType, publicKey)
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

func graphAgentAccessLeaseModel(lease *storageModels.AgentAccessLease, now time.Time) *model.AgentAccessLease {
	if lease == nil {
		return nil
	}
	var revokedAt *model.Time
	if !lease.RevokedAt.IsZero() {
		value := model.Time(lease.RevokedAt)
		revokedAt = &value
	}
	var sessionKeyCreatedAt *model.Time
	if !lease.SessionKeyCreatedAt.IsZero() {
		value := model.Time(lease.SessionKeyCreatedAt)
		sessionKeyCreatedAt = &value
	}
	var sessionKeyLastUsedAt *model.Time
	if !lease.SessionKeyLastUsedAt.IsZero() {
		value := model.Time(lease.SessionKeyLastUsedAt)
		sessionKeyLastUsedAt = &value
	}
	sessionPublicKey := optionalGraphString(lease.SessionPublicKey)
	sessionKeyType := optionalGraphString(lease.SessionKeyType)
	revokedBy := optionalGraphString(lease.RevokedBy)
	revokedReason := optionalGraphString(lease.RevokedReason)
	return &model.AgentAccessLease{
		ID:                   strings.TrimSpace(lease.ID),
		Username:             strings.TrimSpace(lease.Username),
		PrincipalUsername:    strings.TrimSpace(lease.PrincipalUsername),
		PrincipalWallet:      strings.TrimSpace(lease.PrincipalWallet),
		AgentWallet:          strings.TrimSpace(lease.AgentWallet),
		Scopes:               append([]string(nil), lease.Scopes...),
		DeviceLabel:          strings.TrimSpace(lease.DeviceLabel),
		Status:               graphEffectiveAgentAccessLeaseStatus(lease, now),
		IdleTimeoutHours:     lease.IdleTimeoutHours,
		TokenTTLHours:        lease.EffectiveTokenTTLHours(),
		IdleExpiresAt:        model.Time(lease.IdleExpiresAt),
		AbsoluteExpiresAt:    model.Time(lease.AbsoluteExpiresAt),
		LastUsedAt:           model.Time(lease.LastUsedAt),
		LeaseVersion:         lease.LeaseVersion,
		SessionPublicKey:     sessionPublicKey,
		SessionKeyType:       sessionKeyType,
		SessionKeyCreatedAt:  sessionKeyCreatedAt,
		SessionKeyLastUsedAt: sessionKeyLastUsedAt,
		CreatedAt:            model.Time(lease.CreatedAt),
		UpdatedAt:            model.Time(lease.UpdatedAt),
		RevokedAt:            revokedAt,
		RevokedBy:            revokedBy,
		RevokedReason:        revokedReason,
	}
}

func graphAgentAccessLeaseChallengeModel(challenge *storageModels.AgentAccessLeaseChallenge) *model.AgentAccessLeaseChallenge {
	if challenge == nil {
		return nil
	}
	sessionPublicKey := optionalGraphString(challenge.SessionPublicKey)
	sessionKeyType := optionalGraphString(challenge.SessionKeyType)
	var typedDataJSON *string
	switch challenge.Action {
	case graphAgentAccessLeaseActionPrincipal, graphAgentAccessLeaseActionAgent, graphAgentAccessLeaseActionSessionKeyAuth, graphAgentAccessLeaseActionRenewWallet:
		if raw, err := json.Marshal(buildGraphAgentAccessLeaseTypedData(challenge)); err == nil {
			value := string(raw)
			typedDataJSON = &value
		}
	}
	return &model.AgentAccessLeaseChallenge{
		ID:               strings.TrimSpace(challenge.ID),
		LeaseID:          strings.TrimSpace(challenge.LeaseID),
		Username:         strings.TrimSpace(challenge.Username),
		Action:           strings.TrimSpace(challenge.Action),
		WalletAddress:    strings.TrimSpace(challenge.Address),
		PrincipalWallet:  strings.TrimSpace(challenge.PrincipalWallet),
		AgentWallet:      strings.TrimSpace(challenge.AgentWallet),
		SessionPublicKey: sessionPublicKey,
		SessionKeyType:   sessionKeyType,
		Scopes:           append([]string(nil), challenge.Scopes...),
		DeviceLabel:      strings.TrimSpace(challenge.DeviceLabel),
		IdleTimeoutHours: challenge.IdleTimeoutHours,
		AbsoluteTTLHours: challenge.AbsoluteTTLHours,
		TokenTTLHours:    challenge.EffectiveTokenTTLHours(),
		Message:          challenge.Message,
		TypedDataJSON:    typedDataJSON,
		IssuedAt:         model.Time(challenge.IssuedAt),
		ExpiresAt:        model.Time(challenge.ExpiresAt),
	}
}

func optionalGraphString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
