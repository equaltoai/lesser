package graph

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

var errAgentSupportNotImplemented = apperrors.Internal("agent support is not implemented")

const delegatedAgentClientID = "lesser-agent-delegation"

const (
	oauthScopeAdmin         = "admin"
	oauthScopePush          = "push"
	oauthScopeWrite         = "write"
	oauthScopeFollow        = "follow"
	oauthScopeWriteStatuses = "write:statuses"
	oauthScopeWriteFollows  = "write:follows"

	agentQuarantineStatusApproved = "approved"
)

// Agent is the resolver for the agent field.
func (r *queryResolver) Agent(ctx context.Context, username string) (*model.Agent, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}

	user, err := r.Storage.User().GetUser(ctx, username)
	if err != nil || user == nil || !user.IsAgent || user.Suspended {
		return nil, nil
	}

	return r.convertStorageUserToAgent(ctx, user), nil
}

type agentListFilters struct {
	typeArg     *model.AgentType
	queryValue  string
	verified    *bool
	ownerFilter string
}

func agentListLimit(first *int, maxLimit int) int {
	limit := 20
	if first != nil && *first > 0 {
		limit = *first
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

func agentListLimit32(first *int, maxLimit int32) int32 {
	limit := int32(20)
	if first != nil && *first > 0 {
		value := *first
		if maxLimit > 0 && value > int(maxLimit) {
			value = int(maxLimit)
		}
		limit = int32(value) // #nosec G115 -- value is clamped above before converting
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

func agentListCursor(after *model.Cursor) string {
	if after == nil {
		return ""
	}
	return strings.TrimSpace(string(*after))
}

func agentUserMatchesListFilters(user *storage.User, filters agentListFilters) bool {
	if user == nil || !user.IsAgent || user.Suspended {
		return false
	}

	if filters.typeArg != nil && normalizeAgentType(user.AgentType) != *filters.typeArg {
		return false
	}

	if filters.verified != nil && agentMetadataBool(user, "agent_verified") != *filters.verified {
		return false
	}

	if filters.ownerFilter != "" {
		owner := strings.TrimPrefix(strings.TrimSpace(user.AgentOwner), "@")
		if !strings.EqualFold(owner, filters.ownerFilter) {
			return false
		}
	}

	if filters.queryValue != "" {
		displayName := strings.ToLower(strings.TrimSpace(user.DisplayName))
		bio := strings.ToLower(strings.TrimSpace(user.Note))
		usernameLower := strings.ToLower(strings.TrimSpace(user.Username))
		if !strings.Contains(usernameLower, filters.queryValue) && !strings.Contains(displayName, filters.queryValue) && !strings.Contains(bio, filters.queryValue) {
			return false
		}
	}

	return true
}

// Agents is the resolver for the agents field.
func (r *queryResolver) Agents(ctx context.Context, first *int, after *model.Cursor, typeArg *model.AgentType, query *string, verified *bool, ownerUsername *string) (*model.AgentConnection, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}

	limit32 := agentListLimit32(first, 100)
	limit := int(limit32)
	cursor := agentListCursor(after)

	users, nextCursor, err := r.Storage.User().ListAgents(ctx, limit32, cursor)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to list agents")
	}
	if len(users) > limit {
		users = users[:limit]
	}

	filters := agentListFilters{
		typeArg:     typeArg,
		queryValue:  strings.ToLower(strings.TrimSpace(derefString(query))),
		verified:    verified,
		ownerFilter: strings.TrimPrefix(strings.TrimSpace(derefString(ownerUsername)), "@"),
	}

	edges := make([]*model.AgentEdge, 0, len(users))
	for _, user := range users {
		if !agentUserMatchesListFilters(user, filters) {
			continue
		}

		agent := r.convertStorageUserToAgent(ctx, user)
		if agent == nil {
			continue
		}

		edges = append(edges, &model.AgentEdge{
			Node:   agent,
			Cursor: agentCursorForUser(user),
		})
	}

	var startCursor, endCursor *model.Cursor
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.AgentConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     nextCursor != "",
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

// MyAgents is the resolver for the myAgents field.
func (r *queryResolver) MyAgents(ctx context.Context) ([]*model.Agent, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeRead) {
		return nil, apperrors.InsufficientScope(auth.ScopeRead)
	}

	if r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}

	ownerUsername := strings.TrimSpace(claims.Username)
	if ownerUsername == "" {
		return nil, ErrAuthenticationRequired
	}

	ownerHandle := "@" + ownerUsername
	cursor := ""
	out := make([]*model.Agent, 0, 8)

	for page := 0; page < 25; page++ {
		users, nextCursor, err := r.Storage.User().ListAgents(ctx, 100, cursor)
		if err != nil {
			return nil, apperrors.InternalWithCause(err, "failed to list agents")
		}

		for _, user := range users {
			if user == nil || !user.IsAgent || user.Suspended {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(user.AgentOwner), ownerHandle) {
				continue
			}
			agent := r.convertStorageUserToAgent(ctx, user)
			if agent != nil {
				out = append(out, agent)
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return out, nil
}

// AgentMemorySearch is the resolver for the agentMemorySearch field.
func (r *queryResolver) AgentMemorySearch(ctx context.Context, query string, tags []string, dateRange *model.DateRangeInput, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
	_ = ctx
	_ = query
	_ = tags
	_ = dateRange
	_ = first
	_ = after
	return nil, errAgentSupportNotImplemented
}

// AgentActivity is the resolver for the agentActivity field.
func (r *queryResolver) AgentActivity(ctx context.Context, username string, first *int, after *model.Cursor) (*model.AgentActivityConnection, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeRead) {
		return nil, apperrors.InsufficientScope(auth.ScopeRead)
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	if !isAgentOwnerOrAdmin(claims, account.User) && !strings.EqualFold(strings.TrimSpace(claims.Username), username) {
		return nil, apperrors.Forbidden("not authorized to view agent activity")
	}

	if r.Storage.Audit() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := agentListLimit(first, 200)

	now := time.Now().UTC()
	start := now.Add(-30 * 24 * time.Hour)
	logs, err := r.Storage.Audit().GetUserAuditLogs(ctx, username, limit+1, start, now)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to query agent activity")
	}

	events := agentActivityEventsFromAuditLogs(username, logs)
	startIndex := agentActivityAfterCursorIndex(events, after)
	sliced, hasNext := agentActivitySlice(events, startIndex, limit)
	edges, startCursor, endCursor := agentActivityEdges(sliced)

	return &model.AgentActivityConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNext,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

func agentActivityEventsFromAuditLogs(username string, logs []*storageModels.AuthAuditLog) []*model.AgentActivityEvent {
	events := make([]*model.AgentActivityEvent, 0, len(logs))
	for _, log := range logs {
		event := agentActivityEventFromAuditLog(username, log)
		if event != nil {
			events = append(events, event)
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return time.Time(events[i].Timestamp).After(time.Time(events[j].Timestamp))
	})

	return events
}

func agentActivityEventFromAuditLog(username string, log *storageModels.AuthAuditLog) *model.AgentActivityEvent {
	if log == nil {
		return nil
	}

	action := strings.TrimSpace(log.EventType)
	if !strings.HasPrefix(action, "agent.") {
		return nil
	}

	metaRaw := strings.TrimSpace(log.Metadata)
	targetID := agentActivityTargetID(metaRaw)
	metaPtr := agentActivityMetadataPtr(metaRaw)

	return &model.AgentActivityEvent{
		EventID:       log.ID,
		AgentUsername: username,
		Action:        action,
		TargetID:      targetID,
		MetadataJSON:  metaPtr,
		Timestamp:     model.Time(log.Timestamp),
	}
}

func agentActivityTargetID(metaRaw string) *string {
	metaRaw = strings.TrimSpace(metaRaw)
	if metaRaw == "" {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(metaRaw), &parsed); err != nil {
		return nil
	}

	value, ok := parsed["target_id"].(string)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func agentActivityMetadataPtr(metaRaw string) *string {
	metaRaw = strings.TrimSpace(metaRaw)
	if metaRaw == "" {
		return nil
	}
	return &metaRaw
}

func agentActivityAfterCursorIndex(events []*model.AgentActivityEvent, after *model.Cursor) int {
	if after == nil {
		return 0
	}

	needle := strings.TrimSpace(string(*after))
	if needle == "" {
		return 0
	}

	for i, evt := range events {
		if evt != nil && strings.EqualFold(evt.EventID, needle) {
			return i + 1
		}
	}

	return 0
}

func agentActivitySlice(events []*model.AgentActivityEvent, startIndex, limit int) ([]*model.AgentActivityEvent, bool) {
	if startIndex >= len(events) {
		return nil, false
	}

	sliced := events[startIndex:]
	hasNext := len(sliced) > limit
	if hasNext {
		sliced = sliced[:limit]
	}

	return sliced, hasNext
}

func agentActivityEdges(events []*model.AgentActivityEvent) ([]*model.AgentActivityEdge, *model.Cursor, *model.Cursor) {
	edges := make([]*model.AgentActivityEdge, 0, len(events))
	for _, evt := range events {
		if evt == nil {
			continue
		}
		edges = append(edges, &model.AgentActivityEdge{
			Node:   evt,
			Cursor: model.Cursor(evt.EventID),
		})
	}

	var startCursor, endCursor *model.Cursor
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return edges, startCursor, endCursor
}

// AdminAgentPolicy is the resolver for the adminAgentPolicy field.
func (r *queryResolver) AdminAgentPolicy(ctx context.Context) (*model.AdminAgentPolicy, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeAdmin) {
		return nil, apperrors.InsufficientScope(auth.ScopeAdmin)
	}

	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	cfg, err := r.Storage.Instance().GetAgentInstanceConfig(ctx)
	if err != nil || cfg == nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent policy")
	}

	return adminAgentPolicyFromStorage(cfg), nil
}

// RegisterAgent is the resolver for the registerAgent field.
func (r *mutationResolver) RegisterAgent(ctx context.Context, input model.RegisterAgentInput) (*model.RegisterAgentPayload, error) {
	_ = ctx
	_ = input
	return nil, errAgentSupportNotImplemented
}

// UpdateAgent is the resolver for the updateAgent field.
func (r *mutationResolver) UpdateAgent(ctx context.Context, username string, input model.UpdateAgentInput) (*model.Agent, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		return nil, apperrors.InsufficientScope("write:accounts")
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	if !isAgentOwnerOrAdmin(claims, account.User) {
		return nil, apperrors.Forbidden("not authorized to modify agent")
	}

	if v := strings.TrimSpace(derefString(input.DisplayName)); v != "" {
		if err := common.ValidateDisplayName(v); err != nil {
			return nil, err
		}
		account.User.DisplayName = v
		if account.Actor != nil {
			account.Actor.Name = v
		}
	}

	if v := strings.TrimSpace(derefString(input.Bio)); v != "" {
		if err := common.ValidateAccountBio(v); err != nil {
			return nil, err
		}
		account.User.Note = v
		if account.Actor != nil {
			account.Actor.Summary = v
		}
	}

	if input.AgentType != nil {
		account.User.AgentType = strings.TrimSpace(input.AgentType.String())
	}

	if v := strings.TrimSpace(derefString(input.AgentVersion)); v != "" {
		account.User.AgentVersion = v
	} else if v := strings.TrimSpace(derefString(input.Version)); v != "" {
		account.User.AgentVersion = v
	}

	now := time.Now().UTC()
	if input.ExitQuarantine != nil && *input.ExitQuarantine {
		applyAgentQuarantineExit(account.User, claims, true, now)
	}

	if input.AgentCapabilities != nil {
		applyAgentCapabilitiesInput(ctx, r, account.User, input.AgentCapabilities)
	}

	r.ensureAgentActor(username, account)
	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}

	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to update agent")
	}

	return r.convertStorageUserToAgent(ctx, account.User), nil
}

// DeleteAgent is the resolver for the deleteAgent field.
func (r *mutationResolver) DeleteAgent(ctx context.Context, username string) (*model.Agent, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		return nil, apperrors.InsufficientScope("write:accounts")
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	if !isAgentOwnerOrAdmin(claims, account.User) {
		return nil, apperrors.Forbidden("not authorized to delete agent")
	}

	account.User.Suspended = true
	account.User.Discoverable = false
	now := time.Now().UTC()
	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}

	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to delete agent")
	}

	return r.convertStorageUserToAgent(ctx, account.User), nil
}

// DelegateToAgent is the resolver for the delegateToAgent field.
func (r *mutationResolver) DelegateToAgent(ctx context.Context, input model.DelegateToAgentInput) (*model.DelegationPayload, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}

	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		return nil, apperrors.InsufficientScope("write:accounts")
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	agentUsername := strings.TrimSpace(input.AgentUsername)
	if err := common.ValidateUsernameParamID(agentUsername); err != nil {
		return nil, err
	}

	requestedScopes, err := validateDelegationScopes(claims.Scopes, input.Scopes)
	if err != nil {
		return nil, err
	}

	accessTTL, err := validateAccessTokenTTL(input.ExpiresIn)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	account, err := r.resolveDelegatedAgentAccount(ctx, claims, agentUsername, requestedScopes)
	if err != nil {
		return nil, err
	}

	if r.Config.JWTSecret == "" {
		return nil, apperrors.Internal("jwt secret not configured")
	}
	oauthSvc := auth.NewOAuthService(r.Config.JWTSecret, r.Config, r.Storage, nil)
	accessToken, refreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTL(ctx, agentUsername, delegatedAgentClientID, "", requestedScopes, accessTTL)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to mint delegated agent tokens")
	}

	refreshExpiry := now.Add(auth.RefreshTokenDuration)
	_ = r.Storage.Account().CreateRefreshToken(ctx, &storage.RefreshToken{
		Token:     refreshToken,
		Username:  agentUsername,
		ClientID:  delegatedAgentClientID,
		Scopes:    requestedScopes,
		CreatedAt: now,
		ExpiresAt: refreshExpiry,
	})

	return &model.DelegationPayload{
		Agent:        r.convertStorageUserToAgent(ctx, account.User),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		Scope:        strings.Join(requestedScopes, " "),
		CreatedAt:    model.Time(now),
		ExpiresIn:    int(accessTTL.Seconds()),
	}, nil
}

func (r *mutationResolver) resolveDelegatedAgentAccount(ctx context.Context, claims *auth.Claims, agentUsername string, requestedScopes []string) (*storage.Account, error) {
	account, err := r.Storage.Account().GetAccount(ctx, agentUsername)
	if err != nil {
		if common.IsNotFound(err) {
			return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
		}
		return nil, apperrors.InternalWithCause(err, "failed to load agent account")
	}
	if account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}
	if !isAgentOwnerOrAdmin(claims, account.User) {
		return nil, apperrors.Forbidden("not authorized to delegate to agent")
	}
	if err := validateDelegationAgainstAgentEnvelope(account.User, requestedScopes); err != nil {
		return nil, err
	}

	return account, nil
}

// RevokeAgentToken is the resolver for the revokeAgentToken field.
func (r *mutationResolver) RevokeAgentToken(ctx context.Context, username string) (bool, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return false, err
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return false, err
	}

	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return false, err
	}
	if !claims.HasScope("write:accounts") && !claims.HasScope(auth.ScopeWrite) {
		return false, apperrors.InsufficientScope("write:accounts")
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return false, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return false, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	if !isAgentOwnerOrAdmin(claims, account.User) {
		return false, apperrors.Forbidden("not authorized to revoke agent tokens")
	}

	_, err = r.Storage.Account().DeleteRefreshTokensByUsernameAndClientID(ctx, username, delegatedAgentClientID)
	if err != nil {
		return false, apperrors.InternalWithCause(err, "failed to revoke agent tokens")
	}

	return true, nil
}

// UpdateAdminAgentPolicy is the resolver for the updateAdminAgentPolicy field.
func (r *mutationResolver) UpdateAdminAgentPolicy(ctx context.Context, input model.UpdateAdminAgentPolicyInput) (*model.AdminAgentPolicy, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeAdmin) {
		return nil, apperrors.InsufficientScope(auth.ScopeAdmin)
	}

	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	if input.DefaultQuarantineDays < 0 || input.DefaultQuarantineDays > 365 {
		return nil, common.ValidationError{Field: "defaultQuarantineDays", Message: "must be between 0 and 365"}
	}
	if input.MaxAgentsPerOwner < 0 || input.MaxAgentsPerOwner > 1000 {
		return nil, common.ValidationError{Field: "maxAgentsPerOwner", Message: "must be between 0 and 1000"}
	}
	if input.RemoteQuarantineDays < 0 || input.RemoteQuarantineDays > 365 {
		return nil, common.ValidationError{Field: "remoteQuarantineDays", Message: "must be between 0 and 365"}
	}
	if input.AgentMaxPostsPerHour < 0 || input.AgentMaxPostsPerHour > 10000 {
		return nil, common.ValidationError{Field: "agentMaxPostsPerHour", Message: "must be between 0 and 10000"}
	}
	if input.VerifiedAgentMaxPostsPerHour < 0 || input.VerifiedAgentMaxPostsPerHour > 10000 {
		return nil, common.ValidationError{Field: "verifiedAgentMaxPostsPerHour", Message: "must be between 0 and 10000"}
	}
	if input.AgentMaxFollowsPerHour < 0 || input.AgentMaxFollowsPerHour > 10000 {
		return nil, common.ValidationError{Field: "agentMaxFollowsPerHour", Message: "must be between 0 and 10000"}
	}
	if input.VerifiedAgentMaxFollowsPerHour < 0 || input.VerifiedAgentMaxFollowsPerHour > 10000 {
		return nil, common.ValidationError{Field: "verifiedAgentMaxFollowsPerHour", Message: "must be between 0 and 10000"}
	}
	if input.HybridRetrievalMaxCandidates < 0 || input.HybridRetrievalMaxCandidates > 5000 {
		return nil, common.ValidationError{Field: "hybridRetrievalMaxCandidates", Message: "must be between 0 and 5000"}
	}

	cfg := &storageModels.AgentInstanceConfig{
		AllowAgents:                    input.AllowAgents,
		AllowAgentRegistration:         input.AllowAgentRegistration,
		DefaultQuarantineDays:          input.DefaultQuarantineDays,
		MaxAgentsPerOwner:              input.MaxAgentsPerOwner,
		AllowRemoteAgents:              input.AllowRemoteAgents,
		RemoteQuarantineDays:           input.RemoteQuarantineDays,
		BlockedAgentDomains:            normalizeDomainList(input.BlockedAgentDomains),
		TrustedAgentDomains:            normalizeDomainList(input.TrustedAgentDomains),
		AgentMaxPostsPerHour:           input.AgentMaxPostsPerHour,
		VerifiedAgentMaxPostsPerHour:   input.VerifiedAgentMaxPostsPerHour,
		AgentMaxFollowsPerHour:         input.AgentMaxFollowsPerHour,
		VerifiedAgentMaxFollowsPerHour: input.VerifiedAgentMaxFollowsPerHour,
		HybridRetrievalEnabled:         input.HybridRetrievalEnabled,
		HybridRetrievalMaxCandidates:   input.HybridRetrievalMaxCandidates,
		UpdatedAt:                      time.Now().UTC(),
	}

	if err := cfg.UpdateKeys(); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to update policy keys")
	}
	if err := r.Storage.Instance().SetAgentInstanceConfig(ctx, cfg); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to update agent policy")
	}

	return adminAgentPolicyFromStorage(cfg), nil
}

// AdminVerifyAgent is the resolver for the adminVerifyAgent field.
func (r *mutationResolver) AdminVerifyAgent(ctx context.Context, username string, input *model.AdminVerifyAgentInput) (*model.Agent, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeAdmin) {
		return nil, apperrors.InsufficientScope(auth.ScopeAdmin)
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	now := time.Now().UTC()
	if account.User.Metadata == nil {
		account.User.Metadata = map[string]any{}
	}
	account.User.Metadata["agent_verified"] = true
	account.User.Metadata["agent_verified_at"] = now.Format(time.RFC3339)
	account.User.Metadata["agent_verified_by"] = claims.Username
	if input != nil && strings.TrimSpace(derefString(input.Reason)) != "" {
		account.User.Metadata["agent_verified_reason"] = strings.TrimSpace(derefString(input.Reason))
	}

	if input != nil && input.ExitQuarantine != nil && *input.ExitQuarantine {
		applyAgentQuarantineExit(account.User, claims, true, now)
	}

	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}
	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to verify agent")
	}

	return r.convertStorageUserToAgent(ctx, account.User), nil
}

// AdminUnverifyAgent is the resolver for the adminUnverifyAgent field.
func (r *mutationResolver) AdminUnverifyAgent(ctx context.Context, username string, input *model.AdminVerifyAgentInput) (*model.Agent, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeAdmin) {
		return nil, apperrors.InsufficientScope(auth.ScopeAdmin)
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	now := time.Now().UTC()
	if account.User.Metadata == nil {
		account.User.Metadata = map[string]any{}
	}
	account.User.Metadata["agent_verified"] = false
	account.User.Metadata["agent_unverified_at"] = now.Format(time.RFC3339)
	account.User.Metadata["agent_unverified_by"] = claims.Username
	if input != nil && strings.TrimSpace(derefString(input.Reason)) != "" {
		account.User.Metadata["agent_unverified_reason"] = strings.TrimSpace(derefString(input.Reason))
	}

	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}
	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to unverify agent")
	}

	return r.convertStorageUserToAgent(ctx, account.User), nil
}

// AdminSuspendAgent is the resolver for the adminSuspendAgent field.
func (r *mutationResolver) AdminSuspendAgent(ctx context.Context, username string) (*model.Agent, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeAdmin) {
		return nil, apperrors.InsufficientScope(auth.ScopeAdmin)
	}

	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	account, err := r.Storage.Account().GetAccount(ctx, username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent not found")
	}

	account.User.Suspended = true
	now := time.Now().UTC()
	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}
	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to suspend agent")
	}

	return r.convertStorageUserToAgent(ctx, account.User), nil
}

func (r *Resolver) requireAuthClaims(ctx context.Context) (*auth.Claims, error) {
	claims, ok := ctx.Value(common.ContextKeyClaims).(*auth.Claims)
	if !ok || claims == nil || strings.TrimSpace(claims.Username) == "" {
		return nil, ErrAuthenticationRequired
	}
	return claims, nil
}

func (r *Resolver) ensureAgentsEnabled(ctx context.Context) error {
	if r == nil || r.Config == nil || !r.Config.AllowAgents {
		return apperrors.Forbidden("agents are disabled")
	}

	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil
	}

	policy, err := r.Storage.Instance().GetAgentInstanceConfig(ctx)
	if err != nil {
		return apperrors.InternalWithCause(err, "failed to load agent policy")
	}
	if policy == nil || !policy.AllowAgents {
		return apperrors.Forbidden("agents are disabled by instance policy")
	}

	return nil
}

//nolint:unused // Retained for follow-up GraphQL registration flows.
func (r *Resolver) ensureAgentRegistrationEnabled(ctx context.Context) error {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return err
	}
	if r.Config == nil || !r.Config.AllowAgentRegistration {
		return apperrors.Forbidden("agent registration is disabled")
	}

	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil
	}

	policy, err := r.Storage.Instance().GetAgentInstanceConfig(ctx)
	if err != nil {
		return apperrors.InternalWithCause(err, "failed to load agent policy")
	}
	if policy == nil || !policy.AllowAgentRegistration {
		return apperrors.Forbidden("agent registration is disabled by instance policy")
	}

	return nil
}

func validateDelegationScopes(ownerScopes []string, requested []string) ([]string, error) {
	if err := common.ValidateSliceNotEmpty("scopes", requested); err != nil {
		return nil, err
	}

	clean := make([]string, 0, len(requested))
	for _, scope := range requested {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil, common.ValidationError{Field: "scopes", Message: "cannot contain empty values"}
		}

		base := strings.Split(scope, ":")[0]
		switch base {
		case oauthScopeAdmin, oauthScopePush:
			return nil, apperrors.Forbidden("delegation cannot grant admin or push scopes")
		}

		clean = append(clean, scope)
	}

	if err := common.ValidateApplicationScopes(strings.Join(clean, " ")); err != nil {
		return nil, err
	}
	if !scopesAreSubset(ownerScopes, clean) {
		return nil, apperrors.Forbidden("requested scopes exceed delegator scopes")
	}
	return clean, nil
}

func validateDelegationAgainstAgentEnvelope(user *storage.User, requestedScopes []string) error {
	allowedScopes, hasStoredEnvelope := agentDelegationEnvelope(user)
	if !hasStoredEnvelope {
		return nil
	}
	if !scopesAreSubset(allowedScopes, requestedScopes) {
		return apperrors.Forbidden("requested scopes exceed agent delegated scopes")
	}
	return nil
}

func agentDelegationEnvelope(user *storage.User) ([]string, bool) {
	if user == nil || user.Metadata == nil {
		return nil, false
	}
	if _, ok := user.Metadata["agent_delegated_scopes"]; !ok {
		return nil, false
	}
	return agentDelegatedScopes(user), true
}

func validateAccessTokenTTL(expiresIn *int) (time.Duration, error) {
	if expiresIn == nil || *expiresIn == 0 {
		return auth.AccessTokenDuration, nil
	}

	if *expiresIn < 60 {
		return 0, common.ValidationError{Field: "expiresIn", Message: "must be at least 60 seconds"}
	}

	maxSeconds := 7 * 24 * 60 * 60
	if *expiresIn > maxSeconds {
		return 0, common.ValidationError{Field: "expiresIn", Message: "cannot exceed 7 days"}
	}

	return time.Duration(*expiresIn) * time.Second, nil
}

//nolint:unused // Retained for follow-up GraphQL delegated-agent creation work.
func deriveAgentCapabilitiesFromScopes(scopes []string) agents.Capabilities {
	var caps agents.Capabilities

	for _, scope := range scopes {
		base := strings.Split(strings.TrimSpace(scope), ":")[0]
		switch base {
		case oauthScopeWrite:
			caps.CanPost = true
			caps.CanReply = true
			caps.CanBoost = true
			caps.CanDM = true
		case oauthScopeFollow:
			caps.CanFollow = true
		}

		if scope == oauthScopeWriteStatuses {
			caps.CanPost = true
			caps.CanReply = true
			caps.CanBoost = true
			caps.CanDM = true
		}
		if scope == oauthScopeWriteFollows {
			caps.CanFollow = true
		}
	}

	return caps
}

//nolint:unused // Retained for follow-up GraphQL delegated-agent creation work.
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

func isAgentOwnerOrAdmin(claims *auth.Claims, agentUser *storage.User) bool {
	if claims == nil || agentUser == nil {
		return false
	}

	if claims.HasScope(auth.ScopeAdmin) || claims.HasScope("admin:write") || claims.HasScope("admin:all") {
		return true
	}

	owner := strings.TrimSpace(agentUser.AgentOwner)
	if owner == "" {
		return false
	}
	owner = strings.TrimPrefix(owner, "@")
	return strings.EqualFold(owner, strings.TrimSpace(claims.Username))
}

//nolint:unused // Retained for follow-up GraphQL delegated-agent creation work.
func (r *Resolver) agentRegistrationLimits(ctx context.Context) (quarantineDays int, maxPostsPerHourAllowed int) {
	quarantineDays = 7
	maxPostsPerHourAllowed = 50

	if r != nil && r.Storage != nil && r.Storage.Instance() != nil {
		if policy, err := r.Storage.Instance().GetAgentInstanceConfig(ctx); err == nil && policy != nil {
			if policy.DefaultQuarantineDays > 0 {
				quarantineDays = policy.DefaultQuarantineDays
			}
			if policy.AgentMaxPostsPerHour > 0 {
				maxPostsPerHourAllowed = policy.AgentMaxPostsPerHour
			}
		}
	}

	if maxPostsPerHourAllowed <= 0 {
		maxPostsPerHourAllowed = 50
	}

	return quarantineDays, maxPostsPerHourAllowed
}

func applyAgentCapabilitiesInput(ctx context.Context, r *mutationResolver, user *storage.User, input *model.AgentCapabilitiesInput) {
	if user == nil || input == nil {
		return
	}

	caps := user.AgentCapabilities
	if caps == nil {
		caps = &agents.Capabilities{}
	}

	if input.CanPost != nil {
		caps.CanPost = *input.CanPost
	}
	if input.CanReply != nil {
		caps.CanReply = *input.CanReply
	}
	if input.CanBoost != nil {
		caps.CanBoost = *input.CanBoost
	}
	if input.CanFollow != nil {
		caps.CanFollow = *input.CanFollow
	}
	if input.CanDm != nil {
		caps.CanDM = *input.CanDm
	}
	if input.RequiresApproval != nil {
		caps.RequiresApproval = *input.RequiresApproval
	}
	if input.MaxPostsPerHour != nil {
		caps.MaxPostsPerHour = *input.MaxPostsPerHour
	}
	if input.RestrictedDomains != nil {
		caps.RestrictedDomains = append([]string(nil), input.RestrictedDomains...)
	}

	maxAllowed := r.maxPostsPerHourAllowedForUpdate(ctx, user)
	if caps.MaxPostsPerHour > maxAllowed {
		caps.MaxPostsPerHour = maxAllowed
	}

	user.AgentCapabilities = caps
}

func (r *mutationResolver) maxPostsPerHourAllowedForUpdate(ctx context.Context, user *storage.User) int {
	allowed := 50
	verifiedAllowed := 200

	if r != nil && r.Storage != nil && r.Storage.Instance() != nil {
		if policy, err := r.Storage.Instance().GetAgentInstanceConfig(ctx); err == nil && policy != nil {
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

func applyAgentQuarantineExit(user *storage.User, claims *auth.Claims, exitQuarantine bool, now time.Time) {
	if !exitQuarantine || user == nil {
		return
	}
	if user.Metadata == nil {
		user.Metadata = map[string]any{}
	}

	approvedBy := ""
	if claims != nil {
		approvedBy = strings.TrimSpace(claims.Username)
	}

	user.Metadata["agent_quarantine_status"] = agentQuarantineStatusApproved
	user.Metadata["agent_quarantine_end"] = now.Format(time.RFC3339)
	user.Metadata["agent_quarantine_approved_by"] = approvedBy
	user.Metadata["agent_quarantine_approved_at"] = now.Format(time.RFC3339)
}

func (r *Resolver) ensureAgentActor(username string, account *storage.Account) {
	if r == nil || account == nil || account.User == nil || r.Config == nil {
		return
	}
	if account.Actor == nil {
		actorID := r.Config.ActorURL(username)
		account.Actor = activitypub.NewActor(activitypub.ServiceType, actorID, username)
	}
	account.Actor.Type = activitypub.ServiceType
	account.User.IsAgent = true
	account.Actor = activitypubutil.BuildLocalActor(username, r.Config.BaseURL(), account.User, account.Actor)
}

func adminAgentPolicyFromStorage(cfg *storageModels.AgentInstanceConfig) *model.AdminAgentPolicy {
	if cfg == nil {
		return &model.AdminAgentPolicy{
			BlockedAgentDomains: []string{},
			TrustedAgentDomains: []string{},
			UpdatedAt:           model.Time(time.Now().UTC()),
		}
	}

	blocked := append([]string(nil), cfg.BlockedAgentDomains...)
	trusted := append([]string(nil), cfg.TrustedAgentDomains...)
	if blocked == nil {
		blocked = []string{}
	}
	if trusted == nil {
		trusted = []string{}
	}

	updatedAt := cfg.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	return &model.AdminAgentPolicy{
		AllowAgents:                    cfg.AllowAgents,
		AllowAgentRegistration:         cfg.AllowAgentRegistration,
		DefaultQuarantineDays:          cfg.DefaultQuarantineDays,
		MaxAgentsPerOwner:              cfg.MaxAgentsPerOwner,
		AllowRemoteAgents:              cfg.AllowRemoteAgents,
		RemoteQuarantineDays:           cfg.RemoteQuarantineDays,
		BlockedAgentDomains:            blocked,
		TrustedAgentDomains:            trusted,
		AgentMaxPostsPerHour:           cfg.AgentMaxPostsPerHour,
		VerifiedAgentMaxPostsPerHour:   cfg.VerifiedAgentMaxPostsPerHour,
		AgentMaxFollowsPerHour:         cfg.AgentMaxFollowsPerHour,
		VerifiedAgentMaxFollowsPerHour: cfg.VerifiedAgentMaxFollowsPerHour,
		HybridRetrievalEnabled:         cfg.HybridRetrievalEnabled,
		HybridRetrievalMaxCandidates:   cfg.HybridRetrievalMaxCandidates,
		UpdatedAt:                      model.Time(updatedAt),
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

func agentCursorForUser(user *storage.User) model.Cursor {
	if user == nil {
		return ""
	}
	username := strings.TrimSpace(user.Username)
	if username == "" {
		return ""
	}
	createdAt := user.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return model.Cursor(createdAt.UTC().Format(time.RFC3339) + "#" + username)
}

// AgentActivity is the resolver for the agentActivity field.
func (r *subscriptionResolver) AgentActivity(ctx context.Context, username string) (<-chan *model.AgentActivityEvent, error) {
	_ = ctx
	_ = username

	ch := make(chan *model.AgentActivityEvent)
	close(ch)
	return ch, errAgentSupportNotImplemented
}
