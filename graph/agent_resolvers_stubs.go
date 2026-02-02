package graph

import (
	"context"
	stdErrors "errors"

	"github.com/equaltoai/lesser/graph/model"
)

var errAgentSupportNotImplemented = stdErrors.New("agent support is not implemented")

// Agent is the resolver for the agent field.
func (r *queryResolver) Agent(ctx context.Context, username string) (*model.Agent, error) {
	_ = ctx
	_ = username
	return nil, errAgentSupportNotImplemented
}

// Agents is the resolver for the agents field.
func (r *queryResolver) Agents(ctx context.Context, first *int, after *model.Cursor, typeArg *model.AgentType) (*model.AgentConnection, error) {
	_ = ctx
	_ = first
	_ = after
	_ = typeArg
	return nil, errAgentSupportNotImplemented
}

// MyAgents is the resolver for the myAgents field.
func (r *queryResolver) MyAgents(ctx context.Context) ([]*model.Agent, error) {
	_ = ctx
	return nil, errAgentSupportNotImplemented
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

// RegisterAgent is the resolver for the registerAgent field.
func (r *mutationResolver) RegisterAgent(ctx context.Context, input model.RegisterAgentInput) (*model.RegisterAgentPayload, error) {
	_ = ctx
	_ = input
	return nil, errAgentSupportNotImplemented
}

// UpdateAgent is the resolver for the updateAgent field.
func (r *mutationResolver) UpdateAgent(ctx context.Context, username string, input model.UpdateAgentInput) (*model.Agent, error) {
	_ = ctx
	_ = username
	_ = input
	return nil, errAgentSupportNotImplemented
}

// DelegateToAgent is the resolver for the delegateToAgent field.
func (r *mutationResolver) DelegateToAgent(ctx context.Context, input model.DelegateToAgentInput) (*model.DelegationPayload, error) {
	_ = ctx
	_ = input
	return nil, errAgentSupportNotImplemented
}

// RevokeAgentToken is the resolver for the revokeAgentToken field.
func (r *mutationResolver) RevokeAgentToken(ctx context.Context, username string) (bool, error) {
	_ = ctx
	_ = username
	return false, errAgentSupportNotImplemented
}

// AgentActivity is the resolver for the agentActivity field.
func (r *subscriptionResolver) AgentActivity(ctx context.Context, username string) (<-chan *model.AgentActivityEvent, error) {
	_ = ctx
	_ = username
	return nil, errAgentSupportNotImplemented
}
