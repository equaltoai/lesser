package cost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type delegatingStorage struct {
	recordedCost    *FederationCost
	healthUpdated   *InstanceHealth
	configSaved     *InstanceConfig
	costRetrieved   *FederationCost
	healthRetrieved *InstanceHealth

	costMetricsRetrieved *CostMetrics
	unhealthyRetrieved   []*InstanceHealth
	configRetrieved      *InstanceConfig
	configsListRetrieved []*InstanceConfig
}

func (s *delegatingStorage) RecordCost(_ context.Context, cost *FederationCost) error {
	s.recordedCost = cost
	return nil
}
func (s *delegatingStorage) GetInstanceCost(_ context.Context, _ string, _ string) (*FederationCost, error) {
	return s.costRetrieved, nil
}
func (s *delegatingStorage) GetCostMetrics(_ context.Context, _ string) (*CostMetrics, error) {
	return s.costMetricsRetrieved, nil
}
func (s *delegatingStorage) UpdateInstanceHealth(_ context.Context, health *InstanceHealth) error {
	s.healthUpdated = health
	return nil
}
func (s *delegatingStorage) GetInstanceHealth(_ context.Context, _ string) (*InstanceHealth, error) {
	return s.healthRetrieved, nil
}
func (s *delegatingStorage) ListUnhealthyInstances(_ context.Context) ([]*InstanceHealth, error) {
	return s.unhealthyRetrieved, nil
}
func (s *delegatingStorage) SaveInstanceConfig(_ context.Context, config *InstanceConfig) error {
	s.configSaved = config
	return nil
}
func (s *delegatingStorage) GetInstanceConfig(_ context.Context, _ string) (*InstanceConfig, error) {
	return s.configRetrieved, nil
}
func (s *delegatingStorage) ListInstanceConfigs(_ context.Context) ([]*InstanceConfig, error) {
	return s.configsListRetrieved, nil
}

func TestDynamoStorage_Delegates(t *testing.T) {
	underlying := &delegatingStorage{
		costRetrieved:   &FederationCost{InstanceDomain: "example.com"},
		healthRetrieved: &InstanceHealth{Domain: "example.com"},
		costMetricsRetrieved: &CostMetrics{
			Period: "2026-01",
		},
		unhealthyRetrieved: []*InstanceHealth{
			{Domain: "unhealthy.example"},
		},
		configRetrieved:      &InstanceConfig{Domain: "config.example"},
		configsListRetrieved: []*InstanceConfig{{Domain: "list.example"}},
	}

	s := NewDynamoStorage(underlying, zap.NewNop(), nil)

	cost := &FederationCost{InstanceDomain: "example.com", BillingPeriod: "2026-01"}
	assert.NoError(t, s.RecordCost(context.Background(), cost))
	assert.Equal(t, cost, underlying.recordedCost)

	gotCost, err := s.GetInstanceCost(context.Background(), "example.com", "2026-01")
	assert.NoError(t, err)
	assert.Equal(t, underlying.costRetrieved, gotCost)

	health := &InstanceHealth{Domain: "example.com", LastHealthCheck: time.Now()}
	assert.NoError(t, s.UpdateInstanceHealth(context.Background(), health))
	assert.Equal(t, health, underlying.healthUpdated)

	gotHealth, err := s.GetInstanceHealth(context.Background(), "example.com")
	assert.NoError(t, err)
	assert.Equal(t, underlying.healthRetrieved, gotHealth)

	cfg := &InstanceConfig{Domain: "example.com"}
	assert.NoError(t, s.SaveInstanceConfig(context.Background(), cfg))
	assert.Equal(t, cfg, underlying.configSaved)

	metrics, err := s.GetCostMetrics(context.Background(), "2026-01")
	assert.NoError(t, err)
	assert.Equal(t, underlying.costMetricsRetrieved, metrics)

	unhealthy, err := s.ListUnhealthyInstances(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, underlying.unhealthyRetrieved, unhealthy)

	cfg2, err := s.GetInstanceConfig(context.Background(), "config.example")
	assert.NoError(t, err)
	assert.Equal(t, underlying.configRetrieved, cfg2)

	configs, err := s.ListInstanceConfigs(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, underlying.configsListRetrieved, configs)
}
