// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// TrackingRepository is a thread-safe in-memory implementation of interfaces.TrackingRepository.
type TrackingRepository struct {
	mu sync.RWMutex
	// records stores cost records keyed by "operationType:id:timestamp"
	records map[string]*models.DynamoDBCostRecord
	// recordsByOp stores record keys by operation type
	recordsByOp map[string][]string
	// recordsByTable stores record keys by table name
	recordsByTable map[string][]string
	// aggregations stores aggregated records
	aggregations map[string]*models.DynamoDBCostAggregation
	// relayCosts stores relay cost records
	relayCosts map[string][]*models.RelayCost
	// relayMetrics stores relay metrics
	relayMetrics map[string]*models.RelayMetrics
	// relayBudgets stores relay budgets
	relayBudgets map[string]*models.RelayBudget
}

// NewTrackingRepository creates a new in-memory tracking repository
func NewTrackingRepository() *TrackingRepository {
	return &TrackingRepository{
		records:        make(map[string]*models.DynamoDBCostRecord),
		recordsByOp:   make(map[string][]string),
		recordsByTable: make(map[string][]string),
		aggregations:  make(map[string]*models.DynamoDBCostAggregation),
		relayCosts:    make(map[string][]*models.RelayCost),
		relayMetrics:  make(map[string]*models.RelayMetrics),
		relayBudgets:  make(map[string]*models.RelayBudget),
	}
}

func (r *TrackingRepository) makeKey(opType, id string, ts time.Time) string {
	return opType + ":" + id + ":" + ts.Format(time.RFC3339Nano)
}

// Create creates a new cost tracking record
func (r *TrackingRepository) Create(_ context.Context, tracking *models.DynamoDBCostRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(tracking.OperationType, tracking.ID, tracking.Timestamp)
	r.records[key] = tracking
	r.recordsByOp[tracking.OperationType] = append(r.recordsByOp[tracking.OperationType], key)
	r.recordsByTable[tracking.Table] = append(r.recordsByTable[tracking.Table], key)
	return nil
}


// BatchCreate creates multiple cost tracking records efficiently
func (r *TrackingRepository) BatchCreate(ctx context.Context, trackingList []*models.DynamoDBCostRecord) error {
	for _, tracking := range trackingList {
		if err := r.Create(ctx, tracking); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a cost tracking record
func (r *TrackingRepository) Get(_ context.Context, operationType, id string, timestamp time.Time) (*models.DynamoDBCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeKey(operationType, id, timestamp)
	record, exists := r.records[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return record, nil
}

// ListByOperationType lists cost tracking records by operation type
func (r *TrackingRepository) ListByOperationType(_ context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DynamoDBCostRecord
	for _, key := range r.recordsByOp[operationType] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				results = append(results, record)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// ListByTable lists cost tracking records by table
func (r *TrackingRepository) ListByTable(_ context.Context, tableName string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostRecord, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DynamoDBCostRecord
	for _, key := range r.recordsByTable[tableName] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				results = append(results, record)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, "", nil
}

// GetRecentCosts retrieves recent cost tracking records
func (r *TrackingRepository) GetRecentCosts(_ context.Context, since time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DynamoDBCostRecord
	for _, record := range r.records {
		if record.Timestamp.After(since) {
			results = append(results, record)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// GetAggregated retrieves aggregated cost tracking
func (r *TrackingRepository) GetAggregated(_ context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := period + ":" + operationType + ":" + windowStart.Format(time.RFC3339)
	agg, exists := r.aggregations[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return agg, nil
}

// CreateAggregated creates an aggregated cost tracking record
func (r *TrackingRepository) CreateAggregated(_ context.Context, aggregated *models.DynamoDBCostAggregation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := aggregated.Period + ":" + aggregated.OperationType + ":" + aggregated.WindowStart.Format(time.RFC3339)
	r.aggregations[key] = aggregated
	return nil
}

// UpdateAggregated updates an existing aggregated cost tracking record
func (r *TrackingRepository) UpdateAggregated(_ context.Context, aggregated *models.DynamoDBCostAggregation) error {
	return r.CreateAggregated(context.Background(), aggregated)
}


// ListAggregatedByPeriod lists aggregated cost tracking for a period
func (r *TrackingRepository) ListAggregatedByPeriod(_ context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DynamoDBCostAggregation
	for _, agg := range r.aggregations {
		if agg.Period == period && agg.OperationType == operationType {
			if agg.WindowStart.After(startTime) && agg.WindowStart.Before(endTime) {
				results = append(results, agg)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, "", nil
}

// Aggregate performs aggregation of raw cost tracking data
func (r *TrackingRepository) Aggregate(_ context.Context, operationType, period string, windowStart, windowEnd time.Time) error {
	return nil // No-op for in-memory
}

// GetAggregatedCostsByPeriod retrieves aggregated costs for a specific period
func (r *TrackingRepository) GetAggregatedCostsByPeriod(_ context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DynamoDBCostAggregation
	for _, agg := range r.aggregations {
		if agg.Period == period && agg.WindowStart.After(startDate) && agg.WindowStart.Before(endDate) {
			results = append(results, agg)
		}
	}
	return results, nil
}

// GetTableCostStats calculates cost statistics for a table
func (r *TrackingRepository) GetTableCostStats(_ context.Context, tableName string, startTime, endTime time.Time) (*interfaces.TableCostStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &interfaces.TableCostStats{
		TableName:          tableName,
		StartTime:          startTime,
		EndTime:            endTime,
		OperationBreakdown: make(map[string]interfaces.OperationCostStats),
	}

	for _, key := range r.recordsByTable[tableName] {
		if record, exists := r.records[key]; exists {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				stats.Count++
				stats.TotalCostMicroCents += record.TotalCostMicroCents
			}
		}
	}
	stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1000000.0
	return stats, nil
}

// GetHighCostOperations returns operations that exceed a cost threshold
func (r *TrackingRepository) GetHighCostOperations(_ context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thresholdMicroCents := int64(thresholdDollars * 1000000)
	var results []*models.DynamoDBCostRecord
	for _, record := range r.records {
		if record.TotalCostMicroCents >= thresholdMicroCents {
			if record.Timestamp.After(startTime) && record.Timestamp.Before(endTime) {
				results = append(results, record)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// GetCostTrends calculates cost trends over time
func (r *TrackingRepository) GetCostTrends(_ context.Context, period string, operationType string, lookbackDays int) (*interfaces.CostTrend, error) {
	return &interfaces.CostTrend{Period: period, OperationType: operationType}, nil
}

// GetCostsByOperationType retrieves costs grouped by operation type
func (r *TrackingRepository) GetCostsByOperationType(_ context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	return make(map[string]*models.DynamoDBServiceCostStats), nil
}

// GetCostsByService retrieves costs grouped by service/function
func (r *TrackingRepository) GetCostsByService(_ context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	return make(map[string]*models.DynamoDBServiceCostStats), nil
}

// GetCostsByDateRange returns individual cost records for the specified date range
func (r *TrackingRepository) GetCostsByDateRange(_ context.Context, startDate, endDate time.Time) ([]*models.DynamoDBCostRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DynamoDBCostRecord
	for _, record := range r.records {
		if record.Timestamp.After(startDate) && record.Timestamp.Before(endDate) {
			results = append(results, record)
		}
	}
	return results, nil
}

// GetDailyAggregates returns aggregated daily costs
func (r *TrackingRepository) GetDailyAggregates(_ context.Context, startDate, endDate time.Time) ([]*interfaces.DailyAggregate, error) {
	return []*interfaces.DailyAggregate{}, nil
}

// GetMonthlyAggregate returns aggregated costs for the specified month
func (r *TrackingRepository) GetMonthlyAggregate(_ context.Context, year, month int) (*interfaces.MonthlyAggregate, error) {
	return &interfaces.MonthlyAggregate{Year: year, Month: month}, nil
}

// GetCostProjections retrieves the most recent cost projection
func (r *TrackingRepository) GetCostProjections(_ context.Context, period string) (*storage.CostProjection, error) {
	return nil, storage.ErrNotFound
}


// CreateRelayCost creates a new relay cost record
func (r *TrackingRepository) CreateRelayCost(_ context.Context, relayCost *models.RelayCost) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.relayCosts[relayCost.RelayURL] = append(r.relayCosts[relayCost.RelayURL], relayCost)
	return nil
}

// GetRelayCostsByURL retrieves relay costs for a specific relay URL
func (r *TrackingRepository) GetRelayCostsByURL(_ context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string, operationType string) ([]*models.RelayCost, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.RelayCost
	for _, cost := range r.relayCosts[relayURL] {
		if cost.Timestamp.After(startTime) && cost.Timestamp.Before(endTime) {
			if operationType == "" || cost.OperationType == operationType {
				results = append(results, cost)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, "", nil
}

// GetRelayCostsByDateRange retrieves relay costs for all relays within a date range
func (r *TrackingRepository) GetRelayCostsByDateRange(_ context.Context, startDate, endDate time.Time, limit int) ([]*models.RelayCost, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.RelayCost
	for _, costs := range r.relayCosts {
		for _, cost := range costs {
			if cost.Timestamp.After(startDate) && cost.Timestamp.Before(endDate) {
				results = append(results, cost)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// CreateRelayMetrics creates or updates relay metrics
func (r *TrackingRepository) CreateRelayMetrics(_ context.Context, metrics *models.RelayMetrics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := metrics.RelayURL + ":" + metrics.Period + ":" + metrics.WindowStart.Format(time.RFC3339)
	r.relayMetrics[key] = metrics
	return nil
}

// UpdateRelayMetrics updates existing relay metrics
func (r *TrackingRepository) UpdateRelayMetrics(_ context.Context, metrics *models.RelayMetrics) error {
	return r.CreateRelayMetrics(context.Background(), metrics)
}

// GetRelayMetrics retrieves relay metrics for a specific relay and period
func (r *TrackingRepository) GetRelayMetrics(_ context.Context, relayURL, period string, windowStart time.Time) (*models.RelayMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := relayURL + ":" + period + ":" + windowStart.Format(time.RFC3339)
	metrics, exists := r.relayMetrics[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return metrics, nil
}

// GetRelayMetricsHistory retrieves metrics history for a relay
func (r *TrackingRepository) GetRelayMetricsHistory(_ context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string) ([]*models.RelayMetrics, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.RelayMetrics
	for key, metrics := range r.relayMetrics {
		if len(key) > len(relayURL) && key[:len(relayURL)] == relayURL {
			if metrics.WindowStart.After(startTime) && metrics.WindowStart.Before(endTime) {
				results = append(results, metrics)
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, "", nil
}

// CreateRelayBudget creates a new relay budget configuration
func (r *TrackingRepository) CreateRelayBudget(_ context.Context, budget *models.RelayBudget) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.relayBudgets[budget.RelayURL] = budget
	return nil
}

// Clear clears all data (test helper)
func (r *TrackingRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = make(map[string]*models.DynamoDBCostRecord)
	r.recordsByOp = make(map[string][]string)
	r.recordsByTable = make(map[string][]string)
	r.aggregations = make(map[string]*models.DynamoDBCostAggregation)
	r.relayCosts = make(map[string][]*models.RelayCost)
	r.relayMetrics = make(map[string]*models.RelayMetrics)
	r.relayBudgets = make(map[string]*models.RelayBudget)
}

// Ensure TrackingRepository implements interfaces.TrackingRepository
var _ interfaces.TrackingRepository = (*TrackingRepository)(nil)
