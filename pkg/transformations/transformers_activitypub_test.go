package transformations

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

func TestActivityPubActorToStorage(t *testing.T) {
	// Create a sample ActivityPub actor
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/testuser",
			Type: "Person",
		},
		PreferredUsername: "testuser",
		Name:              "Test User",
		Summary:           "A test user for unit tests",
		URL:               "https://example.com/@testuser",
		Discoverable:      true,
	}

	// Convert to storage model
	storageActor, err := ActivityPubActorToStorage(actor)
	if err != nil {
		t.Fatalf("Failed to convert actor to storage: %v", err)
	}

	// Verify the conversion
	if storageActor.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", storageActor.Username)
	}
	if storageActor.PK != "ACTOR#testuser" {
		t.Errorf("Expected PK 'ACTOR#testuser', got '%s'", storageActor.PK)
	}
	if storageActor.SK != "PROFILE" {
		t.Errorf("Expected SK 'PROFILE', got '%s'", storageActor.SK)
	}
	if storageActor.Actor != actor {
		t.Error("Expected stored Actor to be the same reference")
	}
}

func TestStorageActorToActivityPub(t *testing.T) {
	now := time.Now()
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/testuser",
			Type: "Person",
		},
		PreferredUsername: "testuser",
		Name:              "Test User",
	}

	// Create a storage actor
	storageActor, err := ActivityPubActorToStorage(actor)
	if err != nil {
		t.Fatalf("Failed to create storage actor: %v", err)
	}
	
	storageActor.Actor = actor
	storageActor.CreatedAt = now

	// Convert back to ActivityPub
	resultActor, err := StorageActorToActivityPub(storageActor)
	if err != nil {
		t.Fatalf("Failed to convert storage to actor: %v", err)
	}

	// Verify the conversion
	if resultActor.PreferredUsername != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", resultActor.PreferredUsername)
	}
	if resultActor.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", resultActor.Name)
	}
}

func TestActivityPubActorToMastodon(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/testuser",
			Type: "Person",
		},
		PreferredUsername: "testuser",
		Name:              "Test User",
		Summary:           "A test user",
		URL:               "https://example.com/@testuser",
		Discoverable:      true,
	}

	baseURL := "https://example.com"
	account, err := ActivityPubActorToMastodon(actor, baseURL)
	if err != nil {
		t.Fatalf("Failed to convert actor to Mastodon: %v", err)
	}

	// Verify the conversion
	if account.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", account.Username)
	}
	if account.DisplayName != "Test User" {
		t.Errorf("Expected display name 'Test User', got '%s'", account.DisplayName)
	}
	if account.Note != "A test user" {
		t.Errorf("Expected note 'A test user', got '%s'", account.Note)
	}
	if !account.Discoverable {
		t.Error("Expected account to be discoverable")
	}
	if account.Bot {
		t.Error("Expected account not to be a bot")
	}
}

func TestTransformationRegistry(t *testing.T) {
	// Test that the registry is initialized
	if ActivityPubRegistry == nil {
		t.Fatal("ActivityPubRegistry should be initialized")
	}

	// Test that transformers are registered
	transformers := ActivityPubRegistry.List()
	expectedTransformers := []string{
		"actor_to_storage",
		"storage_to_actor", 
		"actor_to_mastodon",
		"object_to_storage",
		"storage_to_object",
		"object_to_mastodon",
		"activity_to_storage",
		"storage_to_activity",
		"actors_to_mastodon_batch",
		"objects_to_mastodon_batch",
	}

	for _, expected := range expectedTransformers {
		found := false
		for _, registered := range transformers {
			if registered == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected transformer '%s' to be registered", expected)
		}
	}
}

func TestActorTransformerFromRegistry(t *testing.T) {
	// Get transformer from registry
	transformerInterface, exists := ActivityPubRegistry.Get("actor_to_storage")
	if !exists {
		t.Fatal("actor_to_storage transformer not found in registry")
	}

	transformer, ok := transformerInterface.(common.Transformer[*activitypub.Actor, *storagemodels.Actor])
	if !ok {
		t.Fatal("transformer has wrong type")
	}

	// Create test actor
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/testuser",
			Type: "Person",
		},
		PreferredUsername: "testuser",
		Name:              "Test User",
	}

	// Use transformer
	ctx := context.Background()
	storageActor, err := transformer.Transform(ctx, actor)
	if err != nil {
		t.Fatalf("Failed to transform actor: %v", err)
	}

	// Verify result
	if storageActor.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", storageActor.Username)
	}
}

func TestValidationErrors(t *testing.T) {
	// Test nil actor
	_, err := ActivityPubActorToStorage(nil)
	if err == nil {
		t.Error("Expected error for nil actor")
	}
	
	validationErr, ok := err.(common.ValidationError)
	if !ok {
		t.Error("Expected ValidationError")
	} else if validationErr.Field != "actor" {
		t.Errorf("Expected field 'actor', got '%s'", validationErr.Field)
	}

	// Test empty username
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/testuser",
			Type: "Person",
		},
		PreferredUsername: "", // Empty username
	}

	_, err = ActivityPubActorToStorage(actor)
	if err == nil {
		t.Error("Expected error for empty username")
	}

	validationErr, ok = err.(common.ValidationError)
	if !ok {
		t.Error("Expected ValidationError")
	} else if validationErr.Field != "preferredUsername" {
		t.Errorf("Expected field 'preferredUsername', got '%s'", validationErr.Field)
	}
}