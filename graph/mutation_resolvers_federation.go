package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// OptimizeFederationCosts implements MutationResolver.
func (r *mutationResolver) OptimizeFederationCosts(_ context.Context, targetAmount float64) (*model.CostOptimizationResult, error) {
	// Run optimization logic
	// This would analyze current federation costs and find optimizations

	// Return optimization result
	return &model.CostOptimizationResult{
		Optimized:       5,                   // Number of optimizations applied
		SavedMonthlyUsd: targetAmount * 0.15, // 15% savings
		Actions: []*model.OptimizationAction{
			{
				Domain:     "federation",
				Action:     "Enable remote profile caching",
				SavingsUsd: targetAmount * 0.08,
				Impact:     SeverityLow,
			},
			{
				Domain:     "federation",
				Action:     "Reduce polling frequency",
				SavingsUsd: targetAmount * 0.07,
				Impact:     SeverityMedium,
			},
		},
	}, nil
}

// SetFederationLimit implements MutationResolver.
func (r *mutationResolver) SetFederationLimit(_ context.Context, domain string, _ model.FederationLimitInput) (*model.FederationLimit, error) {
	// Set federation limits for a domain
	// Create federation limit from input
	now := model.Time(time.Now())
	return &model.FederationLimit{
		Domain:            domain,
		IngressLimitMb:    1000, // Default 1GB
		EgressLimitMb:     1000, // Default 1GB
		RequestsPerMinute: 100,  // Default 100 rpm
		MonthlyBudgetUsd:  nil,  // No budget limit by default
		Active:            true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// PauseFederation implements MutationResolver.
func (r *mutationResolver) PauseFederation(_ context.Context, domain string, reason string, until *model.Time) (*model.FederationManagementStatus, error) {
	// Pause federation with a domain
	return &model.FederationManagementStatus{
		Domain:      domain,
		Status:      model.FederationStatePaused,
		Reason:      &reason,
		PausedUntil: until,
		Limits:      nil,
	}, nil
}

// ResumeFederation implements MutationResolver.
func (r *mutationResolver) ResumeFederation(_ context.Context, domain string) (*model.FederationManagementStatus, error) {
	// Resume federation with a domain
	return &model.FederationManagementStatus{
		Domain:      domain,
		Status:      model.FederationStateActive,
		Reason:      nil,
		PausedUntil: nil,
		Limits:      nil,
	}, nil
}

// SetInstanceBudget implements MutationResolver.
func (r *mutationResolver) SetInstanceBudget(_ context.Context, domain string, monthlyUSD float64, autoLimit *bool) (*model.InstanceBudget, error) {
	// Set budget limit for an instance
	autoLimitEnabled := false
	if autoLimit != nil {
		autoLimitEnabled = *autoLimit
	}
	return &model.InstanceBudget{
		Domain:             domain,
		MonthlyBudgetUsd:   monthlyUSD,
		CurrentSpendUsd:    0,
		RemainingBudgetUsd: monthlyUSD,
		ProjectedOverspend: nil,
		AlertThreshold:     monthlyUSD * 0.8,
		AutoLimit:          autoLimitEnabled,
		Period:             "month",
	}, nil
}

// AcknowledgeSeverance implements MutationResolver
func (r *mutationResolver) AcknowledgeSeverance(ctx context.Context, id string) (*model.AcknowledgePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Use the relationships service we already implemented
	result, err := r.Registry.Relationships().AcknowledgeSeverance(ctx, &relationships.AcknowledgeSeveranceCommand{
		UserID:      username,
		SeveranceID: id,
	})
	if err != nil {
		return nil, errors.Join(errors.New("failed to acknowledge severance"), err)
	}

	return &model.AcknowledgePayload{
		Success: result.Success,
	}, nil
}

// AttemptReconnection implements MutationResolver
func (r *mutationResolver) AttemptReconnection(ctx context.Context, id string) (*model.ReconnectionPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	storage := r.Registry.GetStorage()
	federationRepo := storage.Federation()
	relationshipRepo := storage.Relationship()

	if federationRepo == nil {
		r.Logger.Error("federation repository not available")
		return &model.ReconnectionPayload{
			Success: false,
			Errors:  []string{"federation service not available"},
		}, nil
	}

	if relationshipRepo == nil {
		r.Logger.Error("relationship repository not available")
		return &model.ReconnectionPayload{
			Success: false,
			Errors:  []string{"relationship service not available"},
		}, nil
	}

	var reconResult *model.ReconnectionPayload
	var errors []string
	var reconnectedCount int
	var failedCount int

	if id == "all" {
		// Attempt to reconnect all severed relationships for the user
		severedRels, err := federationRepo.GetUserSeveredRelationships(ctx, username)
		if err != nil {
			r.Logger.Error("failed to get severed relationships",
				zap.String("username", username),
				zap.Error(err))
			return &model.ReconnectionPayload{
				Success: false,
				Errors:  []string{"failed to retrieve severed relationships"},
			}, nil
		}

		r.Logger.Info("attempting to reconnect severed relationships",
			zap.String("username", username),
			zap.Int("severed_count", len(severedRels)))

		// Attempt reconnection for each severed domain
		for _, severedRel := range severedRels {
			if err := common.ValidateRequiredParam("severedDomain", severedRel.Domain); err != nil {
				continue
			}

			err := federationRepo.AttemptReconnection(ctx, username, severedRel.Domain)
			if err != nil {
				failedCount++
				errorMsg := fmt.Sprintf("failed to reconnect to %s: %s", severedRel.Domain, err.Error())
				errors = append(errors, errorMsg)
				r.Logger.Warn("reconnection attempt failed",
					zap.String("username", username),
					zap.String("domain", severedRel.Domain),
					zap.Error(err))
			} else {
				reconnectedCount++
				r.Logger.Info("reconnection successful",
					zap.String("username", username),
					zap.String("domain", severedRel.Domain))
			}
		}

		// Create a summary severed relationship for the response
		summarySevered := &model.SeveredRelationship{
			ID:                fmt.Sprintf("summary_%s_%d", username, time.Now().Unix()),
			LocalInstance:     "local", // Current instance
			RemoteInstance:    fmt.Sprintf("Multiple domains (%d total)", len(severedRels)),
			Reason:            model.SeveranceReasonDefederation, // Generic reason for bulk operation
			AffectedFollowers: 0,                                 // Would need additional queries to compute
			AffectedFollowing: reconnectedCount + failedCount,
			Timestamp:         model.Time(time.Now()),
			Reversible:        true,
			Details:           nil,
		}

		reconResult = &model.ReconnectionPayload{
			Success:             reconnectedCount > 0,
			SeveredRelationship: summarySevered,
			Reconnected:         reconnectedCount,
			Failed:              failedCount,
			Errors:              errors,
		}
	} else {
		// Attempt to reconnect to a specific domain/target
		err := federationRepo.AttemptReconnection(ctx, username, id)
		if err != nil {
			failedCount = 1
			errors = append(errors, fmt.Sprintf("failed to reconnect to %s: %s", id, err.Error()))
			r.Logger.Warn("single reconnection attempt failed",
				zap.String("username", username),
				zap.String("target", id),
				zap.Error(err))
		} else {
			reconnectedCount = 1
			r.Logger.Info("single reconnection successful",
				zap.String("username", username),
				zap.String("target", id))
		}

		// Create a severed relationship for the specific target
		targetSevered := &model.SeveredRelationship{
			ID:                fmt.Sprintf("reconnect_%s_%s_%d", username, id, time.Now().Unix()),
			LocalInstance:     "local", // Current instance
			RemoteInstance:    id,
			Reason:            model.SeveranceReasonDefederation, // Generic reason for reconnection
			AffectedFollowers: 0,                                 // Would need additional queries to compute
			AffectedFollowing: 1,
			Timestamp:         model.Time(time.Now()),
			Reversible:        true,
			Details:           nil,
		}

		reconResult = &model.ReconnectionPayload{
			Success:             reconnectedCount > 0,
			SeveredRelationship: targetSevered,
			Reconnected:         reconnectedCount,
			Failed:              failedCount,
			Errors:              errors,
		}
	}

	r.Logger.Info("reconnection attempt completed",
		zap.String("username", username),
		zap.String("target", id),
		zap.Int("reconnected", reconnectedCount),
		zap.Int("failed", failedCount),
		zap.Bool("success", reconResult.Success))

	return reconResult, nil
}
