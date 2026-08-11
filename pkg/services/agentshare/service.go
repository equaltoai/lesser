// Package agentshare manages owner-authorized per-agent share lists.
package agentshare

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

var (
	// ErrNotAuthorized means the caller is neither the agent owner nor an administrator.
	ErrNotAuthorized = errors.New("not authorized to manage agent share list")
	// ErrAgentNotFound means the requested local agent is unavailable.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrGranteeNotFound means no local account exists for the grantee username.
	ErrGranteeNotFound = errors.New("grantee account not found")
	// ErrInvalidGrantee means the grantee is not a local Lesser username.
	ErrInvalidGrantee = errors.New("grantee must be a local Lesser username")
	// ErrSelfGrant means an owner or acting administrator tried to share with themselves.
	ErrSelfGrant = errors.New("cannot grant agent access to yourself or the agent owner")
	// ErrAgentSelfGrant means an agent was named as its own grantee.
	ErrAgentSelfGrant = errors.New("cannot grant agent access to the agent itself")
	// ErrGrantNotFound means no persisted grant exists to revoke.
	ErrGrantNotFound = errors.New("agent share grant not found")
)

// Repository is the storage contract required by Service.
type Repository interface {
	CreateAgentShareGrant(context.Context, *models.AgentShareGrant) error
	RegrantAgentShareGrant(context.Context, *models.AgentShareGrant) error
	RevokeAgentShareGrant(context.Context, *models.AgentShareGrant) error
	GetAgentShareGrant(context.Context, string, string) (*models.AgentShareGrant, error)
	ListAgentShareGrantsByAgent(context.Context, string) ([]*models.AgentShareGrant, error)
	ListActiveAgentShareGrantsByGrantee(context.Context, string) ([]*models.AgentShareGrant, error)
	IsActiveAgentShareGrant(context.Context, string, string) (bool, error)
}

// AccountRepository resolves local agents and local grantees.
type AccountRepository interface {
	GetAccount(context.Context, string) (*storage.Account, error)
	GetUser(context.Context, string) (*storage.User, error)
}

// AuditRepository records the grant/revoke activity visible in agent activity logs.
type AuditRepository interface {
	StoreAuditEvent(context.Context, string, string, string, string, string, string, string, string, string, bool, string, map[string]interface{}) error
}

// ManageInput identifies an agent share-list mutation and its authenticated actor.
type ManageInput struct {
	AgentUsername   string
	GranteeUsername string
	ActorUsername   string
	ActorIsAdmin    bool
}

// Service manages per-agent share grants without minting credentials or changing sessions.
type Service struct {
	repo     Repository
	accounts AccountRepository
	audit    AuditRepository
	actorURL func(string) string
	logger   *zap.Logger
	now      func() time.Time
}

// NewService creates an agent share service.
func NewService(repo Repository, accounts AccountRepository, audit AuditRepository, actorURL func(string) string, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{repo: repo, accounts: accounts, audit: audit, actorURL: actorURL, logger: logger, now: func() time.Time { return time.Now().UTC() }}
}

// Grant creates or refreshes one owner-authorized share grant.
func (s *Service) Grant(ctx context.Context, input ManageInput) (*models.AgentShareGrant, error) {
	agent, actor, grantee, err := s.validateManageInput(ctx, input, true)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	grant := &models.AgentShareGrant{
		AgentUsername:   agent,
		GranteeUsername: grantee,
		GrantedBy:       actor,
		GrantedAt:       now,
	}
	existing, getErr := s.repo.GetAgentShareGrant(ctx, agent, grantee)
	if getErr == nil && existing != nil {
		grant.Version = existing.Version
		if err := s.repo.RegrantAgentShareGrant(ctx, grant); err != nil {
			return nil, err
		}
		s.auditMutation(ctx, "agent.share.grant", grant)
		return grant, nil
	}
	if getErr != nil && !errors.Is(getErr, storage.ErrNotFound) {
		return nil, getErr
	}
	if err := s.repo.CreateAgentShareGrant(ctx, grant); err != nil {
		return nil, err
	}
	s.auditMutation(ctx, "agent.share.grant", grant)
	return grant, nil
}

// Revoke immediately marks one grant inactive and removes it from discovery.
func (s *Service) Revoke(ctx context.Context, input ManageInput) (*models.AgentShareGrant, error) {
	agent, actor, grantee, err := s.validateManageInput(ctx, input, false)
	if err != nil {
		return nil, err
	}
	grant, err := s.repo.GetAgentShareGrant(ctx, agent, grantee)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrGrantNotFound
		}
		return nil, err
	}
	if grant.RevokedAt != nil {
		return grant, nil
	}
	now := s.now().UTC()
	grant.RevokedAt = &now
	grant.RevokedBy = actor
	if err := s.repo.RevokeAgentShareGrant(ctx, grant); err != nil {
		return nil, err
	}
	s.auditMutation(ctx, "agent.share.revoke", grant)
	return grant, nil
}

// ListByAgent returns an owner/admin view including revoked audit history.
func (s *Service) ListByAgent(ctx context.Context, agentUsername, actorUsername string, actorIsAdmin bool) ([]*models.AgentShareGrant, error) {
	agent, _, err := s.authorizeAgentOwner(ctx, agentUsername, actorUsername, actorIsAdmin)
	if err != nil {
		return nil, err
	}
	return s.repo.ListAgentShareGrantsByAgent(ctx, agent)
}

// ListSharedWith returns active agents shared with the authenticated local account.
func (s *Service) ListSharedWith(ctx context.Context, granteeUsername string) ([]*models.AgentShareGrant, error) {
	grantee := canonicalUsername(granteeUsername)
	if err := common.ValidateUsername(grantee); err != nil {
		return nil, ErrInvalidGrantee
	}
	return s.repo.ListActiveAgentShareGrantsByGrantee(ctx, grantee)
}

// IsActive performs the repository's uncached direct membership check.
func (s *Service) IsActive(ctx context.Context, agentUsername, granteeUsername string) (bool, error) {
	agent := canonicalUsername(agentUsername)
	grantee := canonicalUsername(granteeUsername)
	if common.ValidateUsername(agent) != nil || common.ValidateUsername(grantee) != nil {
		return false, nil
	}
	return s.repo.IsActiveAgentShareGrant(ctx, agent, grantee)
}

func (s *Service) validateManageInput(ctx context.Context, input ManageInput, validateGrantee bool) (string, string, string, error) {
	agent, ownerIdentifier, err := s.authorizeAgentOwner(ctx, input.AgentUsername, input.ActorUsername, input.ActorIsAdmin)
	if err != nil {
		return "", "", "", err
	}
	actor := canonicalUsername(input.ActorUsername)
	grantee := canonicalUsername(input.GranteeUsername)
	if err := common.ValidateUsername(grantee); err != nil {
		return "", "", "", ErrInvalidGrantee
	}
	if grantee == agent {
		return "", "", "", ErrAgentSelfGrant
	}
	if grantee == actor || auth.AgentOwnerMatchesLocalPrincipal(ownerIdentifier, grantee, s.actorURL) {
		return "", "", "", ErrSelfGrant
	}
	if validateGrantee {
		user, err := s.accounts.GetUser(ctx, grantee)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) || common.IsNotFound(err) {
				return "", "", "", ErrGranteeNotFound
			}
			return "", "", "", err
		}
		if user == nil || canonicalUsername(user.Username) != grantee {
			return "", "", "", ErrGranteeNotFound
		}
	}
	return agent, actor, grantee, nil
}

func (s *Service) authorizeAgentOwner(ctx context.Context, agentUsername, actorUsername string, actorIsAdmin bool) (string, string, error) {
	if s == nil || s.repo == nil || s.accounts == nil {
		return "", "", fmt.Errorf("agent share service is unavailable")
	}
	agent := canonicalUsername(agentUsername)
	actor := canonicalUsername(actorUsername)
	if common.ValidateUsername(agent) != nil || common.ValidateUsername(actor) != nil {
		return "", "", ErrNotAuthorized
	}
	account, err := s.accounts.GetAccount(ctx, agent)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || common.IsNotFound(err) {
			return "", "", ErrAgentNotFound
		}
		return "", "", err
	}
	if account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return "", "", ErrAgentNotFound
	}
	owner := strings.TrimSpace(account.User.AgentOwner)
	if !actorIsAdmin && !auth.AgentOwnerMatchesLocalPrincipal(account.User.AgentOwner, actor, s.actorURL) {
		return "", "", ErrNotAuthorized
	}
	return agent, owner, nil
}

func (s *Service) auditMutation(ctx context.Context, eventType string, grant *models.AgentShareGrant) {
	if s == nil || s.audit == nil || grant == nil {
		return
	}
	metadata := map[string]interface{}{
		"target_id":        grant.GranteeUsername,
		"agent_username":   grant.AgentUsername,
		"grantee_username": grant.GranteeUsername,
		"granted_by":       grant.GrantedBy,
		"granted_at":       grant.GrantedAt,
	}
	if grant.RevokedAt != nil {
		metadata["revoked_at"] = grant.RevokedAt
		metadata["revoked_by"] = grant.RevokedBy
	}
	if err := s.audit.StoreAuditEvent(
		ctx,
		eventType,
		"INFO",
		grant.AgentUsername,
		grant.AgentUsername,
		"",
		"",
		"",
		"",
		"",
		true,
		"",
		metadata,
	); err != nil {
		s.logger.Warn("failed to store agent share audit event", zap.String("event_type", eventType), zap.Error(err))
	}
}

func canonicalUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
