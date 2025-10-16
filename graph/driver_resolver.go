package graph

import (
	"context"

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
func (r *driverResolver) Type(ctx context.Context, obj *cost.Driver) (string, error) {
	if obj.Operation != "" && obj.Operation != "All" {
		return obj.Service + " " + obj.Operation, nil
	}
	return obj.Service, nil
}

// Domain resolves the domain field for a Driver
// This is optional and typically nil for cost drivers
func (r *driverResolver) Domain(ctx context.Context, obj *cost.Driver) (*string, error) {
	return nil, nil
}

// Cost resolves the cost field for a Driver
// Converts from microcents to dollars
func (r *driverResolver) Cost(ctx context.Context, obj *cost.Driver) (float64, error) {
	return float64(obj.CostMicroCents) / 1_000_000.0, nil
}

// PercentOfTotal resolves the percentOfTotal field for a Driver
// Maps from PercentageOfTotal field
func (r *driverResolver) PercentOfTotal(ctx context.Context, obj *cost.Driver) (float64, error) {
	return obj.PercentageOfTotal, nil
}

// Trend resolves the trend field for a Driver
// Defaults to STABLE for now (could be enhanced with historical data)
func (r *driverResolver) Trend(ctx context.Context, obj *cost.Driver) (model.Trend, error) {
	// TODO: Calculate actual trend based on historical data
	return model.TrendStable, nil
}
