package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestActivityDayCounter_Methods(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	c := &ActivityDayCounter{Date: "2026-08-25", Value: 3, UpdatedAt: now}

	require.Equal(t, MainTableName, c.TableName())
	require.NoError(t, c.UpdateKeys())
	require.Equal(t, "ACTIVITY_DAY#2026-08-25", c.PK)
	require.Equal(t, DayCounterSK, c.SK)
	require.Equal(t, c.PK, c.GetPK())
	require.Equal(t, c.SK, c.GetSK())
	// TTL is retention (200 days) past the update time.
	require.Equal(t, now.Add(activeDayRetentionDays*24*time.Hour).Unix(), c.TTL)

	// Zero UpdatedAt defaults to now.
	c2 := &ActivityDayCounter{Date: "2026-08-25"}
	require.NoError(t, c2.UpdateKeys())
	require.False(t, c2.UpdatedAt.IsZero())
	require.Equal(t, c2.UpdatedAt.Add(activeDayRetentionDays*24*time.Hour).Unix(), c2.TTL)
}

func TestActivityActorDay_Methods(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	m := &ActivityActorDay{ActorID: "https://example.com/users/a", Day: "2026-08-25", UpdatedAt: now}

	require.Equal(t, MainTableName, m.TableName())
	require.NoError(t, m.UpdateKeys())
	require.Equal(t, "ACTIVITY_ACTOR#https://example.com/users/a", m.PK)
	require.Equal(t, "DAY#2026-08-25", m.SK)
	require.Equal(t, m.PK, m.GetPK())
	require.Equal(t, m.SK, m.GetSK())
	require.Equal(t, now.Add(activeDayRetentionDays*24*time.Hour).Unix(), m.TTL)
}

func TestDomainCounter_Methods(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	d := &DomainCounter{Domain: "example.com", Value: 2, UpdatedAt: now}

	require.Equal(t, MainTableName, d.TableName())
	require.NoError(t, d.UpdateKeys())
	require.Equal(t, "DOMAIN#example.com", d.PK)
	require.Equal(t, DayCounterSK, d.SK)
	require.Equal(t, d.PK, d.GetPK())
	require.Equal(t, d.SK, d.GetSK())
}

func TestInstanceCountKeys(t *testing.T) {
	utc := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	require.Equal(t, "2026-08-25", DayFormat(utc))

	// DayFormat converts to UTC regardless of the input location.
	local := time.Date(2026, time.August, 25, 23, 30, 0, 0, time.FixedZone("x", 3600))
	require.Equal(t, "2026-08-25", DayFormat(local.Add(-2*time.Hour)))

	require.Equal(t, "ACTIVITY_DAY#2026-08-25", ActivityDayKey("2026-08-25"))
	require.Equal(t, "COUNTER", DayCounterSK)
	require.Equal(t, "INSTANCE#METRICS", InstanceMetricsPK)
	require.Equal(t, "TOTAL_USERS", TotalUsersMetricSK)
	require.Equal(t, "TOTAL_DOMAINS", TotalDomainsMetricSK)
	require.Equal(t, "TOTAL_STATUSES", TotalStatusesMetricSK)
	require.Equal(t, "SEED#ACTIVE_MONTH", ActiveMonthSeedMetricSK)
}
