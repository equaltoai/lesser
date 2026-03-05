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

func (r *InstanceRepository) getCachedWellKnownLesserSoulAgent() (*models.InstanceWellKnownLesserSoulAgent, bool) {
	r.wellKnownLesserSoulAgentCache.mu.RLock()
	cfg := r.wellKnownLesserSoulAgentCache.cfg
	expiresAt := r.wellKnownLesserSoulAgentCache.expiresAt
	r.wellKnownLesserSoulAgentCache.mu.RUnlock()

	if cfg == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return cfg, true
}

func (r *InstanceRepository) setCachedWellKnownLesserSoulAgent(cfg *models.InstanceWellKnownLesserSoulAgent) {
	r.wellKnownLesserSoulAgentCache.mu.Lock()
	r.wellKnownLesserSoulAgentCache.cfg = cfg
	r.wellKnownLesserSoulAgentCache.expiresAt = time.Now().Add(wellKnownLesserSoulAgentCacheTTL)
	r.wellKnownLesserSoulAgentCache.mu.Unlock()
}

// GetWellKnownLesserSoulAgent returns the current HTTPS proof value served at
// GET /.well-known/lesser-soul-agent.
//
// If the record does not exist, it returns (nil, nil).
func (r *InstanceRepository) GetWellKnownLesserSoulAgent(ctx context.Context) (*models.InstanceWellKnownLesserSoulAgent, error) {
	if r == nil || r.wellKnownLesserSoulAgentRepo == nil {
		return nil, nil
	}

	if cached, ok := r.getCachedWellKnownLesserSoulAgent(); ok {
		return cached, nil
	}

	cfg := &models.InstanceWellKnownLesserSoulAgent{}
	err := r.wellKnownLesserSoulAgentRepo.Get(ctx, storage.InstanceConfigKey, models.SKWellKnownLesserSoulAgent, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return nil, nil
	}

	r.setCachedWellKnownLesserSoulAgent(cfg)
	return cfg, nil
}

// SetWellKnownLesserSoulAgent updates the current HTTPS proof value served at
// GET /.well-known/lesser-soul-agent.
func (r *InstanceRepository) SetWellKnownLesserSoulAgent(ctx context.Context, cfg *models.InstanceWellKnownLesserSoulAgent) error {
	if r == nil || r.wellKnownLesserSoulAgentRepo == nil {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("well-known lesser-soul-agent proof is nil")
	}

	cfg.ProofValue = strings.TrimSpace(cfg.ProofValue)
	cfg.UpdatedAt = time.Now().UTC()
	if err := cfg.UpdateKeys(); err != nil {
		return err
	}

	if err := r.wellKnownLesserSoulAgentRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.wellKnownLesserSoulAgentRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedWellKnownLesserSoulAgent(cfg)
	return nil
}
