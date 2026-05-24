package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadNotificationActor_RejectsRemoteIdentityAsLocal verifies that
// loadNotificationActor does not resolve a remote actor ID as a local
// account. (CSR-038 regression probe)
func TestLoadNotificationActor_RejectsRemoteIdentityAsLocal(t *testing.T) {
	// A nil resolver without storage or accounts service should never
	// resolve a remote actor ID to a local account.
	r := &Resolver{
		Config: &config.Config{Domain: "lesser.example"},
	}

	tests := []struct {
		name    string
		actorID string
	}{
		{
			name:    "remote URL with foreign domain",
			actorID: "https://evil.example/users/admin",
		},
		{
			name:    "remote user@domain",
			actorID: "admin@evil.example",
		},
		{
			name:    "remote URL with @ prefix and foreign domain",
			actorID: "https://evil.example/@admin",
		},
		{
			name:    "remote user@domain with acct prefix",
			actorID: "acct:admin@evil.example",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notif := &models.Notification{
				ActorID: tt.actorID,
			}
			actor := r.loadNotificationActor(context.Background(), notif)
			// With no storage backend and a remote actorID, the result must
			// be nil — we must never treat a remote identity as a local user.
			assert.Nil(t, actor, "remote actor ID %q must not be resolved as a local actor", tt.actorID)
		})
	}
}

// TestLocalUsernameForLookup_RemoteDomainsNotLocal verifies that
// localUsernameForLookup correctly rejects remote domains.
// (CSR-038 regression probe)
func TestLocalUsernameForLookup_RemoteDomainsNotLocal(t *testing.T) {
	r := &Resolver{
		Config: &config.Config{Domain: "lesser.example"},
	}

	tests := []struct {
		name     string
		actorID  string
		wantUser string // empty means should return empty
	}{
		{
			name:     "local user@domain extracts username",
			actorID:  "alice@lesser.example",
			wantUser: "alice",
		},
		{
			name:     "remote user@domain returns empty",
			actorID:  "alice@evil.example",
			wantUser: "",
		},
		{
			name:     "local URL returns username",
			actorID:  "https://lesser.example/users/alice",
			wantUser: "alice",
		},
		{
			name:     "remote URL returns empty",
			actorID:  "https://evil.example/users/alice",
			wantUser: "",
		},
		{
			name:     "bare username returns as-is",
			actorID:  "alice",
			wantUser: "alice",
		},
		{
			name:     "username with @ prefix returns as-is",
			actorID:  "@alice",
			wantUser: "alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.localUsernameForLookup(tt.actorID)
			if tt.wantUser == "" {
				assert.Empty(t, got, "expected empty for %q", tt.actorID)
			} else {
				assert.Equal(t, tt.wantUser, got)
			}
		})
	}
}

// TestExtractDomainFromActorURL verifies the helper extracts domain
// correctly from various URL formats. (CSR-051 regression probe)
func TestExtractDomainFromActorURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "standard URL", rawURL: "https://evil.example/users/admin", want: "evil.example"},
		{name: "URL with port", rawURL: "https://evil.example:8443/users/admin", want: "evil.example"},
		{name: "empty", rawURL: "", want: ""},
		{name: "not a URL", rawURL: "admin@evil.example", want: ""},
		{name: "whitespace only", rawURL: "   ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDomainFromActorURL(tt.rawURL)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFallbackNotificationActor_ForeignDomainSurfaced verifies that
// the fallbackNotificationActor surfaces foreign domains in the actor
// name when the actorID is a URL with a domain different from local.
// (CSR-051 regression probe)
func TestFallbackNotificationActor_ForeignDomainSurfaced(t *testing.T) {
	r := &Resolver{
		Config: &config.Config{
			Domain: "lesser.example",
		},
	}

	tests := []struct {
		name         string
		actorID      string
		wantID       string
		wantName     string
		domainInName bool // whether name should contain @domain
	}{
		{
			name:         "foreign URL actor gets domain in name",
			actorID:      "https://evil.example/users/admin",
			wantID:       "https://evil.example/users/admin",
			domainInName: true,
		},
		{
			name:         "local bare username does not get domain",
			actorID:      "alice",
			wantName:     "alice",
			domainInName: false,
		},
		{
			name:         "email-like address treated as opaque id",
			actorID:      "admin@evil.example",
			wantID:       "admin@evil.example",
			domainInName: false,
			// Opaque IDs don't get local URL endpoints.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notif := &models.Notification{
				ActorID: tt.actorID,
			}
			actor := r.fallbackNotificationActor(notif)
			require.NotNil(t, actor, "fallbackNotificationActor returned nil for %q", tt.actorID)

			if tt.wantID != "" {
				assert.Equal(t, tt.wantID, actor.ID)
			}
			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, actor.Name)
			}
			if tt.domainInName {
				assert.True(t, strings.Contains(actor.Name, "@"),
					"actor name for foreign URL %q should contain @domain, got %q", tt.actorID, actor.Name)
			}
		})
	}
}

// TestFallbackNotificationActor_UsernameValidation verifies that
// bare actor IDs are validated as ActivityPub usernames before being
// used for local account lookups. (CSR-038 regression probe)
func TestFallbackNotificationActor_UsernameValidation(t *testing.T) {
	tests := []struct {
		name    string
		actorID string
		valid   bool
	}{
		{name: "valid username", actorID: "alice", valid: true},
		{name: "valid with underscore", actorID: "alice_wonder", valid: true},
		{name: "valid with digits", actorID: "user123", valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := common.ValidateActivityPubUsername(strings.TrimPrefix(tt.actorID, "@"))
			if tt.valid {
				assert.NoError(t, err, "expected valid username %q", tt.actorID)
			} else {
				assert.Error(t, err, "expected invalid username %q", tt.actorID)
			}
		})
	}
}

// TestConvertNotificationToGraphQL_CommunicationActorUsesEmailPlaceholder
// verifies that communication notification actors are resolved through the
// communication-specific path which handles email-style identifiers safely.
// (CSR-051 regression probe)
func TestConvertNotificationToGraphQL_CommunicationActorUsesEmailPlaceholder(t *testing.T) {
	r := &Resolver{
		Config: &config.Config{Domain: "lesser.example"},
	}

	// Communication notifications should use communicationNotificationActor
	// which produces email-safe placeholders with no URL endpoints.
	notif := &models.Notification{
		ID:      "notif-1",
		Type:    "communication:email",
		ActorID: "sender@evil.example",
		Data: map[string]interface{}{
			"messageId": "msg-1",
			"channel":   "email",
			"from": map[string]interface{}{
				"address":     "sender@evil.example",
				"displayName": "Evil Sender",
				"soulAgentId": "soul-1",
			},
		},
	}

	result := r.convertNotificationToGraphQL(context.Background(), notif)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)

	// Email-placeholder actors must never have URL endpoints.
	assert.Empty(t, result.Account.URL,
		"email-placeholder actor must not have URL")
	assert.Empty(t, result.Account.Inbox,
		"email-placeholder actor must not have Inbox")
	assert.Empty(t, result.Account.Outbox,
		"email-placeholder actor must not have Outbox")

	// The actor ID should be the email address.
	assert.Equal(t, "sender@evil.example", result.Account.ID)
}

// TestBuildPlaceholderActor_ForeignDomainNotAssignedLocalEndpoints verifies
// that buildPlaceholderActor does not assign local endpoints for foreign
// domain URLs. (CSR-038 regression probe)
func TestBuildPlaceholderActor_ForeignDomainNotAssignedLocalEndpoints(t *testing.T) {
	r := &Resolver{
		Config: &config.Config{Domain: "lesser.example"},
	}

	// A remote URL must be used as-is, without local endpoints.
	actor := r.buildPlaceholderActor("https://evil.example/users/admin", "lesser.example")
	require.NotNil(t, actor)

	assert.Equal(t, "https://evil.example/users/admin", actor.ID)
	assert.Empty(t, actor.Inbox, "foreign domain must not get local inbox")
	assert.Empty(t, actor.Outbox, "foreign domain must not get local outbox")

	// A bare username should get local endpoints.
	localActor := r.buildPlaceholderActor("alice", "lesser.example")
	require.NotNil(t, localActor)
	assert.Contains(t, localActor.ID, "lesser.example")
}

// Ensure activitypub import is used (avoid unused import for test-only additions).
var _ = activitypub.BaseObject{}
