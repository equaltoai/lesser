package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
)

var (
	// ErrSoulBodyBindingAlreadyExists indicates the soul is already bound to another local body.
	ErrSoulBodyBindingAlreadyExists = errors.New("soul already bound")
	// ErrSoulBodyAlreadyHasBinding indicates the local body is already bound to another soul.
	ErrSoulBodyAlreadyHasBinding = errors.New("body already has soul")
)

// GetSoulBodyBinding returns the binding record for a soul agent, or (nil, nil) when no binding exists.
func (r *InstanceRepository) GetSoulBodyBinding(ctx context.Context, agentID string) (*models.InstanceSoulBodyBinding, error) {
	if r == nil || r.soulBodyBindingRepo == nil {
		return nil, nil
	}

	normalizedAgentID := strings.ToLower(strings.TrimSpace(agentID))
	if normalizedAgentID == "" {
		return nil, nil
	}

	binding := &models.InstanceSoulBodyBinding{}
	err := r.soulBodyBindingRepo.Get(ctx, storage.InstanceConfigKey, models.SoulBodyBindingSortKey(normalizedAgentID), binding)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if strings.TrimSpace(binding.PK) == "" || strings.TrimSpace(binding.SK) == "" {
		return nil, nil
	}
	return binding, nil
}

// GetSoulBodyBindingByUsername returns the binding record for a local username, or (nil, nil) when no binding exists.
func (r *InstanceRepository) GetSoulBodyBindingByUsername(ctx context.Context, username string) (*models.InstanceSoulBodyBinding, error) {
	if r == nil || r.soulBodyBindingUsernameRepo == nil {
		return nil, nil
	}

	normalizedUsername := strings.TrimSpace(username)
	if normalizedUsername == "" {
		return nil, nil
	}

	index := &models.InstanceSoulBodyBindingUsername{}
	err := r.soulBodyBindingUsernameRepo.Get(ctx, models.SoulBodyBindingUsernamePartitionKey(normalizedUsername), models.SKSoulBodyBindingUsername, index)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if strings.TrimSpace(index.AgentID) == "" {
		return nil, nil
	}

	return r.GetSoulBodyBinding(ctx, index.AgentID)
}

// BindSoulBody creates an explicit soul/body binding with one-to-one uniqueness enforcement.
func (r *InstanceRepository) BindSoulBody(ctx context.Context, agentID string, username string, principalAddress string) (*models.InstanceSoulBodyBinding, error) {
	if r == nil || r.soulBodyBindingRepo == nil || r.soulBodyBindingUsernameRepo == nil {
		return nil, nil
	}

	binding, usernameIndex, err := buildSoulBodyBindingModels(agentID, username, principalAddress)
	if err != nil {
		return nil, err
	}

	if existing, err := r.resolveExistingSoulBodyBinding(ctx, binding.AgentID, binding.Username); err != nil || existing != nil {
		return existing, err
	}

	if r.transactWriteFn == nil {
		return nil, fmt.Errorf("database does not support transact write operations")
	}

	if err := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx.Create(binding)
		tx.Create(usernameIndex)
		return nil
	}); err != nil {
		existing, resolveErr := r.resolveExistingSoulBodyBinding(ctx, binding.AgentID, binding.Username)
		if resolveErr != nil || existing != nil {
			return existing, resolveErr
		}
		return nil, err
	}

	return binding, nil
}

func buildSoulBodyBindingModels(agentID string, username string, principalAddress string) (*models.InstanceSoulBodyBinding, *models.InstanceSoulBodyBindingUsername, error) {
	binding := models.NewInstanceSoulBodyBinding(agentID, username, principalAddress)
	binding.BoundAt = time.Now().UTC()
	binding.UpdatedAt = binding.BoundAt
	if err := binding.UpdateKeys(); err != nil {
		return nil, nil, err
	}

	usernameIndex := models.NewInstanceSoulBodyBindingUsername(binding.Username, binding.AgentID)
	usernameIndex.UpdatedAt = binding.UpdatedAt
	if err := usernameIndex.UpdateKeys(); err != nil {
		return nil, nil, err
	}

	return binding, usernameIndex, nil
}

func (r *InstanceRepository) resolveExistingSoulBodyBinding(ctx context.Context, agentID string, username string) (*models.InstanceSoulBodyBinding, error) {
	existingByAgent, err := r.GetSoulBodyBinding(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if existingByAgent != nil {
		if existingByAgent.Username == username {
			return existingByAgent, nil
		}
		return nil, fmt.Errorf("%w: soul %s already bound to %s", ErrSoulBodyBindingAlreadyExists, agentID, existingByAgent.Username)
	}

	existingByUsername, err := r.GetSoulBodyBindingByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existingByUsername != nil {
		if existingByUsername.AgentID == agentID {
			return existingByUsername, nil
		}
		return nil, fmt.Errorf("%w: username %s already bound to %s", ErrSoulBodyAlreadyHasBinding, username, existingByUsername.AgentID)
	}

	return nil, nil
}
