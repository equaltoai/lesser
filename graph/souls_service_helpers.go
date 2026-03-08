package graph

import (
	"errors"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"go.uber.org/zap"
)

func (r *Resolver) getSoulService() (soulInventoryService, error) {
	if r == nil {
		return nil, errors.New("resolver is nil")
	}
	if r.soulsClient != nil {
		return r.soulsClient, nil
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

	logger := r.Logger
	if logger == nil && r.Registry != nil {
		logger = r.Registry.GetLogger()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return soulservice.NewService(storage.Account(), storage.Instance(), r.Config, logger), nil
}

func mapSoulServiceError(err error) error {
	switch {
	case errors.Is(err, soulservice.ErrTrustNotConfigured):
		return apperrors.NewAppError(
			apperrors.CodeUnprocessableEntity,
			apperrors.CategoryBusiness,
			"trust not configured",
		)
	case errors.Is(err, soulservice.ErrSoulNotAvailable):
		return apperrors.NotFound("soul")
	case errors.Is(err, soulservice.ErrSoulAlreadyBound):
		return apperrors.NewAppError(
			apperrors.CodeConflict,
			apperrors.CategoryBusiness,
			"soul already bound to another local body",
		)
	case errors.Is(err, soulservice.ErrBodyAlreadyHasSoul):
		return apperrors.NewAppError(
			apperrors.CodeConflict,
			apperrors.CategoryBusiness,
			"local body already has a soul",
		)
	default:
		return err
	}
}
