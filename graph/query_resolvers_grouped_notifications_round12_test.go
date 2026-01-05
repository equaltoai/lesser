package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound12QueryResolvers_GroupedNotifications_Helpers(t *testing.T) {
	limit, cursor, includeAll, types, excludeTypes, strategy := parseGroupedNotificationsInput(nil)
	require.Equal(t, defaultGroupedNotificationsLimit, limit)
	require.Equal(t, "", cursor)
	require.False(t, includeAll)
	require.Empty(t, types)
	require.Empty(t, excludeTypes)
	require.NotNil(t, strategy)

	first := 5
	after := model.Cursor("c1")
	include := true
	opts := &model.GroupingStrategyInput{
		TimeWindowHours: ptrIntValue(0),
		MaxGroupSize:    ptrIntValue(1),
		MinGroupSize:    ptrIntValue(1),
		SampleSize:      ptrIntValue(1),
		GroupByType:     ptrBool(true),
		GroupByTarget:   ptrBool(false),
	}
	_, cursor, includeAll, types, excludeTypes, strategy = parseGroupedNotificationsInput(&model.GroupedNotificationsInput{
		First:        &first,
		After:        &after,
		IncludeAll:   &include,
		Types:        []string{"mention"},
		ExcludeTypes: []string{"follow"},
		Options:      opts,
	})
	require.Equal(t, "c1", cursor)
	require.True(t, includeAll)
	require.Equal(t, []string{"mention"}, types)
	require.Equal(t, []string{"follow"}, excludeTypes)
	require.NotNil(t, strategy)

	strategy = applyGroupingStrategyOptions(nil, nil)
	require.NotNil(t, strategy)

	group := &notifications.GroupedNotification{
		ID:                "g1",
		Type:              "mention",
		GroupKey:          "k1",
		Count:             2,
		LatestCreatedAt:   time.Now(),
		EarliestCreatedAt: time.Now().Add(-time.Hour),
		IsRead:            false,
		SampleAccounts: []notifications.NotificationAccount{
			{ID: "alice"},
		},
		TargetStatus:    &notifications.NotificationStatus{ID: "status-1"},
		MostRecentNotif: &storageModels.Notification{ID: "n1", TargetID: "status-1"},
		AllNotifications: []*storageModels.Notification{
			{ID: "n1"},
			nil,
			{ID: "n2"},
		},
	}

	require.NotEmpty(t, groupedNotificationSummary(notifications.NewGroupedNotificationsService(zap.NewNop()), group))
	require.Equal(t, []string{"n1"}, groupedNotificationIDs(group, false))
	require.Equal(t, []string{"n1", "n2"}, groupedNotificationIDs(group, true))
	require.NotNil(t, groupedNotificationTargetStatusID(group))
	require.NotNil(t, groupedNotificationMostRecentID(group))
}

func TestRound12QueryResolvers_GroupedNotifications_ConversionAndResolver(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	ctx := round12AuthContext("alice")
	groups, err := q.GroupedNotifications(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, groups)

	groupingService := notifications.NewGroupedNotificationsService(zap.NewNop())
	group := &notifications.GroupedNotification{
		ID:                "g1",
		Type:              "mention",
		GroupKey:          "k1",
		Count:             1,
		LatestCreatedAt:   time.Now(),
		EarliestCreatedAt: time.Now(),
		IsRead:            false,
		SampleAccounts: []notifications.NotificationAccount{
			{ID: "alice"},
		},
		MostRecentNotif: &storageModels.Notification{ID: "n1", TargetID: "status-1"},
	}

	actors, actorIDs := q.resolveGroupedNotificationSampleActors(context.Background(), group, nil)
	require.Empty(t, actors)
	require.Equal(t, []string{"alice"}, actorIDs)

	converted := q.convertGroupedNotificationGroup(context.Background(), groupingService, group, false, resolver.Registry.Accounts())
	require.NotNil(t, converted)
	require.NotNil(t, converted.MostRecentNotificationID)
}
