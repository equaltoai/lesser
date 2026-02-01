package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestImportBudget_Lifecycle_Usage_Limits_Alerts_AndReset(t *testing.T) {
	before := time.Now()
	b := &ImportBudget{
		Username: "alice",
		Period:   PeriodDaily,

		ImportLimitMicroCents:   100,
		ExportLimitMicroCents:   200,
		CombinedLimitMicroCents: 300,
		IsActive:                true,

		CurrentImportCost:   90,
		CurrentExportCost:   10,
		CurrentCombinedCost: 100,
		AlertSendingEnabled: true,
	}
	err := b.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "USER_BUDGET#alice#"+PeriodDaily, b.PK)
	assert.Equal(t, "CONFIG", b.SK)
	assert.Equal(t, "BUDGET#"+PeriodDaily, b.GSI1PK)
	assert.Equal(t, "USER#alice", b.GSI1SK)
	assert.Equal(t, 80.0, b.AlertThresholdPercent)
	assert.True(t, b.TTL > 0)

	ttl := time.Unix(b.TTL, 0)
	assert.True(t, ttl.After(before.AddDate(0, 0, 7).Add(-5*time.Second)))
	assert.True(t, ttl.Before(after.AddDate(0, 0, 7).Add(5*time.Second)))

	assert.InDelta(t, 90.0, b.GetImportUsagePercent(), 0.00001)
	assert.InDelta(t, 5.0, b.GetExportUsagePercent(), 0.00001)
	assert.InDelta(t, 33.3333, b.GetCombinedUsagePercent(), 0.01)

	assert.True(t, b.IsImportOverLimit(20))
	assert.False(t, b.IsImportOverLimit(0))
	assert.False(t, b.IsExportOverLimit(0))
	assert.False(t, b.IsCombinedOverLimit(0, 0))

	// Alert threshold trips on import usage (90% >= 80%).
	assert.True(t, b.ShouldSendAlert())
	sent := time.Now()
	b.LastAlertSent = &sent
	assert.False(t, b.ShouldSendAlert())

	// NeedsReset uses NextResetAt.
	b.NextResetAt = time.Now().Add(-time.Minute)
	assert.True(t, b.NeedsReset())

	// Reset zeros usage and updates period boundaries.
	b.Reset()
	assert.Equal(t, int64(0), b.CurrentImportCost)
	assert.Equal(t, int64(0), b.CurrentExportCost)
	assert.Equal(t, int64(0), b.CurrentCombinedCost)
	assert.Equal(t, int64(0), b.ImportCount)
	assert.Equal(t, int64(0), b.ExportCount)
	assert.NotNil(t, b.LastResetAt)
	assert.False(t, b.PeriodStart.IsZero())
	assert.True(t, b.PeriodEnd.After(b.PeriodStart))
	assert.True(t, b.NextResetAt.Equal(b.PeriodEnd))

	// Remaining budget helpers.
	b.ImportLimitMicroCents = 0
	assert.Equal(t, int64(-1), b.GetRemainingImportBudget())
	b.ImportLimitMicroCents = 100
	b.CurrentImportCost = 110
	assert.Equal(t, int64(0), b.GetRemainingImportBudget())
}
