package cost

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

type adapterFakeDB struct {
	query core.Query
}

func (db *adapterFakeDB) Model(_ any) core.Query                      { return db.query }
func (db *adapterFakeDB) Transaction(_ func(tx *core.Tx) error) error { return nil }
func (db *adapterFakeDB) Migrate() error                              { return nil }
func (db *adapterFakeDB) AutoMigrate(_ ...any) error                  { return nil }
func (db *adapterFakeDB) Close() error                                { return nil }
func (db *adapterFakeDB) WithContext(_ context.Context) core.DB       { return db }

type adapterFakeQuery struct {
	firstErr   error
	firstValue any

	allErr   error
	allValue any

	createErr error
	updateErr error
}

func (q *adapterFakeQuery) Where(_ string, _ string, _ any) core.Query                    { return q }
func (q *adapterFakeQuery) Index(_ string) core.Query                                     { return q }
func (q *adapterFakeQuery) Filter(_ string, _ string, _ any) core.Query                   { return q }
func (q *adapterFakeQuery) OrFilter(_ string, _ string, _ any) core.Query                 { return q }
func (q *adapterFakeQuery) FilterGroup(_ func(core.Query)) core.Query                     { return q }
func (q *adapterFakeQuery) OrFilterGroup(_ func(core.Query)) core.Query                   { return q }
func (q *adapterFakeQuery) IfNotExists() core.Query                                       { return q }
func (q *adapterFakeQuery) IfExists() core.Query                                          { return q }
func (q *adapterFakeQuery) WithCondition(_ string, _ string, _ any) core.Query            { return q }
func (q *adapterFakeQuery) WithConditionExpression(_ string, _ map[string]any) core.Query { return q }
func (q *adapterFakeQuery) OrderBy(_ string, _ string) core.Query                         { return q }
func (q *adapterFakeQuery) Limit(_ int) core.Query                                        { return q }
func (q *adapterFakeQuery) Offset(_ int) core.Query                                       { return q }
func (q *adapterFakeQuery) Select(_ ...string) core.Query                                 { return q }
func (q *adapterFakeQuery) ConsistentRead() core.Query                                    { return q }
func (q *adapterFakeQuery) WithRetry(_ int, _ time.Duration) core.Query                   { return q }
func (q *adapterFakeQuery) Cursor(_ string) core.Query                                    { return q }
func (q *adapterFakeQuery) SetCursor(_ string) error                                      { return nil }
func (q *adapterFakeQuery) WithContext(_ context.Context) core.Query                      { return q }
func (q *adapterFakeQuery) UpdateBuilder() core.UpdateBuilder                             { return &adapterFakeUpdateBuilder{} }
func (q *adapterFakeQuery) BatchGetBuilder() core.BatchGetBuilder {
	return &adapterFakeBatchGetBuilder{}
}
func (q *adapterFakeQuery) ParallelScan(_ int32, _ int32) core.Query          { return q }
func (q *adapterFakeQuery) AllPaginated(_ any) (*core.PaginatedResult, error) { return nil, nil }
func (q *adapterFakeQuery) Count() (int64, error)                             { return 0, nil }
func (q *adapterFakeQuery) CreateOrUpdate() error                             { return nil }
func (q *adapterFakeQuery) Delete() error                                     { return nil }
func (q *adapterFakeQuery) Scan(_ any) error                                  { return nil }
func (q *adapterFakeQuery) ScanAllSegments(_ any, _ int32) error              { return nil }
func (q *adapterFakeQuery) BatchGet(_ []any, _ any) error                     { return nil }
func (q *adapterFakeQuery) BatchGetWithOptions(_ []any, _ any, _ *core.BatchGetOptions) error {
	return nil
}
func (q *adapterFakeQuery) BatchCreate(_ any) error                                    { return nil }
func (q *adapterFakeQuery) BatchDelete(_ []any) error                                  { return nil }
func (q *adapterFakeQuery) BatchWrite(_ []any, _ []any) error                          { return nil }
func (q *adapterFakeQuery) BatchUpdateWithOptions(_ []any, _ []string, _ ...any) error { return nil }

func (q *adapterFakeQuery) First(dest any) error {
	if q.firstErr != nil {
		return q.firstErr
	}
	if q.firstValue == nil {
		return nil
	}

	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}

	value := reflect.ValueOf(q.firstValue)
	rv.Elem().Set(value)
	return nil
}

func (q *adapterFakeQuery) All(dest any) error {
	if q.allErr != nil {
		return q.allErr
	}
	if q.allValue == nil {
		return nil
	}

	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}

	value := reflect.ValueOf(q.allValue)
	if rv.Elem().Kind() != reflect.Slice || value.Kind() != reflect.Slice {
		return nil
	}

	rv.Elem().Set(value)
	return nil
}

func (q *adapterFakeQuery) Create() error { return q.createErr }

func (q *adapterFakeQuery) Update(_ ...string) error { return q.updateErr }

type adapterFakeUpdateBuilder struct{}

func (b *adapterFakeUpdateBuilder) Set(_ string, _ any) core.UpdateBuilder { return b }
func (b *adapterFakeUpdateBuilder) SetIfNotExists(_ string, _ any, _ any) core.UpdateBuilder {
	return b
}
func (b *adapterFakeUpdateBuilder) Add(_ string, _ any) core.UpdateBuilder              { return b }
func (b *adapterFakeUpdateBuilder) Increment(_ string) core.UpdateBuilder               { return b }
func (b *adapterFakeUpdateBuilder) Decrement(_ string) core.UpdateBuilder               { return b }
func (b *adapterFakeUpdateBuilder) Remove(_ string) core.UpdateBuilder                  { return b }
func (b *adapterFakeUpdateBuilder) Delete(_ string, _ any) core.UpdateBuilder           { return b }
func (b *adapterFakeUpdateBuilder) AppendToList(_ string, _ any) core.UpdateBuilder     { return b }
func (b *adapterFakeUpdateBuilder) PrependToList(_ string, _ any) core.UpdateBuilder    { return b }
func (b *adapterFakeUpdateBuilder) RemoveFromListAt(_ string, _ int) core.UpdateBuilder { return b }
func (b *adapterFakeUpdateBuilder) SetListElement(_ string, _ int, _ any) core.UpdateBuilder {
	return b
}
func (b *adapterFakeUpdateBuilder) Condition(_ string, _ string, _ any) core.UpdateBuilder { return b }
func (b *adapterFakeUpdateBuilder) OrCondition(_ string, _ string, _ any) core.UpdateBuilder {
	return b
}
func (b *adapterFakeUpdateBuilder) ConditionExists(_ string) core.UpdateBuilder    { return b }
func (b *adapterFakeUpdateBuilder) ConditionNotExists(_ string) core.UpdateBuilder { return b }
func (b *adapterFakeUpdateBuilder) ConditionVersion(_ int64) core.UpdateBuilder    { return b }
func (b *adapterFakeUpdateBuilder) ReturnValues(_ string) core.UpdateBuilder       { return b }
func (b *adapterFakeUpdateBuilder) Execute() error                                 { return nil }
func (b *adapterFakeUpdateBuilder) ExecuteWithResult(_ any) error                  { return nil }

type adapterFakeBatchGetBuilder struct{}

func (b *adapterFakeBatchGetBuilder) Keys(_ []any) core.BatchGetBuilder    { return b }
func (b *adapterFakeBatchGetBuilder) ChunkSize(_ int) core.BatchGetBuilder { return b }
func (b *adapterFakeBatchGetBuilder) ConsistentRead() core.BatchGetBuilder { return b }
func (b *adapterFakeBatchGetBuilder) Parallel(_ int) core.BatchGetBuilder  { return b }
func (b *adapterFakeBatchGetBuilder) WithRetry(_ *core.RetryPolicy) core.BatchGetBuilder {
	return b
}
func (b *adapterFakeBatchGetBuilder) Select(_ ...string) core.BatchGetBuilder { return b }
func (b *adapterFakeBatchGetBuilder) OnProgress(_ core.BatchProgressCallback) core.BatchGetBuilder {
	return b
}
func (b *adapterFakeBatchGetBuilder) OnError(_ core.BatchChunkErrorHandler) core.BatchGetBuilder {
	return b
}
func (b *adapterFakeBatchGetBuilder) Execute(_ any) error { return nil }

func TestRepositoryAdapter_RecordAndGetCost(t *testing.T) {
	q := &adapterFakeQuery{}
	db := &adapterFakeDB{query: q}
	adapter := NewRepositoryAdapter(db, zap.NewNop(), nil)

	cost := &FederationCost{
		InstanceDomain: "example.com",
		IngressBytes:   10,
		EgressBytes:    20,
		RequestCount:   2,
		ErrorCount:     1,
		ErrorRate:      0.5,
		AverageCostUSD: 1.5,
		LastUpdated:    time.Now(),
		BillingPeriod:  "2026-01",
	}
	assert.NoError(t, adapter.RecordCost(context.Background(), cost))

	q.firstErr = dynamormErrors.ErrItemNotFound
	got, err := adapter.GetInstanceCost(context.Background(), "example.com", "2026-01")
	assert.NoError(t, err)
	assert.Nil(t, got)

	q.firstErr = nil
	q.firstValue = models.FederationCostTracking{
		InstanceDomain: "example.com",
		IngressBytes:   10,
		EgressBytes:    20,
		RequestCount:   2,
		ErrorCount:     1,
		ErrorRate:      0.5,
		AverageCostUSD: 1.5,
		LastUpdated:    time.Now(),
		BillingPeriod:  "2026-01",
	}
	got, err = adapter.GetInstanceCost(context.Background(), "example.com", "2026-01")
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, "example.com", got.InstanceDomain)

	q.createErr = errors.New("create failed")
	assert.Error(t, adapter.RecordCost(context.Background(), cost))
}

func TestRepositoryAdapter_GetCostMetrics(t *testing.T) {
	q := &adapterFakeQuery{
		allValue: []models.FederationCostTracking{
			{InstanceDomain: "a.example", AverageCostUSD: 0.5, RequestCount: 2, EgressBytes: 1024, IngressBytes: 2048},
			{InstanceDomain: "b.example", AverageCostUSD: 1.0, RequestCount: 1, EgressBytes: 4096, IngressBytes: 0},
		},
	}
	db := &adapterFakeDB{query: q}
	adapter := NewRepositoryAdapter(db, zap.NewNop(), nil)

	metrics, err := adapter.GetCostMetrics(context.Background(), "2026-01")
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.InDelta(t, 2.0, metrics.TotalCostUSD, 0.0001)
	assert.Equal(t, int64(3), metrics.RequestCount)
}

func TestRepositoryAdapter_HealthAndConfig(t *testing.T) {
	q := &adapterFakeQuery{}
	db := &adapterFakeDB{query: q}
	adapter := NewRepositoryAdapter(db, zap.NewNop(), nil)

	health := &InstanceHealth{
		Domain:          "example.com",
		HealthScore:     0.5,
		ResponseTimeP95: 1000,
		SuccessRate:     0.9,
		LastHealthCheck: time.Now(),
		IsHealthy:       false,
	}
	assert.NoError(t, adapter.UpdateInstanceHealth(context.Background(), health))

	q.firstErr = dynamormErrors.ErrItemNotFound
	gotHealth, err := adapter.GetInstanceHealth(context.Background(), "missing.example")
	assert.NoError(t, err)
	assert.Nil(t, gotHealth)

	q.firstErr = nil
	q.firstValue = models.FederationInstanceHealthTracking{
		Domain:           "example.com",
		HealthScore:      0.5,
		ResponseTimeP95:  1000,
		SuccessRate:      0.9,
		LastHealthCheck:  time.Now(),
		ConsecutiveFails: 2,
		IsHealthy:        false,
	}
	gotHealth, err = adapter.GetInstanceHealth(context.Background(), "example.com")
	assert.NoError(t, err)
	assert.NotNil(t, gotHealth)
	assert.Equal(t, "example.com", gotHealth.Domain)

	q.allValue = []models.FederationInstanceHealthTracking{
		{Domain: "a.example", HealthScore: 0.1, IsHealthy: false},
		{Domain: "b.example", HealthScore: 0.2, IsHealthy: false},
	}
	unhealthy, err := adapter.ListUnhealthyInstances(context.Background())
	assert.NoError(t, err)
	assert.Len(t, unhealthy, 2)

	now := time.Now()
	overrideBudget := 5.0
	rateLimit := 100
	config := &InstanceConfig{
		Domain:            "example.com",
		Tier:              TierPremium,
		CustomBudgetUSD:   &overrideBudget,
		RateLimitOverride: &rateLimit,
		RetryPolicy: &RetryPolicy{
			MaxRetries:     1,
			InitialBackoff: 0,
			MaxBackoff:     0,
			BackoffFactor:  1.0,
		},
		Created:      now,
		LastModified: now,
	}
	assert.NoError(t, adapter.SaveInstanceConfig(context.Background(), config))

	q.firstValue = models.FederationInstanceConfigTracking{
		Domain:            "example.com",
		Tier:              models.FederationTier(TierPremium),
		CustomBudgetUSD:   &overrideBudget,
		RateLimitOverride: &rateLimit,
		RetryPolicy: &models.RetryPolicy{
			MaxRetries:     1,
			InitialBackoff: 0,
			MaxBackoff:     0,
			BackoffFactor:  1.0,
		},
		Created:      now,
		LastModified: now,
	}
	gotConfig, err := adapter.GetInstanceConfig(context.Background(), "example.com")
	assert.NoError(t, err)
	assert.NotNil(t, gotConfig)
	assert.NotNil(t, gotConfig.RetryPolicy)

	q.allValue = []models.FederationInstanceConfigTracking{
		{Domain: "a.example"},
	}
	configs, err := adapter.ListInstanceConfigs(context.Background())
	assert.NoError(t, err)
	assert.Len(t, configs, 1)
}
