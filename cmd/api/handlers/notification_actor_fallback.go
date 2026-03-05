package handlers

import (
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

func (h *Handler) fallbackNotificationActor(actorID string) *activitypub.Actor {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil
	}

	username := extractUsernameFromNotificationActorID(actorID)
	if username == "" {
		username = actorID
	}

	baseURL := ""
	if h != nil && h.cfg != nil {
		baseURL = strings.TrimSuffix(strings.TrimSpace(h.cfg.BaseURL()), "/")
	}

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{Type: activitypub.PersonType},
	}

	if strings.Contains(actorID, "://") {
		actor.ID = actorID
		actor.URL = actorID
	} else if baseURL != "" && username != "" {
		actor.ID = fmt.Sprintf("%s/users/%s", baseURL, username)
		actor.URL = fmt.Sprintf("%s/@%s", baseURL, username)
		actor.Inbox = fmt.Sprintf("%s/users/%s/inbox", baseURL, username)
		actor.Outbox = fmt.Sprintf("%s/users/%s/outbox", baseURL, username)
	} else {
		actor.ID = actorID
		actor.URL = actorID
	}

	actor.PreferredUsername = username
	actor.Name = username

	return actor
}

func extractUsernameFromNotificationActorID(actorID string) string {
	cleaned := strings.TrimSpace(actorID)
	if cleaned == "" {
		return ""
	}

	cleaned = strings.TrimPrefix(cleaned, "@")

	// URL-style actor identifier
	if strings.Contains(cleaned, "://") {
		parsed, err := neturl.Parse(cleaned)
		if err != nil {
			return ""
		}
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for i := len(segments) - 1; i >= 0; i-- {
			if seg := strings.TrimSpace(segments[i]); seg != "" {
				return strings.TrimPrefix(seg, "@")
			}
		}
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			return host
		}
		return ""
	}

	// Handle "user@domain" and email-like identifiers.
	if strings.Contains(cleaned, "@") {
		parts := strings.Split(cleaned, "@")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}

	return cleaned
}
