package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/cost"
)

// driverResolver implements the DriverResolver interface
type driverResolver struct{ *Resolver }

// Driver returns the DriverResolver
func (r *Resolver) Driver() DriverResolver {
	return &driverResolver{r}
}

// Type resolves the type field for a Driver
// Maps from the Service and Operation fields
func (r *driverResolver) Type(_ context.Context, obj *cost.Driver) (string, error) {
	if obj.Operation != "" && obj.Operation != allOperationsValue {
		return obj.Service + " " + obj.Operation, nil
	}
	return obj.Service, nil
}

// Domain resolves the domain field for a Driver
// This is optional and typically nil for cost drivers
func (r *driverResolver) Domain(_ context.Context, _ *cost.Driver) (*string, error) {
	return nil, nil
}

// Cost resolves the cost field for a Driver
func (r *driverResolver) Cost(_ context.Context, obj *cost.Driver) (float64, error) {
	// Convert microcents to dollars
	return float64(obj.CostMicroCents) / 1000000.0, nil
}

// PercentOfTotal resolves the percentOfTotal field for a Driver
// Maps from PercentageOfTotal field
func (r *driverResolver) PercentOfTotal(_ context.Context, obj *cost.Driver) (float64, error) {
	return obj.PercentageOfTotal, nil
}

// Trend resolves the trend field for a Driver
// Reads the pre-calculated trend from the Driver object
func (r *driverResolver) Trend(_ context.Context, obj *cost.Driver) (model.Trend, error) {
	// Convert string trend to GraphQL enum
	switch strings.ToUpper(obj.Trend) {
	case "INCREASING":
		return model.TrendIncreasing, nil
	case "DECREASING":
		return model.TrendDecreasing, nil
	default:
		return model.TrendStable, nil
	}
}
