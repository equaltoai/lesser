package models

import (
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestMediaSpending_BeforeCreate_PeriodTimes_Keys_AndValidation(t *testing.T) {
	ms := &MediaSpending{
		UserID:     "user-12345678",
		Username:   "alice",
		PeriodType: PeriodMonthly,
		Period:     "2024-01",

		ProcessingSpendMicros: 1,
		StorageSpendMicros:    2,
		BandwidthSpendMicros:  3,
		ComputeSpendMicros:    4,
		TotalSpendMicros:      999, // should be auto-corrected by Validate
	}

	before := time.Now()
	err := ms.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "MEDIA_SPENDING#user-12345678", ms.PK)
	assert.Equal(t, "PERIOD#2024-01", ms.SK)
	assert.Equal(t, "SPENDING#monthly", ms.GSI1PK)
	assert.Equal(t, "2024-01#user-12345678", ms.GSI1SK)
	assert.Equal(t, "COST_CATEGORY#"+ResourceCompute, ms.GSI2PK)
	assert.Contains(t, ms.GSI2SK, "#user-12345678")

	start, parseErr := time.Parse(common.MonthFormat, "2024-01")
	assert.NoError(t, parseErr)
	assert.Equal(t, start, ms.PeriodStartAt)
	assert.Equal(t, start.AddDate(0, 1, 0).Add(-time.Nanosecond), ms.PeriodEndAt)

	assert.Equal(t, int64(10), ms.TotalSpendMicros)
	assert.WithinDuration(t, time.Now(), ms.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, ms.CreatedAt, ms.UpdatedAt, 2*time.Second)
	if assert.NotNil(t, ms.ExpiresAt) {
		ttl := time.Unix(*ms.ExpiresAt, 0)
		assert.True(t, ttl.After(before.Add(2*365*24*time.Hour-5*time.Second)))
		assert.True(t, ttl.Before(after.Add(2*365*24*time.Hour+5*time.Second)))
	}

	// UpdateKeys validations.
	ms2 := &MediaSpending{}
	assert.Error(t, ms2.UpdateKeys())
}

func TestMediaSpending_setPeriodTimes_Errors(t *testing.T) {
	ms := &MediaSpending{UserID: "user-12345678", PeriodType: PeriodMonthly, Period: "bad"}
	err := ms.setPeriodTimes()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidMonthlyPeriodFormat))

	ms = &MediaSpending{UserID: "user-12345678", PeriodType: PeriodDaily, Period: "bad"}
	err = ms.setPeriodTimes()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDailyPeriodFormat))

	ms = &MediaSpending{UserID: "user-12345678", PeriodType: "nope", Period: "2024-01"}
	err = ms.setPeriodTimes()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidPeriodType))
}

func TestMediaSpending_BeforeUpdate_BudgetTracking(t *testing.T) {
	ms := &MediaSpending{
		UserID:            "user-12345678",
		PeriodType:        PeriodDaily,
		Period:            "2024-01-15",
		BudgetLimitMicros: 100,
		TotalSpendMicros:  150,
		CreatedAt:         time.Unix(1700000000, 0).UTC(),
	}

	err := ms.BeforeUpdate()
	assert.NoError(t, err)
	assert.True(t, ms.BudgetExceeded)
	assert.Greater(t, ms.BudgetUsagePercent, 100.0)
	assert.NotNil(t, ms.BudgetExceededAt)
}

func TestMediaSpendingTransaction_BeforeCreate_Validate_UpdateKeys(t *testing.T) {
	mst := &MediaSpendingTransaction{
		UserID:     "user-12345678",
		Category:   ResourceProcessing,
		CostMicros: 10,
		Service:    "s3",
		Operation:  "storage_put",
	}

	err := mst.BeforeCreate()
	assert.NoError(t, err)
	assert.NotEmpty(t, mst.TransactionID)
	assert.Contains(t, mst.TransactionID, "user-123")

	assert.Equal(t, "SPENDING_TXN#user-12345678", mst.PK)
	assert.Contains(t, mst.SK, "TXN#")
	assert.NotEmpty(t, mst.GSI1PK)
	assert.NotEmpty(t, mst.GSI1SK)
	assert.NotNil(t, mst.ExpiresAt)

	// Validate errors.
	mst2 := &MediaSpendingTransaction{
		UserID:        "user-12345678",
		TransactionID: "t1",
		Category:      "nope",
		CostMicros:    10,
	}
	assert.Error(t, mst2.Validate())

	mst3 := &MediaSpendingTransaction{
		UserID:        "user-12345678",
		TransactionID: "t1",
		Category:      ResourceProcessing,
		CostMicros:    -1,
	}
	assert.Error(t, mst3.Validate())

	// UpdateKeys with fixed CreatedAt.
	fixed := time.Unix(1700000000, 0).UTC()
	mst4 := &MediaSpendingTransaction{
		UserID:        "user-12345678",
		TransactionID: "t1",
		CreatedAt:     fixed,
	}
	assert.NoError(t, mst4.UpdateKeys())
	assert.Equal(t, "SPENDING_TXN#user-12345678", mst4.PK)
	assert.Contains(t, mst4.SK, "TXN#")
	assert.Equal(t, "TXN_TIME#"+fixed.Format(common.DateFormat), mst4.GSI1PK)
}

func TestMediaSpending_AddSpending_Breakdowns_Efficiency_AndBudgetHelpers(t *testing.T) {
	ms := &MediaSpending{
		UserID:            "user-12345678",
		PeriodType:        PeriodDaily,
		Period:            "2024-01-15",
		BudgetLimitMicros: 100,
	}

	tx1 := &MediaSpendingTransaction{
		UserID:        "user-12345678",
		TransactionID: "t1",
		Category:      ResourceProcessing,
		Service:       "s3",
		Operation:     "image_resize",
		CostMicros:    10,
		MediaID:       "m1",
		FileSize:      1024 * 1024,
	}
	ms.AddSpending(tx1)

	tx2 := &MediaSpendingTransaction{
		UserID:         "user-12345678",
		TransactionID:  "t2",
		Category:       "bandwidth",
		Service:        "cloudfront",
		Operation:      "deliver",
		CostMicros:     20,
		BytesProcessed: 2048,
		IsError:        true,
	}
	ms.AddSpending(tx2)

	tx3 := &MediaSpendingTransaction{
		UserID:           "user-12345678",
		TransactionID:    "t3",
		Category:         "compute",
		Service:          ResourceLambda,
		Operation:        "invoke",
		CostMicros:       30,
		ProcessingTimeMs: 50,
	}
	ms.AddSpending(tx3)

	assert.Equal(t, int64(3), ms.TotalOperations)
	assert.Equal(t, int64(1), ms.ImageProcessingOps)
	assert.Equal(t, int64(1), ms.FilesProcessed)
	assert.Equal(t, int64(1024*1024), ms.BytesProcessed)
	assert.Equal(t, int64(1), ms.FailedOperations)
	assert.Equal(t, int64(20), ms.ErrorCostMicros)

	costs := ms.GetCostBreakdown()
	assert.Equal(t, int64(10), costs[ResourceProcessing])
	assert.Equal(t, int64(20), costs["bandwidth"])
	assert.Equal(t, int64(30), costs["compute"])

	services := ms.GetServiceBreakdown()
	assert.Equal(t, int64(10), services["s3_requests"])
	assert.Equal(t, int64(20), services["cloudfront"])
	assert.Equal(t, int64(30), services["lambda"])

	metrics := ms.GetEfficiencyMetrics()
	assert.NotZero(t, metrics["cost_per_file"])
	assert.NotZero(t, metrics["cost_per_mb"])
	assert.NotZero(t, metrics["cost_per_operation"])
	assert.NotZero(t, metrics["failure_rate"])

	assert.False(t, ms.IsOverBudget())
	assert.Equal(t, int64(60), ms.TotalSpendMicros)
	assert.Equal(t, int64(40), ms.GetRemainingBudget())

	ms.TotalSpendMicros = 1000
	assert.True(t, ms.IsOverBudget())
	assert.Equal(t, int64(0), ms.GetRemainingBudget())
}
