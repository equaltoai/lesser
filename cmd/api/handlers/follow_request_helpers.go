package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// convertActorToAccount converts an ActivityPub actor to a Mastodon account format
func (h *Handler) convertActorToAccount(ctx context.Context, actor *activitypub.Actor) map[string]any {
	// Default avatar and header
	avatar := fmt.Sprintf("https://%s/avatars/default.png", h.cfg.Domain)
	header := fmt.Sprintf("https://%s/headers/default.png", h.cfg.Domain)

	if actor.Icon != nil && actor.Icon.URL != "" {
		avatar = actor.Icon.URL
	}
	if actor.Image != nil && actor.Image.URL != "" {
		header = actor.Image.URL
	}

	// Get metadata
	createdAt := time.Now() // Default fallback
	lastStatusAt := ""

	// Get actor with metadata
	_, metadata, err := h.store.GetActorWithMetadata(ctx, actor.PreferredUsername)
	if err == nil && metadata != nil {
		createdAt = metadata.CreatedAt
		if metadata.LastStatusAt != nil {
			lastStatusAt = metadata.LastStatusAt.Format(time.RFC3339)
		}
	}

	// Get counts
	statusesCount, _ := h.store.GetStatusCount(ctx, actor.ID)
	followersCount, _ := h.store.GetFollowersCount(ctx, actor.ID)

	// Get following count by checking first page
	following, _, _ := h.store.GetFollowing(ctx, actor.PreferredUsername, 1, "")
	followingCount := len(following)

	return map[string]any{
		"id":              actor.PreferredUsername,
		"username":        actor.PreferredUsername,
		"acct":            actor.PreferredUsername,
		"url":             actor.URL,
		"display_name":    actor.Name,
		"note":            actor.Summary,
		"avatar":          avatar,
		"avatar_static":   avatar,
		"header":          header,
		"header_static":   header,
		"locked":          actor.ManuallyApprovesFollowers,
		"bot":             actor.Type == "Service",
		"discoverable":    actor.Discoverable,
		"created_at":      createdAt.Format(time.RFC3339),
		"last_status_at":  lastStatusAt,
		"statuses_count":  statusesCount,
		"followers_count": followersCount,
		"following_count": followingCount,
		"fields":          []any{},
		"emojis":          []any{},
	}
}

// sendAcceptActivity sends an Accept activity to the follower
func (h *Handler) sendAcceptActivity(ctx context.Context, followerID, followedID string) error {
	// Get follower actor to determine inbox
	followerActor, err := h.store.GetActor(ctx, followerID)
	if err != nil {
		return fmt.Errorf("failed to get follower actor: %w", err)
	}

	// Get followed actor
	followedActor, err := h.store.GetActor(ctx, followedID)
	if err != nil {
		return fmt.Errorf("failed to get followed actor: %w", err)
	}

	// Create Accept activity
	acceptActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Accept",
			ID:      fmt.Sprintf("https://%s/activities/%d", h.cfg.Domain, time.Now().UnixNano()),
		},
		Actor: followedActor.ID,
		Object: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: "Follow",
				ID:   fmt.Sprintf("https://%s/follows/%s", followerActor.URL, followedID),
			},
			Actor:  followerActor.ID,
			Object: followedActor.ID,
		},
	}

	// Send to follower's inbox
	if followerActor.Inbox != "" {
		if err := h.deliverActivity(ctx, acceptActivity, followerActor.Inbox); err != nil {
			return fmt.Errorf("failed to deliver accept activity: %w", err)
		}
	}

	h.logger.Info("accept activity sent",
		zap.String("follower_id", followerID),
		zap.String("followed_id", followedID),
		zap.String("inbox", followerActor.Inbox))

	return nil
}

// sendRejectActivity sends a Reject activity to the follower
func (h *Handler) sendRejectActivity(ctx context.Context, followerID, followedID string) error {
	// Get follower actor to determine inbox
	followerActor, err := h.store.GetActor(ctx, followerID)
	if err != nil {
		return fmt.Errorf("failed to get follower actor: %w", err)
	}

	// Get followed actor
	followedActor, err := h.store.GetActor(ctx, followedID)
	if err != nil {
		return fmt.Errorf("failed to get followed actor: %w", err)
	}

	// Create Reject activity
	rejectActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Reject",
			ID:      fmt.Sprintf("https://%s/activities/%d", h.cfg.Domain, time.Now().UnixNano()),
		},
		Actor: followedActor.ID,
		Object: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: "Follow",
				ID:   fmt.Sprintf("https://%s/follows/%s", followerActor.URL, followedID),
			},
			Actor:  followerActor.ID,
			Object: followedActor.ID,
		},
	}

	// Send to follower's inbox
	if followerActor.Inbox != "" {
		if err := h.deliverActivity(ctx, rejectActivity, followerActor.Inbox); err != nil {
			return fmt.Errorf("failed to deliver reject activity: %w", err)
		}
	}

	h.logger.Info("reject activity sent",
		zap.String("follower_id", followerID),
		zap.String("followed_id", followedID),
		zap.String("inbox", followerActor.Inbox))

	return nil
}

// deliverActivity delivers an ActivityPub activity to a remote inbox
func (h *Handler) deliverActivity(ctx context.Context, activity *activitypub.Activity, inboxURL string) error {
	// This would implement HTTP signature authentication and delivery
	// For now, just log the delivery attempt
	h.logger.Info("delivering activity",
		zap.String("type", activity.Type),
		zap.String("id", activity.ID),
		zap.String("inbox", inboxURL))

	// Convert activity to map for delivery
	activityBytes, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	var activityMap map[string]any
	if err := json.Unmarshal(activityBytes, &activityMap); err != nil {
		return fmt.Errorf("failed to unmarshal activity to map: %w", err)
	}

	// ActivityPub delivery with HTTP signatures
	if err := h.deliverActivityPubActivity(ctx, activityMap, inboxURL); err != nil {
		h.logger.Error("failed to deliver ActivityPub activity", zap.Error(err))
		return err
	}
	// This would involve:
	// 1. Serializing the activity to JSON
	// 2. Signing the request with the server's private key
	// 3. Making an HTTP POST to the inbox URL
	// 4. Handling delivery failures and retries

	return nil
}

// Helper method for ActivityPub delivery
func (h *Handler) deliverActivityPubActivity(ctx context.Context, activity map[string]any, targetInbox string) error {
	// Convert activity to JSON
	activityJSON, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	// Create HTTP signature
	signature, err := h.createHTTPSignature(ctx, "POST", targetInbox, activityJSON)
	if err != nil {
		return fmt.Errorf("failed to create HTTP signature: %w", err)
	}

	// Send HTTP POST request with signature
	req, err := http.NewRequestWithContext(ctx, "POST", targetInbox, bytes.NewReader(activityJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Signature", signature)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-2xx status code: %d", resp.StatusCode)
	}

	return nil
}

func (h *Handler) createHTTPSignature(_ context.Context, _ string, _ string, _ []byte) (string, error) {
	// Simplified HTTP signature creation
	// In production, would use proper HTTP signature library
	return "keyId=\"#main-key\",algorithm=\"rsa-sha256\",headers=\"(request-target) date digest\"", nil
}
