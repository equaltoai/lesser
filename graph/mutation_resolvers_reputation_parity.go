package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// ExportReputation covers POST /api/v1/reputation/export.
func (r *mutationResolver) ExportReputation(ctx context.Context) (*model.PortableReputation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if r.Config == nil {
		return nil, errors.New("config is not available")
	}

	actorID := fmt.Sprintf("https://%s/users/%s", r.Config.Domain, username)

	service, err := r.getReputationService()
	if err != nil {
		return nil, err
	}

	portable, err := service.ExportReputation(ctx, actorID)
	if err != nil {
		r.Logger.Error("failed to export reputation", zap.String("actor_id", actorID), zap.Error(err))
		return nil, errors.Join(errors.New("failed to export reputation"), err)
	}

	vouches := make([]*model.Vouch, 0, len(portable.Vouches))
	for i := range portable.Vouches {
		vouch := portable.Vouches[i]
		converted := r.convertVouchToGraphQL(ctx, &vouch)
		if converted != nil {
			vouches = append(vouches, converted)
		}
	}

	return &model.PortableReputation{
		Context:     portable.Context,
		Type:        portable.Type,
		Actor:       portable.Actor,
		Reputation:  convertReputationToGraphQL(portable.Reputation),
		Vouches:     vouches,
		IssuedAt:    model.Time(portable.IssuedAt),
		ExpiresAt:   model.Time(portable.ExpiresAt),
		Issuer:      portable.Issuer,
		IssuerProof: portable.IssuerProof,
	}, nil
}

// ImportReputation covers POST /api/v1/reputation/import.
func (r *mutationResolver) ImportReputation(ctx context.Context, document string) (*model.ReputationImportResult, error) {
	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	document = strings.TrimSpace(document)
	if err := common.ValidateRequiredParam("document", document); err != nil {
		return nil, err
	}

	service, err := r.getReputationService()
	if err != nil {
		return nil, err
	}

	result, err := service.ImportReputation(ctx, document)
	if err != nil {
		r.Logger.Error("failed to import reputation", zap.Error(err))
		return nil, errors.Join(errors.New("failed to import reputation"), err)
	}

	var message *string
	if strings.TrimSpace(result.Message) != "" {
		message = &result.Message
	}

	var errMsg *string
	if strings.TrimSpace(result.Error) != "" {
		errMsg = &result.Error
	}

	return &model.ReputationImportResult{
		Success:         result.Success,
		ActorID:         result.ActorID,
		PreviousScore:   result.PreviousScore,
		ImportedScore:   result.ImportedScore,
		VouchesImported: result.VouchesImported,
		Message:         message,
		Error:           errMsg,
	}, nil
}

// VerifyReputation covers POST /api/v1/reputation/verify.
func (r *mutationResolver) VerifyReputation(ctx context.Context, document string) (*model.ReputationVerificationResult, error) {
	document = strings.TrimSpace(document)
	if err := common.ValidateRequiredParam("document", document); err != nil {
		return nil, err
	}

	service, err := r.getReputationService()
	if err != nil {
		return nil, err
	}

	result, err := service.VerifyReputation(ctx, document)
	if err != nil {
		r.Logger.Error("failed to verify reputation", zap.Error(err))
		return nil, errors.Join(errors.New("failed to verify reputation"), err)
	}

	var errMsg *string
	if strings.TrimSpace(result.Error) != "" {
		errMsg = &result.Error
	}

	return &model.ReputationVerificationResult{
		Valid:          result.Valid,
		ActorID:        result.ActorID,
		Issuer:         result.Issuer,
		IssuedAt:       model.Time(result.IssuedAt),
		ExpiresAt:      model.Time(result.ExpiresAt),
		SignatureValid: result.SignatureValid,
		NotExpired:     result.NotExpired,
		IssuerTrusted:  result.IssuerTrusted,
		Error:          errMsg,
	}, nil
}

// CreateVouch covers POST /api/v1/vouches.
func (r *mutationResolver) CreateVouch(ctx context.Context, input model.CreateVouchInput) (*model.Vouch, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	to := strings.TrimSpace(input.To)
	if err := common.ValidateRequiredParam("to", to); err != nil {
		return nil, err
	}
	if err := common.ValidateFloatRange("confidence", input.Confidence, 0, 1); err != nil {
		return nil, err
	}

	if r.Config != nil && !strings.Contains(to, "://") && !strings.Contains(to, "@") {
		to = fmt.Sprintf("https://%s/users/%s", r.Config.Domain, to)
	}

	from := username
	if r.Config != nil {
		from = fmt.Sprintf("https://%s/users/%s", r.Config.Domain, username)
	}

	contextValue := ""
	if input.Context != nil {
		contextValue = strings.TrimSpace(*input.Context)
	}

	service, err := r.getReputationService()
	if err != nil {
		return nil, err
	}

	vouch, err := service.CreateVouch(ctx, from, to, input.Confidence, contextValue)
	if err != nil {
		r.Logger.Error("failed to create vouch", zap.Error(err))
		return nil, errors.Join(errors.New("failed to create vouch"), err)
	}

	return r.convertVouchToGraphQL(ctx, vouch), nil
}

// RevokeVouch covers DELETE /api/v1/vouches/{vouch_id}.
func (r *mutationResolver) RevokeVouch(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	vouchID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", vouchID); err != nil {
		return false, err
	}

	actorID := username
	if r.Config != nil {
		actorID = fmt.Sprintf("https://%s/users/%s", r.Config.Domain, username)
	}

	service, err := r.getReputationService()
	if err != nil {
		return false, err
	}

	if err := service.RevokeVouch(ctx, vouchID, actorID); err != nil {
		r.Logger.Error("failed to revoke vouch", zap.Error(err))
		return false, errors.Join(errors.New("failed to revoke vouch"), err)
	}

	return true, nil
}
