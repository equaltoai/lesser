package graph

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestExtractUsernameFromActorIdentifier(t *testing.T) {
	testCases := map[string]string{
		"admin":                  "admin",
		"@member":                "member",
		"member@dev.lesser.host": "member",
		"https://remote.example/users/follow_bot":   "follow_bot",
		"https://remote.example/@federated-account": "federated-account",
		"https://remote.example":                    "remote.example",
		"":                                          "",
		"https://remote.example/users/@complexUser": "complexUser",
		"https://remote.example/users/nested/path/": "path",
	}

	for input, expected := range testCases {
		if actual := extractUsernameFromActorIdentifier(input); actual != expected {
			t.Fatalf("extractUsernameFromActorIdentifier(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestFallbackNotificationActorLocalUser(t *testing.T) {
	resolver := &Resolver{
		Config: &config.Config{Domain: "dev.lesser.host"},
	}

	notification := &models.Notification{
		ActorID: "admin",
	}

	actor := resolver.fallbackNotificationActor(notification)
	if actor == nil {
		t.Fatal("expected fallback actor, got nil")
	}

	expectedID := "https://dev.lesser.host/users/admin"
	if actor.ID != expectedID {
		t.Fatalf("expected actor ID %q, got %q", expectedID, actor.ID)
	}
	if actor.PreferredUsername != "admin" {
		t.Fatalf("expected preferred username \"admin\", got %q", actor.PreferredUsername)
	}
	if actor.Inbox != "https://dev.lesser.host/users/admin/inbox" {
		t.Fatalf("expected inbox to be populated for local actor, got %q", actor.Inbox)
	}
}

func TestFallbackNotificationActorRemoteUser(t *testing.T) {
	resolver := &Resolver{
		Config: &config.Config{Domain: "dev.lesser.host"},
	}

	rawID := "https://remote.example/users/visitor"
	notification := &models.Notification{
		ActorID: rawID,
	}

	actor := resolver.fallbackNotificationActor(notification)
	if actor == nil {
		t.Fatal("expected fallback actor, got nil")
	}

	if actor.ID != rawID {
		t.Fatalf("expected actor ID %q, got %q", rawID, actor.ID)
	}
	if actor.PreferredUsername != "visitor" {
		t.Fatalf("expected preferred username \"visitor\", got %q", actor.PreferredUsername)
	}
	if actor.URL != rawID {
		t.Fatalf("expected actor URL %q, got %q", rawID, actor.URL)
	}
	if actor.Inbox != "" || actor.Outbox != "" {
		t.Fatalf("expected remote actor endpoints to be empty, got inbox=%q outbox=%q", actor.Inbox, actor.Outbox)
	}
}
