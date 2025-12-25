package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

const (
	defaultGroupedNotificationsLimit = 20
	maxGroupedNotificationsLimit     = 100
)

func (r *queryResolver) GroupedNotifications(ctx context.Context, input *model.GroupedNotificationsInput) ([]*model.GroupedNotificationGroup, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if r.Registry == nil || r.Registry.Notifications() == nil {
		return nil, errors.New("notification service is not available")
	}

	limit, cursor, includeAll, types, excludeTypes, strategy := parseGroupedNotificationsInput(input)
	listResult, err := r.Registry.Notifications().ListNotifications(ctx, &notifications.ListNotificationsQuery{
		UserID:       username,
		Types:        types,
		ExcludeTypes: excludeTypes,
		IncludeRead:  true,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit * 2,
			Cursor: cursor,
		},
	})
	if err != nil {
		r.Logger.Error("failed to list notifications for grouping",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list notifications"), err)
	}

	groupingService := notifications.NewGroupedNotificationsService(r.Logger)
	groups, err := groupingService.GroupNotifications(ctx, listResult.Notifications, strategy)
	if err != nil {
		r.Logger.Error("failed to group notifications",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to group notifications"), err)
	}

	result := make([]*model.GroupedNotificationGroup, 0, len(groups))
	for _, group := range groups {
		converted := r.convertGroupedNotificationGroup(ctx, groupingService, group, includeAll, r.Registry.Accounts())
		if converted == nil {
			continue
		}
		result = append(result, converted)
	}

	r.trackDynamoOperation(ctx, DynamoOperationRead, int64(len(listResult.Notifications)))
	return result, nil
}

func parseGroupedNotificationsInput(input *model.GroupedNotificationsInput) (limit int, cursor string, includeAll bool, types []string, excludeTypes []string, strategy *notifications.GroupingStrategy) {
	limit = defaultGroupedNotificationsLimit
	if input != nil && input.First != nil && *input.First > 0 && *input.First <= maxGroupedNotificationsLimit {
		limit = *input.First
	}

	if input != nil && input.After != nil {
		cursor = string(*input.After)
	}

	includeAll = false
	if input != nil && input.IncludeAll != nil {
		includeAll = *input.IncludeAll
	}

	types = []string{}
	excludeTypes = []string{}
	if input != nil {
		types = input.Types
		excludeTypes = input.ExcludeTypes
	}

	strategy = notifications.DefaultGroupingStrategy()
	if input != nil && input.Options != nil {
		strategy = applyGroupingStrategyOptions(strategy, input.Options)
	}

	return limit, cursor, includeAll, types, excludeTypes, strategy
}

func applyGroupingStrategyOptions(strategy *notifications.GroupingStrategy, opts *model.GroupingStrategyInput) *notifications.GroupingStrategy {
	if strategy == nil {
		strategy = notifications.DefaultGroupingStrategy()
	}
	if opts == nil {
		return strategy
	}

	if opts.TimeWindowHours != nil && *opts.TimeWindowHours >= 0 {
		strategy.TimeWindow = time.Duration(*opts.TimeWindowHours) * time.Hour
	}
	if opts.MaxGroupSize != nil && *opts.MaxGroupSize > 0 {
		strategy.MaxGroupSize = *opts.MaxGroupSize
	}
	if opts.MinGroupSize != nil && *opts.MinGroupSize > 0 {
		strategy.MinGroupSize = *opts.MinGroupSize
	}
	if opts.SampleSize != nil && *opts.SampleSize > 0 {
		strategy.SampleSize = *opts.SampleSize
	}
	if opts.GroupByType != nil {
		strategy.GroupByType = *opts.GroupByType
	}
	if opts.GroupByTarget != nil {
		strategy.GroupByTarget = *opts.GroupByTarget
	}

	return strategy
}

func (r *queryResolver) convertGroupedNotificationGroup(
	ctx context.Context,
	groupingService *notifications.GroupedNotificationsService,
	group *notifications.GroupedNotification,
	includeAll bool,
	accountService *accounts.Service,
) *model.GroupedNotificationGroup {
	if group == nil {
		return nil
	}

	sampleActors, sampleActorIDs := r.resolveGroupedNotificationSampleActors(ctx, group, accountService)

	summary := groupedNotificationSummary(groupingService, group)
	allNotificationIDs := groupedNotificationIDs(group, includeAll)
	targetStatusID := groupedNotificationTargetStatusID(group)
	mostRecentNotificationID := groupedNotificationMostRecentID(group)

	return &model.GroupedNotificationGroup{
		ID:                       group.ID,
		Type:                     group.Type,
		GroupKey:                 group.GroupKey,
		Count:                    group.Count,
		LatestCreatedAt:          model.Time(group.LatestCreatedAt),
		EarliestCreatedAt:        model.Time(group.EarliestCreatedAt),
		Read:                     group.IsRead,
		Summary:                  summary,
		SampleActors:             sampleActors,
		SampleActorIds:           sampleActorIDs,
		TargetStatusID:           targetStatusID,
		MostRecentNotificationID: mostRecentNotificationID,
		AllNotificationIds:       allNotificationIDs,
	}
}

func (r *queryResolver) resolveGroupedNotificationSampleActors(
	ctx context.Context,
	group *notifications.GroupedNotification,
	accountService *accounts.Service,
) (actors []*activitypub.Actor, actorIDs []string) {
	if group == nil {
		return nil, nil
	}

	actorIDs = make([]string, 0, len(group.SampleAccounts))
	actors = make([]*activitypub.Actor, 0, len(group.SampleAccounts))

	for i := range group.SampleAccounts {
		actorID := strings.TrimSpace(group.SampleAccounts[i].ID)
		if actorID == "" {
			continue
		}
		actorIDs = append(actorIDs, actorID)
		if accountService == nil {
			continue
		}

		account, err := accountService.GetAccount(ctx, actorID)
		if err != nil || account == nil {
			continue
		}

		actor := account.Actor
		if actor == nil {
			actor = r.convertAccountToActor(account)
		}
		if actor == nil {
			continue
		}

		// Enrich summary generation inputs.
		group.SampleAccounts[i].Username = actor.PreferredUsername
		group.SampleAccounts[i].DisplayName = actor.PreferredUsername
		if actor.Name != "" {
			group.SampleAccounts[i].DisplayName = actor.Name
		}
		if actor.Icon != nil {
			group.SampleAccounts[i].Avatar = actor.Icon.URL
		}

		actors = append(actors, actor)
	}

	return actors, actorIDs
}

func groupedNotificationSummary(groupingService *notifications.GroupedNotificationsService, group *notifications.GroupedNotification) string {
	if groupingService == nil || group == nil {
		return ""
	}
	summary := strings.TrimSpace(groupingService.GenerateGroupSummary(group))
	if summary == "" {
		summary = fmt.Sprintf("%d notifications", group.Count)
	}
	return summary
}

func groupedNotificationIDs(group *notifications.GroupedNotification, includeAll bool) []string {
	if group == nil {
		return []string{}
	}
	if !includeAll {
		if group.MostRecentNotif != nil {
			return []string{group.MostRecentNotif.ID}
		}
		return []string{}
	}

	ids := make([]string, 0, len(group.AllNotifications))
	for _, notif := range group.AllNotifications {
		if notif == nil {
			continue
		}
		ids = append(ids, notif.ID)
	}
	return ids
}

func groupedNotificationTargetStatusID(group *notifications.GroupedNotification) *string {
	if group == nil {
		return nil
	}
	if group.TargetStatus != nil && group.TargetStatus.ID != "" {
		return &group.TargetStatus.ID
	}
	if group.MostRecentNotif != nil && group.MostRecentNotif.TargetID != "" {
		return &group.MostRecentNotif.TargetID
	}
	return nil
}

func groupedNotificationMostRecentID(group *notifications.GroupedNotification) *string {
	if group == nil || group.MostRecentNotif == nil || group.MostRecentNotif.ID == "" {
		return nil
	}
	return &group.MostRecentNotif.ID
}
