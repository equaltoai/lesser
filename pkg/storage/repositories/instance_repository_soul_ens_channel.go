package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// GetSoulENSChannel returns the current ENS channel config for an agent.
//
// If the record does not exist, it returns (nil, nil).
func (r *InstanceRepository) GetSoulENSChannel(ctx context.Context, agentID string) (*models.InstanceSoulENSChannel, error) {
	if r == nil || r.soulENSChannelRepo == nil {
		return nil, nil
	}

	normalizedAgentID := strings.ToLower(strings.TrimSpace(agentID))
	if normalizedAgentID == "" {
		return nil, nil
	}

	cfg := &models.InstanceSoulENSChannel{}
	err := r.soulENSChannelRepo.Get(ctx, storage.InstanceConfigKey, models.SoulENSChannelSortKey(normalizedAgentID), cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return nil, nil
	}

	return cfg, nil
}

// SetSoulENSChannel creates or updates the ENS channel config for an agent.
func (r *InstanceRepository) SetSoulENSChannel(ctx context.Context, cfg *models.InstanceSoulENSChannel) error {
	if r == nil || r.soulENSChannelRepo == nil {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("soul ens channel config is nil")
	}

	cfg.AgentID = strings.ToLower(strings.TrimSpace(cfg.AgentID))
	cfg.Name = strings.ToLower(strings.TrimSpace(cfg.Name))
	cfg.ResolverAddress = strings.TrimSpace(cfg.ResolverAddress)
	cfg.Chain = strings.ToLower(strings.TrimSpace(cfg.Chain))
	cfg.UpdatedAt = time.Now().UTC()
	if err := cfg.UpdateKeys(); err != nil {
		return err
	}

	if err := r.soulENSChannelRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.soulENSChannelRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}
