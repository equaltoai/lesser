package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// AgentGovernanceRepository persists typed governance state for agent accounts.
type AgentGovernanceRepository struct {
	base      *BaseRepository[*models.AgentGovernanceState]
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewAgentGovernanceRepository builds the typed agent-governance repository.
func NewAgentGovernanceRepository(db core.DB, tableName string, logger *zap.Logger) *AgentGovernanceRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentGovernanceRepository{
		base:      NewBaseRepository[*models.AgentGovernanceState](db, tableName, logger),
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// GetAgentGovernanceState loads typed governance state for one agent username.
func (r *AgentGovernanceRepository) GetAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error) {
	username = canonicalAgentGovernanceUsername(username)
	if username == "" {
		return nil, storage.ErrInvalidInput
	}

	record := &models.AgentGovernanceState{}
	if err := r.base.Get(ctx, fmt.Sprintf(models.KeyPatternUser, username), models.SKAgentGovernance, record); err != nil {
		if dynamormErrors.IsNotFound(err) || IsRepositoryNotFoundError(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	return modelToStorageAgentGovernanceState(record), nil
}

// GetAgentGovernanceStatesByUsernames batch-loads typed governance state by username.
func (r *AgentGovernanceRepository) GetAgentGovernanceStatesByUsernames(ctx context.Context, usernames []string) (map[string]*storage.AgentGovernanceState, error) {
	result := make(map[string]*storage.AgentGovernanceState, len(usernames))
	seen := make(map[string]struct{}, len(usernames))

	for _, username := range usernames {
		username = canonicalAgentGovernanceUsername(username)
		if username == "" {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}

		state, err := r.GetAgentGovernanceState(ctx, username)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, err
		}
		result[username] = state
	}

	return result, nil
}

// PutAgentGovernanceState creates or updates typed governance state for an agent.
func (r *AgentGovernanceRepository) PutAgentGovernanceState(ctx context.Context, state *storage.AgentGovernanceState) error {
	if state == nil {
		return storage.ErrInvalidInput
	}

	model := storageToModelAgentGovernanceState(state)
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	existing, err := r.GetAgentGovernanceState(ctx, model.Username)
	switch {
	case err == nil && existing != nil:
		model.CreatedAt = existing.CreatedAt
		if err := model.BeforeUpdate(); err != nil {
			return err
		}
		return r.base.Update(ctx, model)
	case err != nil && !errors.Is(err, storage.ErrNotFound):
		return err
	default:
		return r.base.CreateIfNotExists(ctx, model)
	}
}

// DeleteAgentGovernanceState removes typed governance state for an agent.
func (r *AgentGovernanceRepository) DeleteAgentGovernanceState(ctx context.Context, username string) error {
	username = canonicalAgentGovernanceUsername(username)
	if username == "" {
		return storage.ErrInvalidInput
	}
	return r.base.Delete(ctx, fmt.Sprintf(models.KeyPatternUser, username), models.SKAgentGovernance)
}

func modelToStorageAgentGovernanceState(model *models.AgentGovernanceState) *storage.AgentGovernanceState {
	if model == nil {
		return nil
	}

	return &storage.AgentGovernanceState{
		Username:             model.Username,
		QuarantineStatus:     model.QuarantineStatus,
		QuarantineStart:      cloneAgentGovernanceRepoTime(model.QuarantineStart),
		QuarantineEnd:        cloneAgentGovernanceRepoTime(model.QuarantineEnd),
		QuarantineApprovedBy: model.QuarantineApprovedBy,
		QuarantineApprovedAt: cloneAgentGovernanceRepoTime(model.QuarantineApprovedAt),
		DelegatedScopes:      append([]string(nil), model.DelegatedScopes...),
		SelfScopes:           append([]string(nil), model.SelfScopes...),
		SelfSovereign:        model.SelfSovereign,
		Verified:             model.Verified,
		VerifiedAt:           cloneAgentGovernanceRepoTime(model.VerifiedAt),
		VerifiedBy:           model.VerifiedBy,
		VerifiedReason:       model.VerifiedReason,
		UnverifiedAt:         cloneAgentGovernanceRepoTime(model.UnverifiedAt),
		UnverifiedBy:         model.UnverifiedBy,
		UnverifiedReason:     model.UnverifiedReason,
		KeyRotatedAt:         cloneAgentGovernanceRepoTime(model.KeyRotatedAt),
		CreatedAt:            model.CreatedAt,
		UpdatedAt:            model.UpdatedAt,
	}
}

func storageToModelAgentGovernanceState(state *storage.AgentGovernanceState) *models.AgentGovernanceState {
	if state == nil {
		return nil
	}

	return &models.AgentGovernanceState{
		Username:             canonicalAgentGovernanceUsername(state.Username),
		QuarantineStatus:     strings.TrimSpace(state.QuarantineStatus),
		QuarantineStart:      cloneAgentGovernanceRepoTime(state.QuarantineStart),
		QuarantineEnd:        cloneAgentGovernanceRepoTime(state.QuarantineEnd),
		QuarantineApprovedBy: strings.TrimSpace(state.QuarantineApprovedBy),
		QuarantineApprovedAt: cloneAgentGovernanceRepoTime(state.QuarantineApprovedAt),
		DelegatedScopes:      append([]string(nil), state.DelegatedScopes...),
		SelfScopes:           append([]string(nil), state.SelfScopes...),
		SelfSovereign:        state.SelfSovereign,
		Verified:             state.Verified,
		VerifiedAt:           cloneAgentGovernanceRepoTime(state.VerifiedAt),
		VerifiedBy:           strings.TrimSpace(state.VerifiedBy),
		VerifiedReason:       strings.TrimSpace(state.VerifiedReason),
		UnverifiedAt:         cloneAgentGovernanceRepoTime(state.UnverifiedAt),
		UnverifiedBy:         strings.TrimSpace(state.UnverifiedBy),
		UnverifiedReason:     strings.TrimSpace(state.UnverifiedReason),
		KeyRotatedAt:         cloneAgentGovernanceRepoTime(state.KeyRotatedAt),
		CreatedAt:            state.CreatedAt,
		UpdatedAt:            state.UpdatedAt,
	}
}

func cloneAgentGovernanceRepoTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func canonicalAgentGovernanceUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
