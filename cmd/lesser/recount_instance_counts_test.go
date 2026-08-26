package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
)

// TestRunRecountInstanceCounts_EndToEnd drives the recount CLI end-to-end with
// a fakedb-backed database: dry-run reports without writing, --apply rewrites
// the counters, rebuilds the active-month rollup, and persists the
// SEED#ACTIVE_MONTH marker.
func TestRunRecountInstanceCounts_EndToEnd(t *testing.T) {
	prevTable := models.MainTableName
	models.MainTableName = "lesser-test-recount"
	t.Cleanup(func() { models.MainTableName = prevTable })

	ctx := context.Background()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	for _, m := range []any{&models.User{}, &models.Actor{}, &models.DomainCounter{}, &models.InstanceMetrics{}, &models.Activity{}, &models.ActivityDayCounter{}, &models.ActivityActorDay{}} {
		require.NoError(t, db.CreateTable(m))
	}

	for i := 0; i < 2; i++ {
		u := &models.User{Username: "user-" + string(rune('a'+i)), Email: "u@example.com", Role: "user"}
		require.NoError(t, u.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(u).Create())
	}
	for i := 0; i < 3; i++ {
		domain := "a.example.com"
		if i == 2 {
			domain = "b.example.com"
		}
		a := &models.Actor{Username: "actor-" + string(rune('a'+i)), Actor: &activitypub.Actor{
			PreferredUsername: "actor-" + string(rune('a'+i)),
			BaseObject:        activitypub.BaseObject{ID: "https://" + domain + "/users/actor-" + string(rune('a'+i))},
		}}
		require.NoError(t, a.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(a).Create())
	}
	stale := &models.DomainCounter{Domain: "stale.example.com", Value: 3}
	require.NoError(t, stale.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(stale).Create())

	// Two distinct actors active today, one yesterday.
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		published := now
		if i == 2 {
			published = now.AddDate(0, 0, -1)
		}
		act := &models.Activity{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: fmt.Sprintf("act-%d", i), Type: "Create", Published: &published},
				Actor:      "https://example.com/users/actor-" + string(rune('a'+i)),
				Object:     map[string]any{"content": "hello"},
			},
			CreatedAt: published,
		}
		require.NoError(t, act.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(act).Create())
	}

	prevLoad := loadAWSConfigForCLIFn
	prevOpen := openRecountDBFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = prevLoad
		openRecountDBFn = prevOpen
	})
	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "test", nil
	}
	openRecountDBFn = func(aws.Config) (core.DB, func() error, error) {
		return db, nil, nil
	}

	// Dry-run: reports, writes nothing.
	require.NoError(t, runRecountInstanceCountsFn([]string{"--table", "lesser-test-recount"}))
	var metric models.InstanceMetrics
	err = db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalUsersMetricSK).
		First(&metric)
	require.Error(t, err) // counter not written in dry-run

	// Apply: rewrites the counters, drops the stale domain counter, rebuilds
	// the active-month rollup, and persists the SEED#ACTIVE_MONTH marker.
	require.NoError(t, runRecountInstanceCountsFn([]string{"--table", "lesser-test-recount", "--apply"}))
	metric = models.InstanceMetrics{}
	require.NoError(t, db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalUsersMetricSK).
		First(&metric))
	require.EqualValues(t, 2, metric.TotalUsers)

	metric = models.InstanceMetrics{}
	require.NoError(t, db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.TotalDomainsMetricSK).
		First(&metric))
	require.EqualValues(t, 2, metric.Value)

	metric = models.InstanceMetrics{}
	require.NoError(t, db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", models.InstanceMetricsPK).
		Where("SK", "=", models.ActiveMonthSeedMetricSK).
		First(&metric), "SEED#ACTIVE_MONTH marker must be persisted by --apply")

	today := models.DayFormat(now)
	var dayCounter models.ActivityDayCounter
	require.NoError(t, db.WithContext(ctx).Model(&models.ActivityDayCounter{}).
		Where("PK", "=", models.ActivityDayKey(today)).
		Where("SK", "=", models.DayCounterSK).
		First(&dayCounter))
	require.EqualValues(t, 2, dayCounter.Value)
}

func TestRunRecountInstanceCounts_FlagAndDBOpenErrors(t *testing.T) {
	t.Run("unknown flag errors", func(t *testing.T) {
		err := runRecountInstanceCountsFn([]string{"--bogus"})
		require.Error(t, err)
	})

	t.Run("db open failure propagates", func(t *testing.T) {
		prevLoad := loadAWSConfigForCLIFn
		prevOpen := openRecountDBFn
		t.Cleanup(func() {
			loadAWSConfigForCLIFn = prevLoad
			openRecountDBFn = prevOpen
		})
		loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
			return aws.Config{Region: "us-east-1"}, "test", nil
		}
		openRecountDBFn = func(aws.Config) (core.DB, func() error, error) {
			return nil, nil, errors.New("db open failed")
		}
		err := runRecountInstanceCountsFn([]string{"--table", "lesser-dev"})
		require.ErrorContains(t, err, "db open failed")
	})
}

func TestPrintRecountInstanceCountsSummary(t *testing.T) {
	output := captureStdout(t, func() {
		printRecountInstanceCountsSummary(recountInstanceCountsSummary{
			Users:                3,
			Domains:              2,
			DomainCounters:       2,
			StaleDomainCounters:  1,
			ActiveMonthDays:      4,
			StaleActiveMonthDays: 1,
			ActiveMonthSum:       77,
		}, "lesser-dev", "Theory", false)
	})

	require.Contains(t, output, "recount-instance-counts dry-run complete")
	require.Contains(t, output, "table:        lesser-dev")
	require.Contains(t, output, "aws_profile:  Theory")
	require.Contains(t, output, "total_users:    3")
	require.Contains(t, output, "total_domains:  2")
	require.Contains(t, output, "domain counters upserted: 2")
	require.Contains(t, output, "stale domain counters removed: 1")
	require.Contains(t, output, "active_month days upserted: 4")
	require.Contains(t, output, "stale active_month days removed: 1")
	require.Contains(t, output, "active_month sum (retention window): 77")
	require.Contains(t, output, "dry-run: pass --apply to rewrite the counters")

	output = captureStdout(t, func() {
		printRecountInstanceCountsSummary(recountInstanceCountsSummary{
			Users: 4, Domains: 3, DomainCounters: 3, ActiveMonthDays: 5,
			ActiveMonthSum: 10, ActiveMonthSeedMarker: true,
		}, "lesser-dev", "", true)
	})
	require.Contains(t, output, "recount-instance-counts apply complete")
	require.Contains(t, output, "SEED#ACTIVE_MONTH marker: written")
	require.NotContains(t, output, "dry-run: pass --apply")
}
