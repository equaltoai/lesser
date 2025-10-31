package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// RemoteActor represents a cached remote actor in DynamoDB using DynamORM
type RemoteActor struct {
	// Primary key: REMOTE_ACTOR#{handle}
	PK string `dynamorm:"pk" json:"pk"`
	// Sort key: PROFILE
	SK string `dynamorm:"sk" json:"sk"`

	// Actor data (ActivityPub actor object)
	Actor *activitypub.Actor `json:"actor"`

	// Handle (user@domain format)
	Handle string `json:"handle"`

	// Domain extracted from handle
	Domain string `json:"domain"`

	// Cache expiration time
	ExpiresAt time.Time `json:"expires_at"`

	// When this was first cached
	CachedAt time.Time `json:"cached_at"`

	// When this was last updated
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for DynamoDB automatic cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl"`
}

// UpdateKeys updates the key fields for DynamORM
func (r *RemoteActor) UpdateKeys() {
	r.PK = fmt.Sprintf("REMOTE_ACTOR#%s", r.Handle)
	r.SK = SKProfile
	r.Domain = extractDomainFromHandle(r.Handle)
	if !r.ExpiresAt.IsZero() {
		r.TTL = r.ExpiresAt.Unix()
	}
}

// extractDomainFromHandle extracts the domain from a handle like user@domain
func extractDomainFromHandle(handle string) string {
	parts := strings.Split(handle, "@")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

// TableName returns the DynamoDB table backing RemoteActor.
func (RemoteActor) TableName() string {
	return MainTableName
}
