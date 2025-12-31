package models

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNotificationCostTracking_BeforeCreate_UpdateKeys_AddCost(t *testing.T) {
	n := &NotificationCostTracking{
		NotificationID:          "notif-1",
		UserID:                  "user-1",
		Username:                "alice",
		DeliveryMethod:          "push",
		NotificationType:        "mention",
		PushCostMicroCents:      1_000_000,
		WebSocketCostMicroCents: 2_000_000,
		LambdaCostMicroCents:    3_000_000,
		DynamoDBCostMicroCents:  4_000_000,
		TotalCostMicroCents:     10_000_000,
	}

	assert.Empty(t, n.ID)
	assert.True(t, n.Timestamp.IsZero())

	err := n.BeforeCreate()
	assert.NoError(t, err)

	assert.NotEmpty(t, n.ID)
	_, parseErr := uuid.Parse(n.ID)
	assert.NoError(t, parseErr)

	assert.WithinDuration(t, time.Now(), n.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, n.CreatedAt, n.UpdatedAt, 2*time.Second)
	assert.WithinDuration(t, n.CreatedAt, n.Timestamp, 2*time.Second)

	timestampStr := n.Timestamp.Format(common.CompactTimeFormat)
	dateStr := n.Timestamp.Format(common.CompactDateFormat)

	assert.Equal(t, "NOTIF_COST#notif-1", n.PK)
	assert.Equal(t, "TS#"+timestampStr+"#"+n.ID, n.SK)
	assert.Equal(t, "USER#alice", n.GSI1PK)
	assert.Equal(t, "COST#"+timestampStr, n.GSI1SK)
	assert.Equal(t, "METHOD#push", n.GSI2PK)
	assert.Equal(t, "TIMESTAMP#"+timestampStr, n.GSI2SK)
	assert.Equal(t, "DAILY#"+dateStr, n.GSI3PK)
	assert.Equal(t, "COST#"+timestampStr, n.GSI3SK)

	assert.InDelta(t, 1.0, n.PushCostDollars, 0.000001)
	assert.InDelta(t, 2.0, n.WebSocketCostDollars, 0.000001)
	assert.InDelta(t, 3.0, n.LambdaCostDollars, 0.000001)
	assert.InDelta(t, 4.0, n.DynamoDBCostDollars, 0.000001)
	assert.InDelta(t, 10.0, n.TotalCostDollars, 0.000001)

	// Unsupported delivery methods should be ignored (Lesser does not use email/SMS).
	beforeTotal := n.TotalCostMicroCents
	n.AddCost("email", 123)
	n.AddCost("sms", 456)
	assert.Equal(t, beforeTotal, n.TotalCostMicroCents)

	n.AddCost("websocket", 500)
	assert.Equal(t, beforeTotal+500, n.TotalCostMicroCents)

	n.SetError("boom")
	assert.False(t, n.Success)
	assert.Equal(t, "boom", n.ErrorMessage)

	n.SetSuccess()
	assert.True(t, n.Success)
	assert.Empty(t, n.ErrorMessage)
}

func TestNotificationCostAggregation_BeforeCreate_AndRates(t *testing.T) {
	agg := &NotificationCostAggregation{
		Period:         PeriodDaily,
		DeliveryMethod: "push",
		WindowStart:    time.Unix(1700000000, 0).UTC(),
		WindowEnd:      time.Unix(1700003600, 0).UTC(),

		TotalNotifications:   10,
		SuccessfulDeliveries: 8,
		FailedDeliveries:     2,
		TotalRetries:         3,

		TotalPushCostMicroCents:      2_000_000,
		TotalWebSocketCostMicroCents: 1_000_000,
		TotalLambdaCostMicroCents:    3_000_000,
		TotalDynamoDBCostMicroCents:  4_000_000,
		TotalCostMicroCents:          10_000_000,
	}

	err := agg.BeforeCreate()
	assert.NoError(t, err)

	assert.Equal(t, "NOTIF_AGG#daily#push", agg.PK)
	assert.Equal(t, "WINDOW#"+agg.WindowStart.Format(time.RFC3339), agg.SK)

	assert.InDelta(t, 10.0, agg.TotalCostDollars, 0.000001)
	assert.InDelta(t, 80.0, agg.SuccessRate, 0.000001)
	assert.InDelta(t, 30.0, agg.RetryRate, 0.000001)
	assert.InDelta(t, 1.0, agg.CostPerDelivery, 0.000001)

	// Ensure the no-op case is covered.
	agg2 := &NotificationCostAggregation{TotalNotifications: 0}
	agg2.CalculateRates()
	assert.Zero(t, agg2.SuccessRate)
	assert.Zero(t, agg2.RetryRate)
	assert.Zero(t, agg2.CostPerDelivery)
}

func TestNotificationBudget_BeforeCreate_AddSpending_Warnings_Blocking_AllowedMethods_Reset(t *testing.T) {
	b := &NotificationBudget{
		Username:        "alice",
		Period:          PeriodDaily,
		Enabled:         true,
		SendWarningAt:   50,
		BlockDeliveryAt: 90,

		LimitMicroCents: 1000,
		SpentMicroCents: 100,

		AllowedDeliveryMethods: []string{"push", "websocket"},
	}

	err := b.BeforeCreate()
	assert.NoError(t, err)
	assert.Equal(t, "NOTIF_BUDGET#alice", b.PK)
	assert.Equal(t, "PERIOD#daily", b.SK)
	assert.InDelta(t, 0.001, b.LimitDollars, 0.000001)
	assert.InDelta(t, 0.0001, b.SpentDollars, 0.000001)
	assert.Equal(t, int64(900), b.RemainingMicroCents)

	// Warnings are rate-limited.
	b.SpentMicroCents = 600 // 60% of budget
	b.LastWarningTime = time.Now().Add(-2 * time.Hour)
	assert.True(t, b.ShouldSendWarning())
	b.LastWarningTime = time.Now()
	assert.False(t, b.ShouldSendWarning())

	// Allowed methods.
	assert.True(t, b.IsMethodAllowed("push"))
	assert.False(t, b.IsMethodAllowed("email")) // not in allowed list

	// No restrictions => allow anything.
	b.AllowedDeliveryMethods = nil
	assert.True(t, b.IsMethodAllowed("anything"))

	// Blocking cases.
	b.AllowedDeliveryMethods = []string{"push"}
	b.MaxNotificationsPerPeriod = 2
	b.NotificationsSentThisPeriod = 1
	assert.False(t, b.ShouldBlockDelivery())

	ok := b.AddSpending(100) // within budget
	assert.True(t, ok)
	assert.False(t, b.BudgetExceeded)

	// Exceed budget.
	ok = b.AddSpending(10_000)
	assert.False(t, ok)
	assert.True(t, b.BudgetExceeded)
	assert.False(t, b.LastExceededTime.IsZero())
	assert.True(t, b.ShouldBlockDelivery())

	// ResetPeriod calculates next reset based on period.
	start := time.Unix(1700000000, 0).UTC()
	end := time.Unix(1700086400, 0).UTC()
	b.ResetPeriod(start, end)
	assert.Equal(t, int64(0), b.SpentMicroCents)
	assert.Equal(t, int64(0), b.NotificationsSentThisPeriod)
	assert.False(t, b.BudgetExceeded)
	assert.Equal(t, start, b.PeriodStart)
	assert.Equal(t, end, b.PeriodEnd)
	assert.Equal(t, end.AddDate(0, 0, 1), b.NextResetTime)
}

func TestNotificationCostTrackingBuilder_AndCostFunctions(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()

	tracking := NewNotificationCostTrackingBuilder().
		WithNotification("notif-1", "user-1", "alice", "mention").
		WithDelivery("push", "primary", true, 0).
		WithCosts(1, 2, 3, 4).
		WithPerformance(10, 20, 200, 123).
		WithContext("req", "svc", "fn", "lambda-req").
		WithProperty("k", "v").
		WithTag("t", "x").
		WithTimestamp(ts).
		Build()

	assert.Equal(t, "notif-1", tracking.NotificationID)
	assert.Equal(t, "user-1", tracking.UserID)
	assert.Equal(t, "alice", tracking.Username)
	assert.Equal(t, "push", tracking.DeliveryMethod)
	assert.Equal(t, "primary", tracking.Channel)
	assert.True(t, tracking.Success)
	assert.Equal(t, int64(10), tracking.ProcessingTimeMs)
	assert.Equal(t, int64(20), tracking.DeliveryTimeMs)
	assert.Equal(t, int64(30), tracking.TotalTimeMs)
	assert.Equal(t, ts, tracking.Timestamp)
	assert.Equal(t, int64(10), tracking.TotalCostMicroCents)
	assert.Equal(t, "v", tracking.Properties["k"])
	assert.Equal(t, "x", tracking.Tags["t"])

	// Error chain.
	tracking2 := NewNotificationCostTrackingBuilder().WithError("boom").Build()
	assert.False(t, tracking2.Success)
	assert.Equal(t, "boom", tracking2.ErrorMessage)
	assert.False(t, tracking2.Timestamp.IsZero())

	assert.Equal(t, int64(0), CalculateEmailCost(10))
	assert.Equal(t, int64(0), CalculateSMSCost(10))
	assert.Equal(t, int64(2*PushCostPerMessage), CalculatePushCost(2))
	assert.Equal(t, int64(3*WebSocketCostPerMessage), CalculateWebSocketCost(3))
	gbSeconds := 1.5
	assert.Equal(t, int64(LambdaCostPerInvocation)+int64(gbSeconds*float64(LambdaCostPerGBSecond)), CalculateLambdaCost(1, gbSeconds))
	assert.Equal(t, int64(10*DynamoDBReadCostPerRCU)+int64(5*DynamoDBWriteCostPerWCU), CalculateDynamoDBCost(10, 5))
}
