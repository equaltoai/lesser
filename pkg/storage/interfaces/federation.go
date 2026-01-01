// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
)

// FederationRepository defines the interface for federation tracking operations.
// This handles federated instance information, statistics, and cost tracking.
type FederationRepository interface {
	// Instance information operations

	// GetInstanceInfo retrieves information about a federated instance
	GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error)

	// UpsertInstanceInfo creates or updates instance information
	UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error

	// GetKnownInstances retrieves a list of known federated instances
	GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error)

	// Statistics operations

	// GetFederationStatistics retrieves federation statistics for a time range
	GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error)

	// GetInstanceStats retrieves comprehensive statistics for a specific instance
	GetInstanceStats(ctx context.Context, domain string) (*storage.InstanceStats, error)

	// Activity and cost tracking operations

	// RecordFederationActivity records a single federation activity for cost tracking
	RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error

	// GetFederationCosts retrieves aggregated federation costs
	GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error)

	// Health and projections operations

	// GetInstanceHealthReport generates a health report for a specific instance
	GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error)

	// GetCostProjections generates cost projections based on historical data
	GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error)

	// Federation graph operations

	// GetFederationNodes retrieves federation nodes up to a certain depth
	GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error)

	// GetFederationNodesByHealth retrieves federation nodes filtered by health status
	GetFederationNodesByHealth(ctx context.Context, healthStatus string, limit int) ([]*storage.FederationNode, error)

	// GetFederationEdges retrieves edges between specified domains
	GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error)

	// GetInstanceMetadata retrieves metadata for a specific instance
	GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error)

	// CalculateFederationClusters calculates instance clusters based on connections
	CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error)
}
