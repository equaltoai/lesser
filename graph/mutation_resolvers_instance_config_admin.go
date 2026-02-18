package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

type (
	trustPatchSetter       func(context.Context, storagemodels.InstanceTrustConfigPatch) error
	translationPatchSetter func(context.Context, storagemodels.InstanceTranslationConfigPatch) error
	tipsPatchSetter        func(context.Context, storagemodels.InstanceTipsConfigPatch) error
	aiPatchSetter          func(context.Context, storagemodels.AIInstanceConfigPatch) error
)

func applyAdminInstanceConfigPatches(
	ctx context.Context,
	trust *model.AdminTrustConfigPatchInput,
	translation *model.AdminTranslationConfigPatchInput,
	tips *model.AdminTipsConfigPatchInput,
	ai *model.AdminAIConfigPatchInput,
	setTrust trustPatchSetter,
	setTranslation translationPatchSetter,
	setTips tipsPatchSetter,
	setAI aiPatchSetter,
) error {
	if trust != nil && setTrust != nil {
		if trust.BaseURL != nil {
			if err := validateAdminTrustBaseURL(*trust.BaseURL); err != nil {
				return apperrors.BadRequest(err.Error())
			}
		}
		if trust.AttestationsURL != nil {
			if err := validateAdminTrustBaseURL(*trust.AttestationsURL); err != nil {
				return apperrors.BadRequest(err.Error())
			}
		}

		patch := storagemodels.InstanceTrustConfigPatch{
			BaseURL:              trust.BaseURL,
			AttestationsURL:      trust.AttestationsURL,
			InstanceKeySecretARN: trust.InstanceKeySecretArn,
		}
		if err := setTrust(ctx, patch); err != nil {
			return err
		}
	}

	if translation != nil && setTranslation != nil {
		patch := storagemodels.InstanceTranslationConfigPatch{Enabled: translation.Enabled}
		if err := setTranslation(ctx, patch); err != nil {
			return err
		}
	}

	if tips != nil && setTips != nil {
		if err := validateAdminTipsPatch(tips.Enabled, tips.ChainID, tips.ContractAddress); err != nil {
			return err
		}

		patch := storagemodels.InstanceTipsConfigPatch{
			Enabled:         tips.Enabled,
			ChainID:         tips.ChainID,
			ContractAddress: tips.ContractAddress,
		}
		if err := setTips(ctx, patch); err != nil {
			return err
		}
	}

	if ai != nil && setAI != nil {
		patch := storagemodels.AIInstanceConfigPatch{
			AIEnabled:            ai.AiEnabled,
			ModerationEnabled:    ai.ModerationEnabled,
			NSFWDetectionEnabled: ai.NsfwDetectionEnabled,
			SpamDetectionEnabled: ai.SpamDetectionEnabled,
			PIIDetectionEnabled:  ai.PiiDetectionEnabled,
			AIContentDetection:   ai.AiContentDetection,
		}
		if err := setAI(ctx, patch); err != nil {
			return err
		}
	}

	return nil
}

func (r *mutationResolver) UpdateAdminInstanceManagedDefaults(ctx context.Context, input model.UpdateAdminInstanceManagedDefaultsInput) (*model.AdminInstanceConfig, error) {
	if _, err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	instanceRepo := r.Storage.Instance()

	if err := applyAdminInstanceConfigPatches(
		ctx,
		input.Trust,
		input.Translation,
		input.Tips,
		input.Ai,
		instanceRepo.SetTrustManagedDefaults,
		instanceRepo.SetTranslationManagedDefaults,
		instanceRepo.SetTipsManagedDefaults,
		instanceRepo.SetAIManagedDefaults,
	); err != nil {
		return nil, err
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

	if err := applyAdminInstanceConfigPatches(
		ctx,
		input.Trust,
		input.Translation,
		input.Tips,
		input.Ai,
		instanceRepo.SetTrustOverride,
		instanceRepo.SetTranslationOverride,
		instanceRepo.SetTipsOverride,
		instanceRepo.SetAIOverride,
	); err != nil {
		return nil, err
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
