package main

import (
	"testing"

	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	lifttesting "github.com/pay-theory/lift/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler(t *testing.T) {
	// Create test handler
	handler, err := NewInboxHandler()
	require.NoError(t, err)

	t.Run("successful activity delivery", func(t *testing.T) {
		// Setup mock storage
		mockStore := new(mocks.MockStorage)
		handler.store = mockStore

		// Create test app
		app := lifttesting.NewTestApp()
		app.App().POST("/inbox/:username", handler.handlePostInbox)

		// Create test actor
		recipient := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
			Inbox:             "https://example.com/users/alice/inbox",
		}

		// Create test activity
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/activities/123",
				Type: activitypub.FollowType,
				To:   []string{recipient.ID},
			},
			Actor:  "https://remote.example/users/bob",
			Object: recipient.ID,
		}

		// Setup mock expectations
		mockStore.On("GetActor", mock.Anything, "alice").Return(recipient, nil)
		mockStore.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
		mockStore.On("RecordActivity", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)

		// Execute test
		response := app.
			WithHeader("Content-Type", "application/activity+json").
			POST("/inbox/alice", activity)

		// Assert
		assert.Equal(t, 202, response.StatusCode) // StatusAccepted

		// Verify mock expectations
		mockStore.AssertExpectations(t)
	})

	t.Run("actor not found", func(t *testing.T) {
		// Setup mock storage
		mockStore := new(mocks.MockStorage)
		handler.store = mockStore

		// Create test app
		app := lifttesting.NewTestApp()
		app.App().POST("/inbox/:username", handler.handlePostInbox)

		// Setup mock to return actor not found
		mockStore.On("GetActor", mock.Anything, "unknown").Return(nil, common.ActorNotFoundError{Username: "unknown"})

		// Execute test
		response := app.
			WithHeader("Content-Type", "application/activity+json").
			POST("/inbox/unknown", map[string]interface{}{"id": "test", "type": "Follow"})

		// Assert
		assert.Equal(t, 404, response.StatusCode)

		// Verify mock expectations
		mockStore.AssertExpectations(t)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// Setup mock storage
		mockStore := new(mocks.MockStorage)
		handler.store = mockStore

		// Create test app
		app := lifttesting.NewTestApp()
		app.App().POST("/inbox/:username", handler.handlePostInbox)

		recipient := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
		}

		mockStore.On("GetActor", mock.Anything, "alice").Return(recipient, nil)

		// Execute test with invalid JSON (sending string instead of JSON)
		response := app.
			WithHeader("Content-Type", "application/activity+json").
			POST("/inbox/alice", "invalid json")

		// Assert
		assert.Equal(t, 400, response.StatusCode)

		// Verify mock expectations
		mockStore.AssertExpectations(t)
	})
}

func TestIsAddressedTo(t *testing.T) {
	// Create test handler
	handler, err := NewInboxHandler()
	require.NoError(t, err)

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		Inbox: "https://example.com/users/alice/inbox",
	}

	tests := []struct {
		name     string
		activity *activitypub.Activity
		expected bool
	}{
		{
			name: "addressed in to field by actor ID",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{actor.ID},
				},
			},
			expected: true,
		},
		{
			name: "addressed in to field by inbox URL",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{actor.Inbox},
				},
			},
			expected: true,
		},
		{
			name: "addressed to public",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{activitypub.PublicAddress},
				},
			},
			expected: true,
		},
		{
			name: "not addressed to actor",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{"https://example.com/users/bob"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isAddressedTo(tt.activity, actor)
			assert.Equal(t, tt.expected, result)
		})
	}
}
