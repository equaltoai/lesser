package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGroupedNotificationsService_Round25_generateGroupKey(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	svc := NewGroupedNotificationsService(logger)

	now := time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC)

	t.Run("favourite includes type target and status", func(t *testing.T) {
		n := &models.Notification{
			ID:         "n1",
			Type:       "favourite",
			ActorID:    "alice",
			TargetType: "status",
			TargetID:   "s1",
			CreatedAt:  now,
		}
		key := svc.generateGroupKey(n, DefaultGroupingStrategy())
		assert.Contains(t, key, "type:favourite")
		assert.Contains(t, key, "target:status:s1")
		assert.Contains(t, key, "status:s1")
		assert.Contains(t, key, "time:")
	})

	t.Run("follow groups by time window only", func(t *testing.T) {
		n := &models.Notification{
			ID:        "n2",
			Type:      "follow",
			ActorID:   "bob",
			CreatedAt: now,
			TargetID:  "ignored",
		}
		key := svc.generateGroupKey(n, DefaultGroupingStrategy())
		assert.True(t, strings.HasPrefix(key, "type:follow|time:"), "expected follow key to start with type+time, got %q", key)
		assert.NotContains(t, key, "target:")
		assert.NotContains(t, key, "status:")
	})

	t.Run("mention uses unique id", func(t *testing.T) {
		n := &models.Notification{
			ID:        "n3",
			Type:      "mention",
			ActorID:   "carol",
			CreatedAt: now,
		}
		key := svc.generateGroupKey(n, DefaultGroupingStrategy())
		assert.Contains(t, key, "type:mention")
		assert.Contains(t, key, "unique:n3")
	})
}

func TestGroupedNotificationsService_Round25_GroupNotifications(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	svc := NewGroupedNotificationsService(logger)

	base := time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC)
	strategy := &GroupingStrategy{
		TimeWindow:    time.Hour,
		MaxGroupSize:  50,
		MinGroupSize:  2,
		SampleSize:    2,
		GroupByType:   true,
		GroupByTarget: true,
	}

	notifs := []*models.Notification{
		{
			ID:         "fav-1",
			Type:       "favourite",
			ActorID:    "alice",
			TargetType: "status",
			TargetID:   "status-1",
			CreatedAt:  base.Add(10 * time.Minute),
			IsRead:     false,
		},
		{
			ID:         "fav-2",
			Type:       "favourite",
			ActorID:    "bob",
			TargetType: "status",
			TargetID:   "status-1",
			CreatedAt:  base.Add(20 * time.Minute),
			IsRead:     true,
		},
		{
			ID:        "mention-1",
			Type:      "mention",
			ActorID:   "carol",
			CreatedAt: base.Add(30 * time.Minute),
			IsRead:    false,
		},
		{
			ID:        "follow-1",
			Type:      "follow",
			ActorID:   "dave",
			CreatedAt: base.Add(40 * time.Minute),
			IsRead:    true,
		},
	}

	grouped, err := svc.GroupNotifications(context.Background(), notifs, strategy)
	require.NoError(t, err)
	require.NotEmpty(t, grouped)

	var favGroup *GroupedNotification
	var mentionSingles int
	var followSingles int
	for _, g := range grouped {
		if g.Type == "favourite" && g.Count == 2 {
			favGroup = g
		}
		if g.Type == "mention" && g.Count == 1 {
			mentionSingles++
		}
		if g.Type == "follow" && g.Count == 1 {
			followSingles++
		}
	}

	require.NotNil(t, favGroup)
	assert.NotEmpty(t, favGroup.GroupKey)
	assert.Equal(t, 2, favGroup.Count)
	assert.False(t, favGroup.IsRead, "group should be unread when any notification is unread")
	assert.NotNil(t, favGroup.TargetStatus)
	assert.Equal(t, "status-1", favGroup.TargetStatus.ID)
	assert.LessOrEqual(t, len(favGroup.SampleAccounts), strategy.SampleSize)

	assert.Equal(t, 1, mentionSingles, "mentions should not group")
	assert.Equal(t, 1, followSingles, "single follow group should remain individual when min group size is 2")
}

func TestGroupedNotificationsService_Round25_createGroupedNotification_emptySlice(t *testing.T) {
	t.Parallel()

	svc := NewGroupedNotificationsService(zap.NewNop())
	group := svc.createGroupedNotification("key", nil, DefaultGroupingStrategy())
	assert.Nil(t, group)
}

func TestGroupedNotificationsService_Round25_GenerateGroupSummary(t *testing.T) {
	t.Parallel()

	svc := NewGroupedNotificationsService(zap.NewNop())

	group := &GroupedNotification{
		Type: "favourite",
		SampleAccounts: []NotificationAccount{
			{DisplayName: "Alice"},
			{DisplayName: "Bob"},
			{DisplayName: "Carol"},
		},
	}

	group.Count = 1
	assert.Contains(t, svc.GenerateGroupSummary(group), "favourited")

	group.Count = 2
	assert.Contains(t, svc.GenerateGroupSummary(group), "Alice")

	group.Count = 5
	assert.Contains(t, svc.GenerateGroupSummary(group), "and 4 others")

	group.Type = "reblog"
	group.Count = 1
	assert.Contains(t, svc.GenerateGroupSummary(group), "boosted")

	group.Type = "follow"
	group.Count = 3
	assert.Contains(t, svc.GenerateGroupSummary(group), "followed you")

	group.Type = "mention"
	group.Count = 1
	assert.Contains(t, svc.GenerateGroupSummary(group), "mentioned you")

	group.Type = "unknown"
	group.Count = 1
	assert.Contains(t, svc.GenerateGroupSummary(group), "Notification from")

	group.Count = 2
	assert.Contains(t, svc.GenerateGroupSummary(group), "notifications")
}

func TestGroupedNotificationsService_Round25_MarkGroupAsRead(t *testing.T) {
	t.Parallel()

	svc := NewGroupedNotificationsService(zap.NewNop())

	group := &GroupedNotification{
		Type: "favourite",
		AllNotifications: []*models.Notification{
			{ID: "n1", IsRead: false},
			{ID: "n2", IsRead: true},
			{ID: "n3", IsRead: false},
		},
	}

	calls := 0
	err := svc.MarkGroupAsRead(context.Background(), group, func(_ context.Context, id string) error {
		calls++
		if id == "n3" {
			return errors.New("db unavailable")
		}
		return nil
	})
	require.Error(t, err)
	assert.Equal(t, 2, calls, "should attempt to mark each unread notification")
	assert.True(t, group.IsRead, "group should be marked read even if some updates fail")
}
