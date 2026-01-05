package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDynamoDBCostRecord_UpdateKeys_GeneratesIDTimestampAndGSIs(t *testing.T) {
	ct := &DynamoDBCostRecord{
		OperationType: "GetItem",
		Table:         "main",
		Period:        PeriodHour,
	}

	err := ct.UpdateKeys()
	assert.NoError(t, err)

	assert.NotEmpty(t, ct.ID)
	assert.False(t, ct.Timestamp.IsZero())
	assert.Equal(t, "cost#GetItem", ct.PK)
	assert.Contains(t, ct.SK, "ts#")
	assert.Equal(t, "COST_TABLE#main", ct.GSI1PK)
	assert.Contains(t, ct.GSI1SK, "GetItem")
	assert.Equal(t, "COST_AGG#hour#GetItem", ct.GSI2PK)
	assert.NotEmpty(t, ct.GSI2SK)
}

func TestDynamoDBCostRecord_BeforeCreate_AndBeforeUpdate_SetTimestampsTTLAndValidate(t *testing.T) {
	ct := &DynamoDBCostRecord{
		OperationType:       "PutItem",
		Table:               "main",
		Period:              PeriodDay,
		TotalCostMicroCents: 2_000_000,
	}

	before := time.Now()
	err := ct.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.NotZero(t, ct.CreatedAt)
	assert.NotZero(t, ct.UpdatedAt)
	assert.NotEmpty(t, ct.ID)
	assert.False(t, ct.Timestamp.IsZero())
	assert.InDelta(t, 2.0, ct.EstimatedCostDollars, 0.00001)
	assert.True(t, ct.ExpiresAt > 0)

	ttl := time.Unix(ct.ExpiresAt, 0)
	assert.True(t, ttl.After(before.Add(90*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(90*24*time.Hour+5*time.Second)))

	// BeforeUpdate recalculates estimated cost and refreshes UpdatedAt.
	previousUpdatedAt := ct.UpdatedAt
	ct.TotalCostMicroCents = 3_000_000
	err = ct.BeforeUpdate()
	assert.NoError(t, err)
	assert.False(t, ct.UpdatedAt.Before(previousUpdatedAt))
	assert.InDelta(t, 3.0, ct.EstimatedCostDollars, 0.00001)
}

func TestDynamoDBCostRecord_Validate_InvalidOperationAndPeriod(t *testing.T) {
	ct := &DynamoDBCostRecord{
		ID:            "id",
		OperationType: "Nope",
		Table:         "main",
		Period:        "noperiod",
		Timestamp:     time.Now(),
	}
	assert.Error(t, ct.Validate())

	ct.OperationType = "GetItem"
	assert.Error(t, ct.Validate())
}

func TestDynamoDBCostAggregation_Lifecycle_AndValidation(t *testing.T) {
	act := &DynamoDBCostAggregation{
		Period:            PeriodMonth,
		OperationType:     "Query",
		Table:             "main",
		WindowStart:       time.Unix(1700000000, 0).UTC(),
		WindowEnd:         time.Unix(1700003600, 0).UTC(),
		TotalCostMicroCents: 1_500_000,
		TotalOperations:     3,
	}

	before := time.Now()
	err := act.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "cost_agg#month#Query", act.PK)
	assert.Contains(t, act.SK, "window#")
	assert.InDelta(t, 1.5, act.TotalCostDollars, 0.00001)
	assert.InDelta(t, 0.5, act.AverageCostPerOperation, 0.00001)

	ttl := time.Unix(act.ExpiresAt, 0)
	assert.True(t, ttl.After(before.Add(365*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(365*24*time.Hour+5*time.Second)))

	prevUpdated := act.UpdatedAt
	act.TotalCostMicroCents = 3_000_000
	err = act.BeforeUpdate()
	assert.NoError(t, err)
	assert.False(t, act.UpdatedAt.Before(prevUpdated))
	assert.InDelta(t, 3.0, act.TotalCostDollars, 0.00001)
	assert.InDelta(t, 1.0, act.AverageCostPerOperation, 0.00001)

	// Validation errors.
	act2 := &DynamoDBCostAggregation{
		Period:        PeriodHour,
		OperationType: "Query",
		WindowStart:   time.Now(),
		WindowEnd:     time.Time{},
	}
	assert.Error(t, act2.Validate())
}

func TestDynamoDBCostRecordBuilder_AndCalculateCost(t *testing.T) {
	builder := NewDynamoDBCostRecordBuilder().
		ForOperation("GetItem").
		OnTable("main").
		WithCapacityUnits(1000, 2000).
		WithCostMicroCents(10, 20).
		WithItemCount(5).
		WithDuration(12).
		WithService("svc", "fn").
		WithRequestID("rid").
		WithIndex("gsi1").
		WithConsistentRead(true).
		WithTag("k", "v").
		WithPeriod(PeriodHour)

	tracking := builder.Build()
	assert.Equal(t, "GetItem", tracking.OperationType)
	assert.Equal(t, "main", tracking.Table)
	assert.Equal(t, float64(1000), tracking.ReadCapacityUnits)
	assert.Equal(t, float64(2000), tracking.WriteCapacityUnits)
	assert.Equal(t, int64(30), tracking.TotalCostMicroCents)
	assert.Equal(t, "v", tracking.Tags["k"])

	read, write, total := CalculateCost(1000, 1000)
	assert.Equal(t, int64(ReadCostMicroCentsPerUnit), read)
	assert.Equal(t, int64(WriteCostMicroCentsPerUnit), write)
	assert.Equal(t, read+write, total)
}
