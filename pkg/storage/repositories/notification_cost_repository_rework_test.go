package repositories

// Issue #1506 (folded into #1505 rework) — GetDailySpending built its gsi1SK
// window as "COST#" + RFC3339 while the model writer emits "COST#" +
// CompactTimeFormat (pkg/common/time_formats.go, NotificationCostTracking.
// UpdateKeys). The RFC3339 bound sorted ABOVE every written row, so the query
// matched zero rows for every writer — always. These tests pin the fixed
// (writer-aligned) bounds against a REAL row set.

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// TestBatch1506_GetDailySpending_RowSet seeds a real NotificationCostTracking
// row written "today" through the model's own UpdateKeys (gsi1SK =
// COST#YYYYMMDDHHMMSS) and asserts GetDailySpending returns its cost.
//
// Mutation kill: restore the RFC3339 bound format in GetDailySpending — the
// compact-format row sorts ABOVE the RFC3339 upper bound ("COST#2026-08-27..."
// < "COST#20260827..." at byte 5), the window matches zero rows, and this
// test goes RED.
func TestBatch1506_GetDailySpending_RowSet(t *testing.T) {
	ctx := context.Background()
	client := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.NotificationCostTracking{}))

	repo := NewNotificationCostRepository(db, "test-table", zap.NewNop(), nil)

	now := time.Now().UTC()
	tracking := &models.NotificationCostTracking{
		ID:                  "row-1",
		NotificationID:      "notif-1",
		Username:            "alice",
		UserID:              "uid-1",
		DeliveryMethod:      "push",
		NotificationType:    "mention",
		Timestamp:           now,
		TotalCostMicroCents: 123456,
	}
	require.NoError(t, tracking.BeforeCreate(), "writer key hooks must produce the stored row")
	require.NoError(t, db.Model(tracking).Create())

	// The writer emits the compact time format; the row must land inside the
	// day window the read will query.
	require.Equal(t, "COST#"+now.Format(common.CompactTimeFormat), tracking.GSI1SK,
		"writer key shape must be COST# + CompactTimeFormat")

	total, err := repo.GetDailySpending(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(123456), total,
		"a row written inside today's window must be returned (issue #1506)")
}

// TestBatch1506_GetDailySpending_ExcludesYesterdayAndTomorrow pins the window
// edges: a row from yesterday's date (compact format) sorts below today's
// startSK and must NOT be counted; a row from tomorrow sorts at/above endSK
// and must NOT be counted. Guards against the bound fix over-widening the
// window.
func TestBatch1506_GetDailySpending_ExcludesOutsideWindow(t *testing.T) {
	ctx := context.Background()
	client := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.NotificationCostTracking{}))

	repo := NewNotificationCostRepository(db, "test-table", zap.NewNop(), nil)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	outside := []models.NotificationCostTracking{
		{
			ID: "row-yesterday", NotificationID: "notif-y", Username: "alice",
			DeliveryMethod: "push", Timestamp: today.Add(-time.Second), TotalCostMicroCents: 111,
		},
		{
			ID: "row-today", NotificationID: "notif-t", Username: "alice",
			DeliveryMethod: "push", Timestamp: today.Add(12 * time.Hour), TotalCostMicroCents: 222,
		},
		{
			// One second past tomorrow midnight: strictly ABOVE endSK (a row at
			// exactly tomorrow 00:00:00 == endSK is the inclusive-bound edge the
			// production comment declares unwritable).
			ID: "row-tomorrow", NotificationID: "notif-m", Username: "alice",
			DeliveryMethod: "push", Timestamp: today.Add(24*time.Hour + time.Second), TotalCostMicroCents: 333,
		},
	}
	for i := range outside {
		require.NoError(t, outside[i].BeforeCreate())
		require.NoError(t, db.Model(&outside[i]).Create())
	}

	total, err := repo.GetDailySpending(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(222), total,
		"only the row strictly inside today's window counts (issue #1506)")
}
