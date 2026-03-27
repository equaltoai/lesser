package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

var errAgentSupportNotImplemented = apperrors.Internal("agent support is not implemented")

const delegatedAgentClientID = "lesser-agent-delegation"

const (
	oauthScopeAdmin  = "admin"
	oauthScopeFollow = "follow"
	oauthScopePush   = "push"
	oauthScopeWrite  = "write"

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

	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
	}

	return r.convertStorageUserToAgent(user, governance), nil
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

func agentUserMatchesListFilters(user *storage.User, governance *storage.AgentGovernanceState, filters agentListFilters) bool {
	if user == nil || !user.IsAgent || user.Suspended {
		return false
	}

	if filters.typeArg != nil && normalizeAgentType(user.AgentType) != *filters.typeArg {
		return false
	}

	if filters.verified != nil && graphAgentVerifiedState(governance) != *filters.verified {
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

func collectGraphAgentUsernames(users []*storage.User) []string {
	if len(users) == 0 {
		return nil
	}

	usernames := make([]string, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		username := strings.ToLower(strings.TrimSpace(user.Username))
		if username == "" {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		usernames = append(usernames, username)
	}

	return usernames
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
	governanceStates, err := r.loadAgentGovernanceStates(ctx, collectGraphAgentUsernames(users))
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance states")
	}

	filters := agentListFilters{
		typeArg:     typeArg,
		queryValue:  strings.ToLower(strings.TrimSpace(derefString(query))),
		verified:    verified,
		ownerFilter: strings.TrimPrefix(strings.TrimSpace(derefString(ownerUsername)), "@"),
	}

	edges := make([]*model.AgentEdge, 0, len(users))
	for _, user := range users {
		governance := governanceStates[strings.ToLower(strings.TrimSpace(user.Username))]
		if !agentUserMatchesListFilters(user, governance, filters) {
			continue
		}

		agent := r.convertStorageUserToAgent(user, governance)
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
		governanceStates, err := r.loadAgentGovernanceStates(ctx, collectGraphAgentUsernames(users))
		if err != nil {
			return nil, apperrors.InternalWithCause(err, "failed to load agent governance states")
		}

		for _, user := range users {
			if user == nil || !user.IsAgent || user.Suspended {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(user.AgentOwner), ownerHandle) {
				continue
			}
			agent := r.convertStorageUserToAgent(user, governanceStates[strings.ToLower(strings.TrimSpace(user.Username))])
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
	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
	}

	if !isAgentOwnerOrAdmin(claims, account.User) {
		return nil, apperrors.Forbidden("not authorized to modify agent")
	}

	now := time.Now().UTC()
	governance, err = r.applyGraphAgentUpdateInput(ctx, claims, account, governance, input, now)
	if err != nil {
		return nil, err
	}

	if err := r.persistGraphAgentUpdate(ctx, username, account, governance, now); err != nil {
		return nil, err
	}

	return r.convertStorageUserToAgent(account.User, governance), nil
}

func (r *mutationResolver) applyGraphAgentUpdateInput(
	ctx context.Context,
	claims *auth.Claims,
	account *storage.Account,
	governance *storage.AgentGovernanceState,
	input model.UpdateAgentInput,
	now time.Time,
) (*storage.AgentGovernanceState, error) {
	if account == nil || account.User == nil {
		return governance, apperrors.Internal("agent account is required")
	}

	if v := strings.TrimSpace(derefString(input.DisplayName)); v != "" {
		if err := common.ValidateDisplayName(v); err != nil {
			return governance, err
		}
		account.User.DisplayName = v
		if account.Actor != nil {
			account.Actor.Name = v
		}
	}

	if v := strings.TrimSpace(derefString(input.Bio)); v != "" {
		if err := common.ValidateAccountBio(v); err != nil {
			return governance, err
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

	if input.ExitQuarantine != nil && *input.ExitQuarantine {
		governance = applyAgentQuarantineExit(governance, claims, true, now)
	}

	if input.AgentCapabilities != nil {
		applyAgentCapabilitiesInput(ctx, r, account.User, governance, input.AgentCapabilities)
	}

	return governance, nil
}

func (r *mutationResolver) persistGraphAgentUpdate(
	ctx context.Context,
	username string,
	account *storage.Account,
	governance *storage.AgentGovernanceState,
	now time.Time,
) error {
	r.ensureAgentActor(username, account)
	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}

	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return apperrors.InternalWithCause(err, "failed to update agent")
	}
	if governance == nil {
		return nil
	}

	if governance.Username == "" {
		governance.Username = username
	}
	if governance.CreatedAt.IsZero() {
		governance.CreatedAt = now
	}
	governance.UpdatedAt = now
	if err := r.Storage.Account().PutAgentGovernanceState(ctx, governance); err != nil {
		return apperrors.InternalWithCause(err, "failed to update agent governance state")
	}

	return nil
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
	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
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

	return r.convertStorageUserToAgent(account.User, governance), nil
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

	accessTTL, err := validateAccessTokenTTL(r.Config, input.ExpiresIn)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	account, err := r.resolveOrCreateDelegatedAgentAccount(ctx, claims, input, requestedScopes, now)
	if err != nil {
		return nil, err
	}

	if r.Config.JWTSecret == "" {
		return nil, apperrors.Internal("jwt secret not configured")
	}
	bundle, err := auth.IssueAgentRuntimeTokens(ctx, r.Config, r.Storage, auth.AgentRuntimeTokenIssueParams{
		Username:    agentUsername,
		ClientID:    delegatedAgentClientID,
		Scopes:      requestedScopes,
		AccessTTL:   accessTTL,
		DeviceLabel: auth.DefaultAgentRuntimeDeviceLabel,
	})
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to mint delegated agent tokens")
	}
	governance, err := r.loadAgentGovernanceState(ctx, agentUsername)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
	}

	return &model.DelegationPayload{
		Agent:        r.convertStorageUserToAgent(account.User, governance),
		AccessToken:  bundle.AccessToken,
		RefreshToken: bundle.RefreshToken,
		TokenType:    "Bearer",
		Scope:        strings.Join(requestedScopes, " "),
		CreatedAt:    model.Time(bundle.Session.CreatedAt),
		ExpiresIn:    int(accessTTL.Seconds()),
	}, nil
}

func (r *mutationResolver) resolveOrCreateDelegatedAgentAccount(
	ctx context.Context,
	claims *auth.Claims,
	input model.DelegateToAgentInput,
	requestedScopes []string,
	now time.Time,
) (*storage.Account, error) {
	account, err := r.resolveDelegatedAgentAccount(ctx, claims, input.AgentUsername, requestedScopes)
	if err == nil {
		return account, nil
	}
	if !apperrors.HasCode(err, apperrors.CodeNotFound) {
		return nil, err
	}

	if regErr := r.ensureAgentRegistrationEnabled(ctx); regErr != nil {
		return nil, regErr
	}

	return r.createDelegatedAgentAccount(ctx, claims, input, requestedScopes, now)
}

func (r *mutationResolver) resolveDelegatedAgentAccount(ctx context.Context, claims *auth.Claims, agentUsername string, requestedScopes []string) (*storage.Account, error) {
	account, err := r.Storage.Account().GetAccount(ctx, agentUsername)
	if err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) {
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
	governance, err := r.loadAgentGovernanceState(ctx, agentUsername)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
	}
	if err := validateDelegationAgainstAgentEnvelope(governance, requestedScopes); err != nil {
		return nil, err
	}

	return account, nil
}

func (r *mutationResolver) createDelegatedAgentAccount(
	ctx context.Context,
	claims *auth.Claims,
	input model.DelegateToAgentInput,
	requestedScopes []string,
	now time.Time,
) (*storage.Account, error) {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil || r.Config == nil {
		return nil, ErrStorageUnavailable
	}

	agentUsername := strings.TrimSpace(input.AgentUsername)
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = agentUsername
	} else if err := common.ValidateDisplayName(displayName); err != nil {
		return nil, err
	}

	bio := strings.TrimSpace(derefString(input.Bio))
	if bio != "" {
		if err := common.ValidateAccountBio(bio); err != nil {
			return nil, err
		}
	}

	agentType := strings.TrimSpace(string(input.AgentType))
	if agentType == "" {
		agentType = string(model.AgentTypeCustom)
	}

	agentVersion := strings.TrimSpace(derefString(input.AgentVersion))
	if agentVersion == "" {
		agentVersion = strings.TrimSpace(input.Version)
	}
	if agentVersion == "" {
		agentVersion = agentVersionUnknown
	}

	quarantineDays, maxPostsPerHourAllowed := r.agentRegistrationLimits(ctx)
	capabilities := deriveAgentCapabilitiesFromScopes(requestedScopes)
	clampMaxPostsPerHour(&capabilities, maxPostsPerHourAllowed)
	quarantineEnd := now.AddDate(0, 0, quarantineDays)
	ownerIdentifier := "@" + strings.TrimSpace(claims.Username)

	user := &storage.User{
		Username:          agentUsername,
		DisplayName:       displayName,
		Note:              bio,
		Approved:          true,
		Suspended:         false,
		Silenced:          false,
		Role:              "user",
		Locked:            false,
		Discoverable:      true,
		CreatedAt:         now,
		UpdatedAt:         now,
		IsAgent:           true,
		AgentType:         agentType,
		AgentVersion:      agentVersion,
		AgentOwner:        ownerIdentifier,
		AgentCreatedBy:    strings.TrimSpace(claims.Username),
		AgentCapabilities: &capabilities,
	}

	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to generate agent keypair")
	}
	publicKeyPEM, err := federation.EncodePublicKeyPEM(&privateKey.PublicKey)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to encode agent public key")
	}
	privateKeyPEM, err := federation.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to encode agent private key")
	}

	actorID := r.Config.ActorURL(agentUsername)
	actor := activitypub.NewActor(activitypub.ServiceType, actorID, agentUsername)
	actor.Name = displayName
	actor.Summary = bio
	actor.URL = fmt.Sprintf("%s/@%s", r.Config.BaseURL(), agentUsername)
	actor.CreatedAt = &now
	actor.PublicKey = &activitypub.PublicKey{
		ID:           actorID + "#main-key",
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}
	actor = activitypubutil.BuildLocalActor(agentUsername, r.Config.BaseURL(), user, actor)

	account := &storage.Account{
		User:       user,
		Actor:      actor,
		PrivateKey: string(privateKeyPEM),
	}

	if err := r.Storage.Account().CreateAccount(ctx, account); err != nil {
		if common.IsConflict(err) {
			return r.resolveDelegatedAgentAccount(ctx, claims, agentUsername, requestedScopes)
		}
		return nil, apperrors.InternalWithCause(err, "failed to create agent account")
	}
	governance := &storage.AgentGovernanceState{
		Username:         agentUsername,
		QuarantineStatus: "quarantined",
		QuarantineStart:  cloneGraphAgentTime(&now),
		QuarantineEnd:    cloneGraphAgentTime(&quarantineEnd),
		DelegatedScopes:  append([]string(nil), requestedScopes...),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := r.Storage.Account().PutAgentGovernanceState(ctx, governance); err != nil {
		_ = r.Storage.Account().DeleteAccount(ctx, agentUsername)
		return nil, apperrors.InternalWithCause(err, "failed to create agent governance state")
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

	if err = auth.RevokeAllAgentRuntimeSessions(
		ctx,
		r.Storage,
		username,
		"manual_runtime_session_revocation",
		"",
		"",
	); err != nil {
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
	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
	}

	now := time.Now().UTC()
	if governance == nil {
		governance = &storage.AgentGovernanceState{
			Username:  username,
			CreatedAt: now,
		}
	}
	governance.Verified = true
	governance.VerifiedAt = cloneGraphAgentTime(&now)
	governance.VerifiedBy = claims.Username
	governance.UnverifiedAt = nil
	governance.UnverifiedBy = ""
	governance.UnverifiedReason = ""
	if input != nil && strings.TrimSpace(derefString(input.Reason)) != "" {
		governance.VerifiedReason = strings.TrimSpace(derefString(input.Reason))
	}

	if input != nil && input.ExitQuarantine != nil && *input.ExitQuarantine {
		governance = applyAgentQuarantineExit(governance, claims, true, now)
	}
	governance.UpdatedAt = now

	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}
	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to verify agent")
	}
	if err := r.Storage.Account().PutAgentGovernanceState(ctx, governance); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to verify agent governance state")
	}

	return r.convertStorageUserToAgent(account.User, governance), nil
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
	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
	}

	now := time.Now().UTC()
	if governance == nil {
		governance = &storage.AgentGovernanceState{
			Username:  username,
			CreatedAt: now,
		}
	}
	governance.Verified = false
	governance.UnverifiedAt = cloneGraphAgentTime(&now)
	governance.UnverifiedBy = claims.Username
	if input != nil && strings.TrimSpace(derefString(input.Reason)) != "" {
		governance.UnverifiedReason = strings.TrimSpace(derefString(input.Reason))
	}
	governance.UpdatedAt = now

	account.User.UpdatedAt = now
	if account.Actor != nil {
		account.Actor.Updated = &now
	}
	if err := r.Storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to unverify agent")
	}
	if err := r.Storage.Account().PutAgentGovernanceState(ctx, governance); err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to unverify agent governance state")
	}

	return r.convertStorageUserToAgent(account.User, governance), nil
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
	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load agent governance state")
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

	return r.convertStorageUserToAgent(account.User, governance), nil
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

func validateDelegationAgainstAgentEnvelope(governance *storage.AgentGovernanceState, requestedScopes []string) error {
	allowedScopes, hasStoredEnvelope := agentDelegationEnvelope(governance)
	if !hasStoredEnvelope {
		return nil
	}
	if !scopesAreSubset(allowedScopes, requestedScopes) {
		return apperrors.Forbidden("requested scopes exceed agent delegated scopes")
	}
	return nil
}

func agentDelegationEnvelope(governance *storage.AgentGovernanceState) ([]string, bool) {
	if governance == nil || len(governance.DelegatedScopes) == 0 {
		return nil, false
	}
	return governance.DelegatedScopesCopy(), true
}

func validateAccessTokenTTL(cfg *config.Config, expiresIn *int) (time.Duration, error) {
	if expiresIn == nil || *expiresIn == 0 {
		return auth.AgentAccessTokenTTL(cfg), nil
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
	return auth.ScopeSetAllows(ownerScopes, requested)
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

func applyAgentCapabilitiesInput(ctx context.Context, r *mutationResolver, user *storage.User, governance *storage.AgentGovernanceState, input *model.AgentCapabilitiesInput) {
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

	maxAllowed := r.maxPostsPerHourAllowedForUpdate(ctx, governance)
	if caps.MaxPostsPerHour > maxAllowed {
		caps.MaxPostsPerHour = maxAllowed
	}

	user.AgentCapabilities = caps
}

func (r *mutationResolver) maxPostsPerHourAllowedForUpdate(ctx context.Context, governance *storage.AgentGovernanceState) int {
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
	if graphAgentVerifiedState(governance) {
		maxAllowed = verifiedAllowed
	}
	return maxAllowed
}

func applyAgentQuarantineExit(governance *storage.AgentGovernanceState, claims *auth.Claims, exitQuarantine bool, now time.Time) *storage.AgentGovernanceState {
	if !exitQuarantine {
		return governance
	}
	if governance == nil {
		governance = &storage.AgentGovernanceState{}
	}

	approvedBy := ""
	if claims != nil {
		approvedBy = strings.TrimSpace(claims.Username)
	}

	governance.QuarantineStatus = agentQuarantineStatusApproved
	governance.QuarantineEnd = cloneGraphAgentTime(&now)
	governance.QuarantineApprovedBy = approvedBy
	governance.QuarantineApprovedAt = cloneGraphAgentTime(&now)
	return governance
}

func cloneGraphAgentTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
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
