package repositories

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// This file benchmarks the public instance-count reads (issue #1467) against
// a harness-scale in-memory dataset: the legacy scan-count implementations
// (every item body fetched, unique-counted in Go) versus the O(1) counter
// reads (point reads of maintained counters). Absolute in-memory numbers are
// tiny; the ratios show the mechanics, and the field baseline is the 5.7s
// warm latency recorded in the issue for a 2.2KB /api/v2/instance response.

const (
	benchActivities = 20000
	benchUsers      = 10000
	benchActors     = 5000
)

func benchmarkCountsDB(b *testing.B) (core.DB, *TrendingRepository) {
	b.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	if err != nil {
		b.Fatal(err)
	}
	for _, m := range []any{
		&models.Activity{}, &models.User{}, &models.Actor{},
		&models.ActivityDayCounter{}, &models.ActivityActorDay{},
		&models.DomainCounter{}, &models.InstanceMetrics{},
	} {
		if err := db.CreateTable(m); err != nil {
			b.Fatal(err)
		}
	}

	ctx := context.Background()
	now := time.Now().UTC()

	b.StopTimer()
	for i := 0; i < benchActivities; i++ {
		username := fmt.Sprintf("user-%d", i)
		published := now.Add(time.Duration(i%240) * time.Hour)
		a := &models.Activity{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: fmt.Sprintf("activity-%d", i), Type: "Create", Published: &published},
				Actor:      "https://example.com/users/" + username,
				Object:     map[string]any{"content": "hello"},
			},
			CreatedAt: published,
		}
		if err := a.UpdateKeys(); err != nil {
			b.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(a).Create(); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < benchUsers; i++ {
		username := fmt.Sprintf("user-%d", i)
		u := &models.User{Username: username, Email: username + "@example.com", Role: "user"}
		if err := u.UpdateKeys(); err != nil {
			b.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(u).Create(); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < benchActors; i++ {
		username := fmt.Sprintf("actor-%d", i)
		domain := "a.example.com"
		if i%2 == 0 {
			domain = "b.example.com"
		}
		actor := &models.Actor{
			Username:  username,
			NumericID: fmt.Sprintf("num-%d", i),
			Actor: &activitypub.Actor{
				PreferredUsername: username,
				BaseObject:        activitypub.BaseObject{ID: "https://" + domain + "/users/" + username},
			},
		}
		if err := actor.UpdateKeys(); err != nil {
			b.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(actor).Create(); err != nil {
			b.Fatal(err)
		}
	}
	b.StartTimer()

	return db, NewTrendingRepository(db, zap.NewNop(), nil)
}

// BenchmarkLegacyActiveUserScan replicates the pre-fix GetActiveUserCount
// (scan every Activity body in the window, unique-count actors in Go).
func BenchmarkLegacyActiveUserScan(b *testing.B) {
	ctx := context.Background()
	db, _ := benchmarkCountsDB(b)
	cutoff := time.Now().AddDate(0, 0, -30)

	b.ResetTimer()
	for b.Loop() {
		var activities []models.Activity
		if err := db.WithContext(ctx).Model(&models.Activity{}).
			Filter("PublishedAt", ">", cutoff).
			All(&activities); err != nil {
			b.Fatal(err)
		}
		unique := make(map[string]bool, len(activities))
		for _, a := range activities {
			if a.Activity != nil && a.Activity.Actor != "" {
				unique[a.Activity.Actor] = true
			}
		}
		_ = len(unique)
	}
}

// BenchmarkCounterActiveUserRead is the post-fix GetActiveUserCount over the
// same dataset (30 point reads of day counters).
func BenchmarkCounterActiveUserRead(b *testing.B) {
	ctx := context.Background()
	db, repo := benchmarkCountsDB(b)

	// Seed the rollup the way the write path would have after a warm period.
	now := time.Now().UTC()
	b.StopTimer()
	for i := 0; i < 30; i++ {
		day := models.DayFormat(now.AddDate(0, 0, -i))
		counter := &models.ActivityDayCounter{Date: day, Value: 1000, UpdatedAt: now}
		if err := counter.UpdateKeys(); err != nil {
			b.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(counter).Create(); err != nil {
			b.Fatal(err)
		}
	}
	marker := &models.InstanceMetrics{PK: models.InstanceMetricsPK, SK: models.ActiveMonthSeedMetricSK, UpdatedAt: now}
	if err := db.WithContext(ctx).Model(marker).Create(); err != nil {
		b.Fatal(err)
	}
	b.StartTimer()

	b.ResetTimer()
	for b.Loop() {
		if _, err := repo.GetActiveUserCount(ctx, 30); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLegacyTotalUserScan replicates the pre-fix GetTotalUserCount.
func BenchmarkLegacyTotalUserScan(b *testing.B) {
	ctx := context.Background()
	db, _ := benchmarkCountsDB(b)

	b.ResetTimer()
	for b.Loop() {
		var users []models.User
		if err := db.WithContext(ctx).Model(&models.User{}).All(&users); err != nil {
			b.Fatal(err)
		}
		_ = len(users)
	}
}

// BenchmarkCounterTotalUserRead is the post-fix GetTotalUserCount.
func BenchmarkCounterTotalUserRead(b *testing.B) {
	ctx := context.Background()
	db, repo := benchmarkCountsDB(b)

	b.StopTimer()
	metric := &models.InstanceMetrics{PK: models.InstanceMetricsPK, SK: models.TotalUsersMetricSK, TotalUsers: benchUsers, UpdatedAt: time.Now()}
	if err := db.WithContext(ctx).Model(metric).Create(); err != nil {
		b.Fatal(err)
	}
	b.StartTimer()

	b.ResetTimer()
	for b.Loop() {
		if _, err := repo.GetTotalUserCount(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLegacyDomainScan replicates the pre-fix GetTotalDomainCount.
func BenchmarkLegacyDomainScan(b *testing.B) {
	ctx := context.Background()
	db, _ := benchmarkCountsDB(b)

	b.ResetTimer()
	for b.Loop() {
		var actors []models.Actor
		if err := db.WithContext(ctx).Model(&models.Actor{}).All(&actors); err != nil {
			b.Fatal(err)
		}
		unique := make(map[string]bool, 8)
		for _, a := range actors {
			if a.Actor != nil && a.Actor.ID != "" {
				if u, err := url.Parse(a.Actor.ID); err == nil && u.Host != "" {
					unique[u.Host] = true
				}
			}
		}
		_ = len(unique)
	}
}

// BenchmarkCounterDomainRead is the post-fix GetTotalDomainCount.
func BenchmarkCounterDomainRead(b *testing.B) {
	ctx := context.Background()
	db, repo := benchmarkCountsDB(b)

	b.StopTimer()
	metric := &models.InstanceMetrics{PK: models.InstanceMetricsPK, SK: models.TotalDomainsMetricSK, Value: 2, UpdatedAt: time.Now()}
	if err := db.WithContext(ctx).Model(metric).Create(); err != nil {
		b.Fatal(err)
	}
	b.StartTimer()

	b.ResetTimer()
	for b.Loop() {
		if _, err := repo.GetTotalDomainCount(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
