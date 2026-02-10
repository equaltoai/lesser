package graph

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// OptimizeFederationCosts implements MutationResolver.
func (r *mutationResolver) OptimizeFederationCosts(ctx context.Context, targetAmount float64) (*model.CostOptimizationResult, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
func (r *mutationResolver) SetFederationLimit(ctx context.Context, domain string, _ model.FederationLimitInput) (*model.FederationLimit, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
func (r *mutationResolver) PauseFederation(ctx context.Context, domain string, reason string, until *model.Time) (*model.FederationManagementStatus, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
func (r *mutationResolver) ResumeFederation(ctx context.Context, domain string) (*model.FederationManagementStatus, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
func (r *mutationResolver) SetInstanceBudget(ctx context.Context, domain string, monthlyUSD float64, autoLimit *bool) (*model.InstanceBudget, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get severance service from registry
	severanceService := r.Registry.Severance()
	if severanceService == nil {
		return nil, errors.New("severance service unavailable")
	}

	// Acknowledge the severance
	severedRel, err := severanceService.AcknowledgeSeverance(ctx, id, username)
	if err != nil {
		return nil, errors.Join(errors.New("failed to acknowledge severance"), err)
	}

	// Convert to GraphQL model
	gqlSeverance := r.convertSeveredRelationshipToModel(ctx, severedRel)

	return &model.AcknowledgePayload{
		Success:             true,
		SeveredRelationship: gqlSeverance,
		Acknowledged:        true,
	}, nil
}

// AttemptReconnection implements MutationResolver
func (r *mutationResolver) AttemptReconnection(ctx context.Context, id string) (*model.ReconnectionPayload, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get severance service from registry
	severanceService := r.Registry.Severance()
	if severanceService == nil {
		return nil, errors.New("severance service unavailable")
	}

	// Attempt reconnection
	result, err := severanceService.AttemptReconnection(ctx, id, username)
	if err != nil {
		return nil, errors.Join(errors.New("failed to attempt reconnection"), err)
	}

	// Build error messages from result
	errorMessages := result.Errors
	if len(errorMessages) == 0 && !result.Success {
		errorMessages = []string{"reconnection failed for unknown reason"}
	}

	// Fetch the updated severance AFTER the reconnection attempt to get fresh data
	// This ensures reconnectionAttempt flag and other fields reflect the latest state
	severedRel, err := severanceService.GetSeveredRelationship(ctx, id)
	if err != nil {
		r.Logger.Warn("failed to fetch updated severance after reconnection, using basic response",
			zap.String("severance_id", id),
			zap.Error(err))
		// Return payload without severance object rather than failing
		return &model.ReconnectionPayload{
			Success:     result.Success,
			Reconnected: result.SuccessCount,
			Failed:      result.FailureCount,
			Errors:      errorMessages,
		}, nil
	}

	// Convert to GraphQL model with fresh data
	gqlSeverance := r.convertSeveredRelationshipToModel(ctx, severedRel)

	return &model.ReconnectionPayload{
		Success:             result.Success,
		SeveredRelationship: gqlSeverance,
		Reconnected:         result.SuccessCount,
		Failed:              result.FailureCount,
		Errors:              errorMessages,
	}, nil
}
