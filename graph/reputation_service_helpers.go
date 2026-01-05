package graph

import (
	"errors"

	"github.com/equaltoai/lesser/pkg/reputation"
)

func (r *Resolver) getReputationService() (*reputation.Service, error) {
	if r == nil {
		return nil, errors.New("resolver is nil")
	}
	if r.Config == nil {
		return nil, errors.New("config is not available")
	}

	storage := r.Storage
	if storage == nil && r.Registry != nil {
		storage = r.Registry.GetStorage()
	}
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	cfg := &reputation.Config{
		Storage:     storage,
		Logger:      r.Logger,
		InstanceURL: r.Config.BaseURL(),
		PrivateKey:  r.Config.ReputationPrivateKey,
	}

	return reputation.NewService(cfg)
}
