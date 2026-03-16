package notifications

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// GroupedNotificationsService provides advanced notification grouping functionality
type GroupedNotificationsService struct {
	logger *zap.Logger
}

// NewGroupedNotificationsService creates a new grouped notifications service
func NewGroupedNotificationsService(logger *zap.Logger) *GroupedNotificationsService {
	return &GroupedNotificationsService{
		logger: logger,
	}
}

// GroupedNotification represents a group of similar notifications
type GroupedNotification struct {
	ID                string                 `json:"id"`
	Type              string                 `json:"type"`
	GroupKey          string                 `json:"group_key"`
	Count             int                    `json:"count"`
	LatestCreatedAt   time.Time              `json:"latest_created_at"`
	EarliestCreatedAt time.Time              `json:"earliest_created_at"`
	IsRead            bool                   `json:"is_read"`
	SampleAccounts    []NotificationAccount  `json:"sample_accounts"`
	TargetStatus      *NotificationStatus    `json:"status,omitempty"`
	MostRecentNotif   *models.Notification   `json:"most_recent_notification"`
	AllNotifications  []*models.Notification `json:"all_notifications,omitempty"`
}

// NotificationAccount represents an account in grouped notifications
type NotificationAccount struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Avatar      string    `json:"avatar"`
	IsBot       bool      `json:"bot"`
	CreatedAt   time.Time `json:"created_at"`
}

// NotificationStatus represents a status in grouped notifications
type NotificationStatus struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	URL        string    `json:"url"`
	Visibility string    `json:"visibility"`
}

// GroupingStrategy defines how notifications should be grouped
type GroupingStrategy struct {
	TimeWindow    time.Duration `json:"time_window"`     // Group notifications within this time window
	MaxGroupSize  int           `json:"max_group_size"`  // Maximum notifications per group
	MinGroupSize  int           `json:"min_group_size"`  // Minimum to consider grouping
	SampleSize    int           `json:"sample_size"`     // Number of sample accounts to include
	GroupByType   bool          `json:"group_by_type"`   // Group by notification type
	GroupByTarget bool          `json:"group_by_target"` // Group by target object
}

// DefaultGroupingStrategy returns the default grouping strategy
func DefaultGroupingStrategy() *GroupingStrategy {
	return &GroupingStrategy{
		TimeWindow:    24 * time.Hour, // Group within 24 hours
		MaxGroupSize:  50,             // Max 50 notifications per group
		MinGroupSize:  2,              // Need at least 2 to group
		SampleSize:    3,              // Show 3 sample accounts
		GroupByType:   true,
		GroupByTarget: true,
	}
}

// GroupNotifications groups similar notifications together
func (gns *GroupedNotificationsService) GroupNotifications(
	_ context.Context,
	notifications []*models.Notification,
	strategy *GroupingStrategy,
) ([]*GroupedNotification, error) {
	if strategy == nil {
		strategy = DefaultGroupingStrategy()
	}

	// Sort notifications by created time (newest first)
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt.After(notifications[j].CreatedAt)
	})

	// Group notifications by their grouping criteria
	groups := make(map[string][]*models.Notification)

	for _, notif := range notifications {
		groupKey := gns.generateGroupKey(notif, strategy)
		groups[groupKey] = append(groups[groupKey], notif)
	}

	// Convert groups to GroupedNotification objects
	var groupedNotifications []*GroupedNotification

	for groupKey, groupNotifs := range groups {
		if len(groupNotifs) < strategy.MinGroupSize {
			// If group is too small, add individual notifications
			for _, notif := range groupNotifs {
				grouped := gns.createSingleNotificationGroup(notif)
				groupedNotifications = append(groupedNotifications, grouped)
			}
			continue
		}

		grouped := gns.createGroupedNotification(groupKey, groupNotifs, strategy)
		groupedNotifications = append(groupedNotifications, grouped)
	}

	// Sort grouped notifications by latest created time
	sort.Slice(groupedNotifications, func(i, j int) bool {
		return groupedNotifications[i].LatestCreatedAt.After(groupedNotifications[j].LatestCreatedAt)
	})

	return groupedNotifications, nil
}

// generateGroupKey generates a key for grouping similar notifications
func (gns *GroupedNotificationsService) generateGroupKey(
	notification *models.Notification,
	strategy *GroupingStrategy,
) string {
	var keyParts []string

	if strategy.GroupByType {
		keyParts = append(keyParts, "type:"+notification.Type)
	}

	if strategy.GroupByTarget && notification.TargetID != "" {
		keyParts = append(keyParts, "target:"+notification.TargetType+":"+notification.TargetID)
	}

	// Group by time window
	timeSlot := notification.CreatedAt.Truncate(strategy.TimeWindow)
	keyParts = append(keyParts, "time:"+timeSlot.Format(time.RFC3339))

	// Special grouping rules for specific notification types
	switch notification.Type {
	case "favourite", "reblog":
		// Group by target status
		if notification.TargetID != "" {
			keyParts = append(keyParts, "status:"+notification.TargetID)
		}
	case "follow":
		// Group follow notifications by time window only
		keyParts = []string{"type:follow", "time:" + timeSlot.Format(time.RFC3339)}
	case "mention":
		// Don't group mentions - each should be individual
		keyParts = append(keyParts, "unique:"+notification.ID)
	case "reply":
		// Replies should remain individual so parent-thread context stays explicit
		keyParts = append(keyParts, "unique:"+notification.ID)
	}

	return strings.Join(keyParts, "|")
}

// createGroupedNotification creates a GroupedNotification from a group of notifications
func (gns *GroupedNotificationsService) createGroupedNotification(
	groupKey string,
	notifications []*models.Notification,
	strategy *GroupingStrategy,
) *GroupedNotification {
	if err := common.ValidateSliceNotEmpty("notifications", notifications); err != nil {
		return nil
	}

	// Sort by creation time (newest first)
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt.After(notifications[j].CreatedAt)
	})

	mostRecent := notifications[0]
	oldest := notifications[len(notifications)-1]

	// Collect unique accounts
	accountMap := make(map[string]*NotificationAccount)
	for _, notif := range notifications {
		if _, exists := accountMap[notif.ActorID]; !exists {
			accountMap[notif.ActorID] = &NotificationAccount{
				ID:        notif.ActorID,
				CreatedAt: notif.CreatedAt,
				// Additional fields would be populated from actor data
			}
		}
	}

	// Convert to slice and sort by creation time
	var sampleAccounts []NotificationAccount
	for _, account := range accountMap {
		sampleAccounts = append(sampleAccounts, *account)
	}

	sort.Slice(sampleAccounts, func(i, j int) bool {
		return sampleAccounts[i].CreatedAt.After(sampleAccounts[j].CreatedAt)
	})

	// Limit to sample size
	if len(sampleAccounts) > strategy.SampleSize {
		sampleAccounts = sampleAccounts[:strategy.SampleSize]
	}

	// Check if all notifications are read
	allRead := true
	for _, notif := range notifications {
		if !notif.IsRead {
			allRead = false
			break
		}
	}

	grouped := &GroupedNotification{
		ID:                fmt.Sprintf("group_%s", groupKey),
		Type:              mostRecent.Type,
		GroupKey:          groupKey,
		Count:             len(notifications),
		LatestCreatedAt:   mostRecent.CreatedAt,
		EarliestCreatedAt: oldest.CreatedAt,
		IsRead:            allRead,
		SampleAccounts:    sampleAccounts,
		MostRecentNotif:   mostRecent,
		AllNotifications:  notifications,
	}

	// Add target status information if available
	if mostRecent.TargetType == "status" && mostRecent.TargetID != "" {
		grouped.TargetStatus = &NotificationStatus{
			ID: mostRecent.TargetID,
			// Additional fields would be populated from status data
		}
	}

	return grouped
}

// createSingleNotificationGroup creates a GroupedNotification for a single notification
func (gns *GroupedNotificationsService) createSingleNotificationGroup(
	notification *models.Notification,
) *GroupedNotification {
	account := NotificationAccount{
		ID:        notification.ActorID,
		CreatedAt: notification.CreatedAt,
	}

	grouped := &GroupedNotification{
		ID:                notification.ID,
		Type:              notification.Type,
		GroupKey:          notification.ID, // Use notification ID as group key
		Count:             1,
		LatestCreatedAt:   notification.CreatedAt,
		EarliestCreatedAt: notification.CreatedAt,
		IsRead:            notification.IsRead,
		SampleAccounts:    []NotificationAccount{account},
		MostRecentNotif:   notification,
		AllNotifications:  []*models.Notification{notification},
	}

	// Add target status information if available
	if notification.TargetType == "status" && notification.TargetID != "" {
		grouped.TargetStatus = &NotificationStatus{
			ID: notification.TargetID,
		}
	}

	return grouped
}

// GenerateGroupSummary generates a human-readable summary for a group
func (gns *GroupedNotificationsService) GenerateGroupSummary(
	group *GroupedNotification,
) string {
	switch group.Type {
	case "favourite":
		if group.Count == 1 {
			return fmt.Sprintf("%s favourited your post", group.SampleAccounts[0].DisplayName)
		}
		if group.Count <= 3 {
			names := make([]string, len(group.SampleAccounts))
			for i, account := range group.SampleAccounts {
				names[i] = account.DisplayName
			}
			return fmt.Sprintf("%s favourited your post", strings.Join(names, ", "))
		}
		return fmt.Sprintf("%s and %d others favourited your post",
			group.SampleAccounts[0].DisplayName, group.Count-1)

	case "reblog":
		if group.Count == 1 {
			return fmt.Sprintf("%s boosted your post", group.SampleAccounts[0].DisplayName)
		}
		if group.Count <= 3 {
			names := make([]string, len(group.SampleAccounts))
			for i, account := range group.SampleAccounts {
				names[i] = account.DisplayName
			}
			return fmt.Sprintf("%s boosted your post", strings.Join(names, ", "))
		}
		return fmt.Sprintf("%s and %d others boosted your post",
			group.SampleAccounts[0].DisplayName, group.Count-1)

	case "follow":
		if group.Count == 1 {
			return fmt.Sprintf("%s followed you", group.SampleAccounts[0].DisplayName)
		}
		if group.Count <= 3 {
			names := make([]string, len(group.SampleAccounts))
			for i, account := range group.SampleAccounts {
				names[i] = account.DisplayName
			}
			return fmt.Sprintf("%s followed you", strings.Join(names, ", "))
		}
		return fmt.Sprintf("%s and %d others followed you",
			group.SampleAccounts[0].DisplayName, group.Count-1)

	case "mention":
		// Mentions typically aren't grouped, but just in case
		return fmt.Sprintf("%s mentioned you", group.SampleAccounts[0].DisplayName)
	case "reply":
		return fmt.Sprintf("%s replied to your post", group.SampleAccounts[0].DisplayName)

	default:
		if group.Count == 1 {
			return fmt.Sprintf("Notification from %s", group.SampleAccounts[0].DisplayName)
		}
		return fmt.Sprintf("%d notifications", group.Count)
	}
}

// MarkGroupAsRead marks all notifications in a group as read
func (gns *GroupedNotificationsService) MarkGroupAsRead(
	ctx context.Context,
	group *GroupedNotification,
	markReadFunc func(context.Context, string) error,
) error {
	for _, notif := range group.AllNotifications {
		if !notif.IsRead {
			if err := markReadFunc(ctx, notif.ID); err != nil {
				gns.logger.Error("failed to mark notification as read",
					zap.String("notification_id", notif.ID),
					zap.Error(err))
				// Continue with other notifications
			}
		}
	}

	group.IsRead = true
	return nil
}
