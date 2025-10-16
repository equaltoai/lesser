package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
)

// Basic tests for hashtag mutation resolvers
// More comprehensive integration tests are in the tests/ directory

func TestHashtagMutationResolver_BasicStructure(t *testing.T) {
	// Test that the resolvers exist and have correct signatures
	// This is a compile-time check - if this compiles, the resolvers are correctly structured
	var r mutationResolver
	ctx := context.Background()

	t.Run("FollowHashtag resolver exists", func(t *testing.T) {
		// This will panic without proper setup, but we're just checking structure
		defer func() {
			if r := recover(); r != nil {
				// Expected - we don't have a full resolver setup
			}
		}()
		_, _ = r.FollowHashtag(ctx, "golang", nil)
	})

	t.Run("UnfollowHashtag resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		_, _ = r.UnfollowHashtag(ctx, "golang")
	})

	t.Run("UpdateHashtagNotifications resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		settings := model.HashtagNotificationSettingsInput{
			Level: model.NotificationLevelAll,
		}
		_, _ = r.UpdateHashtagNotifications(ctx, "golang", settings)
	})

	t.Run("MuteHashtag resolver exists", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		_, _ = r.MuteHashtag(ctx, "golang", nil)
	})
}

func TestConvertInputToSettingsModel(t *testing.T) {
	// Test the settings converter helper
	r := &mutationResolver{}

	input := model.HashtagNotificationSettingsInput{
		Level: model.NotificationLevelAll,
		Filters: []*model.NotificationFilterInput{
			{
				Type:  "reply",
				Value: "boost",
			},
		},
	}

	result := r.convertInputToSettingsModel(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Level != model.NotificationLevelAll {
		t.Errorf("expected Level to be %v, got %v", model.NotificationLevelAll, result.Level)
	}

	if result.Muted != false {
		t.Errorf("expected Muted to be false, got %v", result.Muted)
	}

	if len(result.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(result.Filters))
	}

	if len(result.Filters) > 0 {
		if result.Filters[0].Type != "reply" {
			t.Errorf("expected filter Type to be 'reply', got %v", result.Filters[0].Type)
		}
		if result.Filters[0].Value != "boost" {
			t.Errorf("expected filter Value to be 'boost', got %v", result.Filters[0].Value)
		}
	}
}

// Additional integration tests with full mock setup should be added
// in the tests/graphql directory
