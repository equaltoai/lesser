package handlers

import (
	"fmt"
	neturl "net/url"
	"strings"
	"unicode"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

const maxNotificationFallbackActorIDLength = 2048

func (h *Handler) fallbackNotificationActor(actorID string) *activitypub.Actor {
	actorID = strings.TrimSpace(actorID)
	if !validNotificationFallbackActorID(actorID) {
		return nil
	}

	username := extractUsernameFromNotificationActorID(actorID)
	if username == "" {
		return nil
	}

	baseURL := ""
	localDomain := ""
	if h != nil && h.cfg != nil {
		baseURL = strings.TrimSuffix(strings.TrimSpace(h.cfg.BaseURL()), "/")
		localDomain = strings.TrimSpace(h.cfg.Domain)
	}

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{Type: activitypub.PersonType},
		PreferredUsername: username,
		Name:              username,
	}

	if strings.Contains(actorID, "://") {
		parsed, err := neturl.Parse(actorID)
		if err != nil || !validNotificationFallbackActorURL(parsed) {
			return nil
		}
		actor.ID = actorID
		actor.URL = actorID
		// For foreign-domain URLs, surface the domain in the actor name to
		// prevent the extracted username from being mistaken for a local user.
		if localDomain != "" && username != "" {
			actorDomain := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
			if actorDomain != "" && !strings.EqualFold(actorDomain, localDomain) {
				actor.Name = username + "@" + actorDomain
			}
		}
	} else if baseURL != "" && username != "" && !hasRemoteDomainIndicator(actorID, localDomain) {
		// Create local URLs for bare usernames and local-domain handles.
		// Remote user@domain and email-like identifiers with foreign domains
		// are rejected from local URL creation and fall through to opaque IDs.
		actor.ID = fmt.Sprintf("%s/users/%s", baseURL, username)
		actor.URL = fmt.Sprintf("%s/@%s", baseURL, username)
		actor.Inbox = fmt.Sprintf("%s/users/%s/inbox", baseURL, username)
		actor.Outbox = fmt.Sprintf("%s/users/%s/outbox", baseURL, username)
	} else {
		// Non-URL identifiers (email addresses, opaque ids) must not be used as URLs.
		actor.ID = actorID
	}

	return actor
}

func validNotificationFallbackActorID(actorID string) bool {
	if actorID == "" || len(actorID) > maxNotificationFallbackActorIDLength {
		return false
	}
	return strings.IndexFunc(actorID, func(r rune) bool {
		return unicode.IsControl(r) || r == '<' || r == '>' || r == '"' || r == '\''
	}) == -1
}

func validNotificationFallbackActorURL(parsed *neturl.URL) bool {
	if parsed == nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != schemeHTTPS && scheme != schemeHTTP {
		return false
	}
	if strings.TrimSpace(parsed.Hostname()) == "" || parsed.User != nil {
		return false
	}
	return parsed.RawQuery == "" && parsed.Fragment == ""
}

func extractUsernameFromNotificationActorID(actorID string) string {
	cleaned := strings.TrimSpace(actorID)
	if !validNotificationFallbackActorID(cleaned) {
		return ""
	}

	cleaned = strings.TrimPrefix(cleaned, "@")

	// URL-style actor identifier
	if strings.Contains(cleaned, "://") {
		parsed, err := neturl.Parse(cleaned)
		if err != nil || !validNotificationFallbackActorURL(parsed) {
			return ""
		}
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for i := len(segments) - 1; i >= 0; i-- {
			if seg := strings.TrimSpace(segments[i]); seg != "" {
				return sanitizedNotificationActorUsername(strings.TrimPrefix(seg, "@"))
			}
		}
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			return sanitizedNotificationActorUsername(host)
		}
		return ""
	}

	// Handle "user@domain" and email-like identifiers.
	if strings.Contains(cleaned, "@") {
		parts := strings.Split(cleaned, "@")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return sanitizedNotificationActorUsername(parts[0])
		}
	}

	return sanitizedNotificationActorUsername(cleaned)
}

func sanitizedNotificationActorUsername(username string) string {
	username = strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if username == "" {
		return ""
	}
	if err := common.ValidateActivityPubUsername(username); err != nil {
		return ""
	}
	return username
}

// hasRemoteDomainIndicator returns true when the actorID has a domain indicator
// (@ sign after optional leading @) that points to a non-local domain. This
// prevents email-like and user@remote.domain actor IDs from being assigned
// local instance endpoints. (CSR-051)
func hasRemoteDomainIndicator(actorID, localDomain string) bool {
	cleaned := strings.TrimPrefix(strings.TrimSpace(actorID), "@")
	at := strings.LastIndex(cleaned, "@")
	if at == -1 {
		return false // no domain indicator at all
	}
	if strings.TrimSpace(localDomain) == "" {
		return false // can't determine — allow local URL creation
	}
	domainPart := strings.TrimSpace(cleaned[at+1:])
	return !strings.EqualFold(domainPart, strings.TrimSpace(localDomain))
}
