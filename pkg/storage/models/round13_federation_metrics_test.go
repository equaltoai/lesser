package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFederationAnalyticsTimeSeries_TimeBucket_AndWindowEnd(t *testing.T) {
	ts := time.Date(2024, 1, 2, 3, 4, 59, 123, time.UTC)

	assert.Equal(t, ts.Truncate(time.Minute), GetTimeBucket(ts, PeriodRaw))
	assert.Equal(t, ts.Truncate(5*time.Minute), GetTimeBucket(ts, Period5Min))
	assert.Equal(t, ts.Truncate(time.Hour), GetTimeBucket(ts, PeriodHourly))

	daily := GetTimeBucket(ts, PeriodDaily)
	assert.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), daily)

	monthly := GetTimeBucket(ts, PeriodMonthly)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), monthly)

	// getWindowEnd helpers (indirectly exercised by NewFederationAnalyticsTimeSeries as well).
	assert.Equal(t, daily.AddDate(0, 0, 1), getWindowEnd(daily, PeriodDaily))
	assert.Equal(t, monthly.AddDate(0, 1, 0), getWindowEnd(monthly, PeriodMonthly))
}

func TestFederationAnalyticsTimeSeries_New_UpdateKeys_HealthAndAlerts(t *testing.T) {
	before := time.Now()
	fs := NewFederationAnalyticsTimeSeries("example.com", Period5Min, time.Now())
	after := time.Now()

	assert.Equal(t, "example.com", fs.Domain)
	assert.Equal(t, Period5Min, fs.Period)
	assert.Contains(t, fs.PK, "FEDERATION_TIMESERIES#example.com#5min")
	assert.NotEmpty(t, fs.GSI1PK)
	assert.NotEmpty(t, fs.GSI2PK)
	assert.True(t, fs.TTL > 0)

	ttl := time.Unix(fs.TTL, 0)
	assert.True(t, ttl.After(before.Add(24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(24*time.Hour+5*time.Second)))

	// Health score branches.
	fs.InstanceReachability = 1.0
	fs.ErrorRate = 0.0
	fs.InboxDeliveryP95 = 1500
	contact := time.Now().Add(-30 * time.Minute)
	fs.LastSuccessfulContact = &contact
	fs.CalculateHealthScore()
	assert.GreaterOrEqual(t, fs.HealthScore, 80.0)
	assert.True(t, fs.IsHealthy())

	fs.HealthScore = 70
	assert.True(t, fs.IsDegraded())
	assert.False(t, fs.IsHealthy())

	fs.HealthScore = 50
	assert.True(t, fs.IsUnhealthy())
	assert.False(t, fs.IsCritical())

	fs.HealthScore = 10
	assert.True(t, fs.IsCritical())

	// Alert conditions.
	fs.InstanceReachability = 0.4
	ok, msg := fs.ShouldTriggerAlert()
	assert.True(t, ok)
	assert.Contains(t, msg, "reachability")

	fs.InstanceReachability = 1.0
	fs.SignatureFailures = 101
	ok, msg = fs.ShouldTriggerAlert()
	assert.True(t, ok)
	assert.Contains(t, msg, "Signature failures")

	fs.SignatureFailures = 0
	fs.InboxDeliveryP95 = 6000
	ok, msg = fs.ShouldTriggerAlert()
	assert.True(t, ok)
	assert.Contains(t, msg, "latency")

	fs.InboxDeliveryP95 = 0
	fs.QueueDepth = 20000
	ok, msg = fs.ShouldTriggerAlert()
	assert.True(t, ok)
	assert.Contains(t, msg, "Queue depth")

	fs.QueueDepth = 0
	ok, msg = fs.ShouldTriggerAlert()
	assert.False(t, ok)
	assert.Equal(t, "", msg)
}

func TestFederationAnalyticsTimeSeries_Aggregate(t *testing.T) {
	base := NewFederationAnalyticsTimeSeries("example.com", PeriodHourly, time.Unix(1700000000, 0).UTC())

	r1 := &FederationAnalyticsTimeSeries{
		TotalInboundVolume:         10,
		TotalOutboundVolume:        20,
		ActivityCount:              100,
		SuccessfulActivities:       90,
		FailedActivities:           10,
		InboxDeliveryP50:           100,
		InboxDeliveryP95:           200,
		InboxDeliveryP99:           300,
		SignatureVerificationTime:  50,
		InstanceReachability:       1.0,
		EndpointAvailability:       0.8,
		SignatureFailures:          1,
		ValidationFailures:         2,
		MalformedActivities:        3,
		RateLimitHits:              4,
		LastSuccessfulContact:      nil,
	}
	r2 := &FederationAnalyticsTimeSeries{
		TotalInboundVolume:         5,
		TotalOutboundVolume:        10,
		ActivityCount:              50,
		SuccessfulActivities:       45,
		FailedActivities:           5,
		InboxDeliveryP50:           200,
		InboxDeliveryP95:           400,
		InboxDeliveryP99:           600,
		SignatureVerificationTime:  150,
		InstanceReachability:       0.5,
		EndpointAvailability:       1.0,
		SignatureFailures:          0,
		ValidationFailures:         0,
		MalformedActivities:        0,
		RateLimitHits:              0,
		LastSuccessfulContact:      nil,
	}

	base.Aggregate([]*FederationAnalyticsTimeSeries{r1, r2})

	assert.Equal(t, int64(15), base.TotalInboundVolume)
	assert.Equal(t, int64(30), base.TotalOutboundVolume)
	assert.Equal(t, int64(150), base.ActivityCount)
	assert.Equal(t, int64(135), base.SuccessfulActivities)
	assert.Equal(t, int64(15), base.FailedActivities)
	assert.Equal(t, int64(2), base.SampleCount)
	assert.Equal(t, int64(150), base.InboxDeliveryP50)
	assert.Equal(t, int64(300), base.InboxDeliveryP95)
	assert.Equal(t, int64(450), base.InboxDeliveryP99)
	assert.InDelta(t, 0.75, base.InstanceReachability, 0.00001)
	assert.InDelta(t, 0.9, base.EndpointAvailability, 0.00001)
	assert.InDelta(t, 0.1, base.ErrorRate, 0.00001)
	assert.False(t, base.UpdatedAt.IsZero())

	// Empty slice should be a no-op.
	updated := base.UpdatedAt
	base.Aggregate(nil)
	assert.Equal(t, updated, base.UpdatedAt)
}

