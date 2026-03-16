package notifications

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
)

func actorTypeOrDefault(actorType string) string {
	trimmed := strings.TrimSpace(actorType)
	if trimmed == "" {
		return "user"
	}
	return trimmed
}

func NewFollowNotificationCommand(recipientUserID, followerID, targetID, actorType string) *CreateNotificationCommand {
	return &CreateNotificationCommand{
		UserID:     recipientUserID,
		Type:       common.NotificationTypeFollow,
		ActorID:    followerID,
		ActorType:  actorTypeOrDefault(actorType),
		TargetID:   targetID,
		TargetType: "account",
		Title:      fmt.Sprintf("%s followed you", followerID),
		Body:       fmt.Sprintf("%s started following you", followerID),
		GroupKey:   fmt.Sprintf("follow:%s", followerID),
		Data: map[string]interface{}{
			"follower": followerID,
		},
	}
}

func NewMentionNotificationCommand(recipientUserID, mentionerID, statusID, statusURL, actorType string) *CreateNotificationCommand {
	cmd := &CreateNotificationCommand{
		UserID:     recipientUserID,
		Type:       common.NotificationTypeMention,
		ActorID:    mentionerID,
		ActorType:  actorTypeOrDefault(actorType),
		TargetID:   statusID,
		TargetType: "status",
		Title:      fmt.Sprintf("%s mentioned you", mentionerID),
		Body:       fmt.Sprintf("%s mentioned you", mentionerID),
		GroupKey:   fmt.Sprintf("mention:%s", statusID),
		Data: map[string]interface{}{
			"status_id": statusID,
			"mentioner": mentionerID,
		},
	}
	if strings.TrimSpace(statusURL) != "" {
		cmd.Data["status_url"] = statusURL
	}
	return cmd
}

func NewReplyNotificationCommand(recipientUserID, replierID, statusID, parentStatusID, statusURL, actorType string) *CreateNotificationCommand {
	cmd := &CreateNotificationCommand{
		UserID:     recipientUserID,
		Type:       common.NotificationTypeReply,
		ActorID:    replierID,
		ActorType:  actorTypeOrDefault(actorType),
		TargetID:   statusID,
		TargetType: "status",
		Title:      fmt.Sprintf("%s replied to your post", replierID),
		Body:       fmt.Sprintf("%s replied to your post", replierID),
		GroupKey:   fmt.Sprintf("reply:%s", statusID),
		Data: map[string]interface{}{
			"status_id":        statusID,
			"parent_status_id": parentStatusID,
			"replier":          replierID,
		},
	}
	if strings.TrimSpace(statusURL) != "" {
		cmd.Data["status_url"] = statusURL
	}
	return cmd
}

func NewReblogNotificationCommand(recipientUserID, boosterID, statusID, statusURL, actorType string) *CreateNotificationCommand {
	cmd := &CreateNotificationCommand{
		UserID:     recipientUserID,
		Type:       common.NotificationTypeReblog,
		ActorID:    boosterID,
		ActorType:  actorTypeOrDefault(actorType),
		TargetID:   statusID,
		TargetType: "status",
		Title:      fmt.Sprintf("%s boosted your post", boosterID),
		Body:       fmt.Sprintf("%s boosted your post", boosterID),
		GroupKey:   fmt.Sprintf("reblog:%s", statusID),
		Data: map[string]interface{}{
			"status_id": statusID,
			"booster":   boosterID,
		},
	}
	if strings.TrimSpace(statusURL) != "" {
		cmd.Data["status_url"] = statusURL
	}
	return cmd
}

func NewFavouriteNotificationCommand(recipientUserID, likerID, statusID, statusURL, actorType string) *CreateNotificationCommand {
	cmd := &CreateNotificationCommand{
		UserID:     recipientUserID,
		Type:       common.NotificationTypeFavourite,
		ActorID:    likerID,
		ActorType:  actorTypeOrDefault(actorType),
		TargetID:   statusID,
		TargetType: "status",
		Title:      fmt.Sprintf("%s favourited your post", likerID),
		Body:       fmt.Sprintf("%s favourited your post", likerID),
		GroupKey:   fmt.Sprintf("favourite:%s", statusID),
		Data: map[string]interface{}{
			"status_id": statusID,
			"liker":     likerID,
		},
	}
	if strings.TrimSpace(statusURL) != "" {
		cmd.Data["status_url"] = statusURL
	}
	return cmd
}
