package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelayCost_BeforeCreate_ComputesTotals_TTL_AndKeys(t *testing.T) {
	baseTime := time.Unix(1700000000, 0).UTC()
	rc := &RelayCost{
		RelayURL:      "https://relay.example/relay",
		OperationType: "delivery",
		Direction:     "outbound",
		HTTPRequestCost:   10,
		DataTransferCost:  20,
		LambdaCost:        30,
		DynamoDBCost:      40,
		SQSCost:           50,
		RequestID:         "",
		Timestamp:         baseTime,
	}
	err := rc.BeforeCreate()
	assert.NoError(t, err)

	assert.Equal(t, int64(150), rc.TotalCostMicroCents)
	assert.NotEmpty(t, rc.RequestID)
	assert.Contains(t, rc.PK, "RELAY_COST#")
	assert.Contains(t, rc.SK, "TS#")
	assert.Contains(t, rc.GSI1PK, "RELAY_COSTS#")
	assert.Contains(t, rc.GSI2PK, "RELAY_COSTS_DAILY#")
	assert.True(t, rc.TTL > 0)

	assert.Equal(t, baseTime.Add(30*24*time.Hour).Unix(), rc.TTL)
}

func TestRelayMetrics_LifecycleAndBudgetCalculations(t *testing.T) {
	window := time.Unix(1700000000, 0).UTC()
	rm := &RelayMetrics{
		RelayURL:             "https://relay.example/relay",
		Period:               PeriodDaily,
		WindowStart:           window,
		TotalOperations:       10,
		SuccessfulOperations:  8,
		FailedOperations:      2,
		TotalDataTransferBytes: 2_000_000,
		TotalCostMicroCents:   1_000_000,
		BudgetLimitMicroCents: 900_000,
	}

	before := time.Now()
	err := rm.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "RELAY_METRICS#https://relay.example/relay#"+PeriodDaily, rm.PK)
	assert.Contains(t, rm.SK, "WINDOW#")
	assert.InDelta(t, 0.8, rm.SuccessRate, 0.00001)
	assert.True(t, rm.CostPerOperation > 0)
	assert.True(t, rm.CostPerMB > 0)
	assert.True(t, rm.BudgetUsedPercent > 100.0)
	assert.True(t, rm.BudgetExceeded)

	ttl := time.Unix(rm.TTL, 0)
	assert.True(t, ttl.After(before.Add(30*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(30*24*time.Hour+5*time.Second)))

	// BeforeUpdate recalculates derived metrics and keys.
	rm.TotalCostMicroCents = 500_000
	err = rm.BeforeUpdate()
	assert.NoError(t, err)
	assert.False(t, rm.BudgetExceeded)
}

func TestRelayBudget_Lifecycle_DefaultThresholds_AndKeys(t *testing.T) {
	before := time.Now()
	rb := &RelayBudget{
		RelayURL:        "https://relay.example/relay",
		Period:          PeriodMonthly,
		LimitMicroCents: 100,
		CurrentUsageMicroCents: 50,
	}
	err := rb.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "RELAY_BUDGET#https://relay.example/relay#"+PeriodMonthly, rb.PK)
	assert.Equal(t, SKConfig, rb.SK)
	assert.Equal(t, 75.0, rb.WarningThresholdPercent)
	assert.Equal(t, 90.0, rb.CriticalThresholdPercent)
	assert.InDelta(t, 50.0, rb.CurrentUsagePercent, 0.00001)
	assert.False(t, rb.BudgetExceeded)
	assert.True(t, rb.TTL > 0)

	ttl := time.Unix(rb.TTL, 0)
	assert.True(t, ttl.After(before.Add(365*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(365*24*time.Hour+5*time.Second)))

	// Validation failures.
	rb2 := &RelayBudget{RelayURL: "x", Period: PeriodDaily, LimitMicroCents: 0}
	assert.Error(t, rb2.BeforeCreate())

	// BeforeUpdate recalculates usage percent.
	rb.CurrentUsageMicroCents = 200
	assert.NoError(t, rb.BeforeUpdate())
	assert.True(t, rb.BudgetExceeded)
}
