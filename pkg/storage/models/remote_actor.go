package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// RemoteActor represents a cached remote actor in DynamoDB using DynamORM
type RemoteActor struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key: REMOTE_ACTOR#{handle}
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	// Sort key: PROFILE
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// Actor data (ActivityPub actor object)
	Actor *activitypub.Actor `theorydb:"json,attr:actor" json:"actor"`

	// Handle (user@domain format)
	Handle string `theorydb:"attr:handle" json:"handle"`

	// Domain extracted from handle
	Domain string `theorydb:"attr:domain" json:"domain"`

	// Cache expiration time
	ExpiresAt time.Time `theorydb:"attr:expiresAt" json:"expires_at"`

	// When this was first cached
	CachedAt time.Time `theorydb:"attr:cachedAt" json:"cached_at"`

	// When this was last updated
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL for DynamoDB automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`
}

// UpdateKeys updates the key fields for DynamORM
func (r *RemoteActor) UpdateKeys() {
	r.Handle = NormalizeRemoteActorHandle(r.Handle)
	r.PK = fmt.Sprintf("REMOTE_ACTOR#%s", r.Handle)
	r.SK = SKProfile
	r.Domain = extractDomainFromHandle(r.Handle)
	if !r.ExpiresAt.IsZero() {
		r.TTL = r.ExpiresAt.Unix()
	}
}

// NormalizeRemoteActorHandle canonicalizes remote actor cache identifiers so
// writes and reads converge on the same durable cache key.
func NormalizeRemoteActorHandle(handle string) string {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return ""
	}

	if parsed, err := url.Parse(handle); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		host := normalizeRemoteActorDomain(parsed.Hostname())
		if host == "" {
			return ""
		}

		path := strings.Trim(parsed.Path, "/")
		if path == "" {
			return ""
		}

		parts := strings.Split(path, "/")
		candidate := ""
		for i, part := range parts {
			if (part == "users" || part == "actors") && i+1 < len(parts) {
				candidate = parts[i+1]
				break
			}
		}
		if candidate == "" && len(parts) > 0 {
			last := parts[len(parts)-1]
			if strings.HasPrefix(last, "@") {
				candidate = last
			}
		}

		candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "@"))
		if candidate != "" {
			return strings.ToLower(candidate) + "@" + host
		}

		return ""
	}

	handle = strings.TrimPrefix(handle, "@")
	parts := strings.Split(handle, "@")
	if len(parts) != 2 {
		if strings.Contains(handle, "/") {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(handle))
	}

	username := strings.ToLower(strings.TrimSpace(parts[0]))
	domain := normalizeRemoteActorDomain(parts[1])
	if username == "" || domain == "" {
		return ""
	}

	return username + "@" + domain
}

// extractDomainFromHandle extracts the domain from a handle like user@domain
func extractDomainFromHandle(handle string) string {
	handle = NormalizeRemoteActorHandle(handle)
	parts := strings.Split(handle, "@")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

func normalizeRemoteActorDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, ":"); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
}

// TableName returns the DynamoDB table backing RemoteActor.
func (RemoteActor) TableName() string {
	return MainTableName
}
