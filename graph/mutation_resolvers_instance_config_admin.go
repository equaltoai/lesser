package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

func (r *mutationResolver) UpdateAdminInstanceManagedDefaults(ctx context.Context, input model.UpdateAdminInstanceManagedDefaultsInput) (*model.AdminInstanceConfig, error) {
	if _, err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	instanceRepo := r.Storage.Instance()

	if input.Trust != nil {
		if input.Trust.BaseURL != nil {
			if err := validateAdminTrustBaseURL(*input.Trust.BaseURL); err != nil {
				return nil, apperrors.BadRequest(err.Error())
			}
		}
		if input.Trust.AttestationsURL != nil {
			if err := validateAdminTrustBaseURL(*input.Trust.AttestationsURL); err != nil {
				return nil, apperrors.BadRequest(err.Error())
			}
		}

		patch := storagemodels.InstanceTrustConfigPatch{
			BaseURL:              input.Trust.BaseURL,
			AttestationsURL:      input.Trust.AttestationsURL,
			InstanceKeySecretARN: input.Trust.InstanceKeySecretArn,
		}
		if err := instanceRepo.SetTrustManagedDefaults(ctx, patch); err != nil {
			return nil, err
		}
	}

	if input.Translation != nil {
		patch := storagemodels.InstanceTranslationConfigPatch{Enabled: input.Translation.Enabled}
		if err := instanceRepo.SetTranslationManagedDefaults(ctx, patch); err != nil {
			return nil, err
		}
	}

	if input.Tips != nil {
		if err := validateAdminTipsPatch(input.Tips.Enabled, input.Tips.ChainID, input.Tips.ContractAddress); err != nil {
			return nil, err
		}

		patch := storagemodels.InstanceTipsConfigPatch{
			Enabled:         input.Tips.Enabled,
			ChainID:         input.Tips.ChainID,
			ContractAddress: input.Tips.ContractAddress,
		}
		if err := instanceRepo.SetTipsManagedDefaults(ctx, patch); err != nil {
			return nil, err
		}
	}

	if input.Ai != nil {
		patch := storagemodels.AIInstanceConfigPatch{
			AIEnabled:            input.Ai.AiEnabled,
			ModerationEnabled:    input.Ai.ModerationEnabled,
			NSFWDetectionEnabled: input.Ai.NsfwDetectionEnabled,
			SpamDetectionEnabled: input.Ai.SpamDetectionEnabled,
			PIIDetectionEnabled:  input.Ai.PiiDetectionEnabled,
			AIContentDetection:   input.Ai.AiContentDetection,
		}
		if err := instanceRepo.SetAIManagedDefaults(ctx, patch); err != nil {
			return nil, err
		}
	}

	return r.resolveAdminInstanceConfig(ctx)
}

func (r *mutationResolver) UpdateAdminInstanceOverrides(ctx context.Context, input model.UpdateAdminInstanceOverridesInput) (*model.AdminInstanceConfig, error) {
	if _, err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	instanceRepo := r.Storage.Instance()

	if input.Trust != nil {
		if input.Trust.BaseURL != nil {
			if err := validateAdminTrustBaseURL(*input.Trust.BaseURL); err != nil {
				return nil, apperrors.BadRequest(err.Error())
			}
		}
		if input.Trust.AttestationsURL != nil {
			if err := validateAdminTrustBaseURL(*input.Trust.AttestationsURL); err != nil {
				return nil, apperrors.BadRequest(err.Error())
			}
		}

		patch := storagemodels.InstanceTrustConfigPatch{
			BaseURL:              input.Trust.BaseURL,
			AttestationsURL:      input.Trust.AttestationsURL,
			InstanceKeySecretARN: input.Trust.InstanceKeySecretArn,
		}
		if err := instanceRepo.SetTrustOverride(ctx, patch); err != nil {
			return nil, err
		}
	}

	if input.Translation != nil {
		patch := storagemodels.InstanceTranslationConfigPatch{Enabled: input.Translation.Enabled}
		if err := instanceRepo.SetTranslationOverride(ctx, patch); err != nil {
			return nil, err
		}
	}

	if input.Tips != nil {
		if err := validateAdminTipsPatch(input.Tips.Enabled, input.Tips.ChainID, input.Tips.ContractAddress); err != nil {
			return nil, err
		}

		patch := storagemodels.InstanceTipsConfigPatch{
			Enabled:         input.Tips.Enabled,
			ChainID:         input.Tips.ChainID,
			ContractAddress: input.Tips.ContractAddress,
		}
		if err := instanceRepo.SetTipsOverride(ctx, patch); err != nil {
			return nil, err
		}
	}

	if input.Ai != nil {
		patch := storagemodels.AIInstanceConfigPatch{
			AIEnabled:            input.Ai.AiEnabled,
			ModerationEnabled:    input.Ai.ModerationEnabled,
			NSFWDetectionEnabled: input.Ai.NsfwDetectionEnabled,
			SpamDetectionEnabled: input.Ai.SpamDetectionEnabled,
			PIIDetectionEnabled:  input.Ai.PiiDetectionEnabled,
			AIContentDetection:   input.Ai.AiContentDetection,
		}
		if err := instanceRepo.SetAIOverride(ctx, patch); err != nil {
			return nil, err
		}
	}

	return r.resolveAdminInstanceConfig(ctx)
}

func (r *mutationResolver) ClearAdminInstanceOverrides(ctx context.Context, features []model.InstanceConfigFeature) (*model.AdminInstanceConfig, error) {
	if _, err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	instanceRepo := r.Storage.Instance()

	for _, feature := range features {
		switch feature {
		case model.InstanceConfigFeatureTrust:
			if err := instanceRepo.ClearTrustOverride(ctx); err != nil {
				return nil, err
			}
		case model.InstanceConfigFeatureTranslation:
			if err := instanceRepo.ClearTranslationOverride(ctx); err != nil {
				return nil, err
			}
		case model.InstanceConfigFeatureTips:
			if err := instanceRepo.ClearTipsOverride(ctx); err != nil {
				return nil, err
			}
		case model.InstanceConfigFeatureAi:
			if err := instanceRepo.ClearAIOverride(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, apperrors.BadRequest("unknown feature: " + string(feature))
		}
	}

	return r.resolveAdminInstanceConfig(ctx)
}

func validateAdminTipsPatch(enabled *bool, chainID *int, contractAddress *string) error {
	if enabled == nil || !*enabled {
		return nil
	}

	if chainID == nil || *chainID <= 0 {
		return apperrors.ValidationFailed("chainId", "chainId is required when enabling tips")
	}

	if contractAddress == nil || strings.TrimSpace(*contractAddress) == "" {
		return apperrors.ValidationFailed("contractAddress", "contractAddress is required when enabling tips")
	}

	return nil
}
