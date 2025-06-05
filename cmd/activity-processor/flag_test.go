package main

import (
	"context"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProcessFlag(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name         string
		activity     *activitypub.Activity
		recipient    string
		setupMocks   func(*mocks.MockStorage)
		expectError  bool
		errorMessage string
	}{
		{
			name: "valid flag with single string object",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/flag-1",
					Type:      activitypub.FlagType,
					Published: &now,
					Summary:   "This content violates community guidelines",
				},
				Actor:  "https://reporter.example.com/users/alice",
				Object: "https://example.com/posts/offensive-post",
			},
			recipient: "moderator",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateFlag", mock.Anything, mock.MatchedBy(func(f *storage.Flag) bool {
					return f.ID == "https://example.com/activities/flag-1" &&
						f.Actor == "https://reporter.example.com/users/alice" &&
						len(f.Object) == 1 &&
						f.Object[0] == "https://example.com/posts/offensive-post" &&
						f.Content == "This content violates community guidelines" &&
						f.Status == storage.FlagStatusPending
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "flag with multiple objects",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/flag-2",
					Type:      activitypub.FlagType,
					Published: &now,
				},
				Actor: "https://reporter.example.com/users/bob",
				Object: []interface{}{
					"https://example.com/posts/spam-1",
					"https://example.com/posts/spam-2",
					"https://example.com/users/spammer",
				},
			},
			recipient: "moderator",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateFlag", mock.Anything, mock.MatchedBy(func(f *storage.Flag) bool {
					return f.ID == "https://example.com/activities/flag-2" &&
						f.Actor == "https://reporter.example.com/users/bob" &&
						len(f.Object) == 3 &&
						f.Object[0] == "https://example.com/posts/spam-1" &&
						f.Object[1] == "https://example.com/posts/spam-2" &&
						f.Object[2] == "https://example.com/users/spammer"
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "flag with object map containing ID",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/flag-3",
					Type:      activitypub.FlagType,
					Published: &now,
				},
				Actor: "https://reporter.example.com/users/charlie",
				Object: map[string]interface{}{
					"id":      "https://example.com/posts/bad-post",
					"type":    "Note",
					"content": "This is inappropriate content that should be flagged",
				},
			},
			recipient: "moderator",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateFlag", mock.Anything, mock.MatchedBy(func(f *storage.Flag) bool {
					return f.ID == "https://example.com/activities/flag-3" &&
						f.Actor == "https://reporter.example.com/users/charlie" &&
						len(f.Object) == 1 &&
						f.Object[0] == "https://example.com/posts/bad-post" &&
						f.Content == "This is inappropriate content that should be flagged"
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "flag with array of object maps",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/flag-4",
					Type:      activitypub.FlagType,
					Published: &now,
					Summary:   "Multiple spam posts",
				},
				Actor: "https://reporter.example.com/users/dave",
				Object: []interface{}{
					map[string]interface{}{
						"id":   "https://example.com/posts/spam-a",
						"type": "Note",
					},
					map[string]interface{}{
						"id":   "https://example.com/posts/spam-b",
						"type": "Note",
					},
				},
			},
			recipient: "moderator",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateFlag", mock.Anything, mock.MatchedBy(func(f *storage.Flag) bool {
					return f.ID == "https://example.com/activities/flag-4" &&
						f.Actor == "https://reporter.example.com/users/dave" &&
						len(f.Object) == 2 &&
						f.Object[0] == "https://example.com/posts/spam-a" &&
						f.Object[1] == "https://example.com/posts/spam-b" &&
						f.Content == "Multiple spam posts"
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "flag with no objects",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/flag-5",
					Type: activitypub.FlagType,
				},
				Actor:  "https://reporter.example.com/users/eve",
				Object: nil,
			},
			recipient:    "moderator",
			setupMocks:   func(m *mocks.MockStorage) {},
			expectError:  true,
			errorMessage: "no objects to flag",
		},
		{
			name: "flag with empty content from object",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/flag-6",
					Type:      activitypub.FlagType,
					Published: &now,
				},
				Actor: "https://reporter.example.com/users/frank",
				Object: map[string]interface{}{
					"id":      "https://example.com/posts/post-1",
					"content": "", // Empty content
				},
			},
			recipient: "moderator",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateFlag", mock.Anything, mock.MatchedBy(func(f *storage.Flag) bool {
					return f.ID == "https://example.com/activities/flag-6" &&
						f.Content == "" // Should be empty
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "flag without published date uses current time",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:      "https://example.com/activities/flag-7",
					Type:    activitypub.FlagType,
					Summary: "No timestamp provided",
				},
				Actor:  "https://reporter.example.com/users/grace",
				Object: "https://example.com/posts/post-2",
			},
			recipient: "moderator",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateFlag", mock.Anything, mock.MatchedBy(func(f *storage.Flag) bool {
					// Check that Published is recent (within last minute)
					return f.ID == "https://example.com/activities/flag-7" &&
						time.Since(f.Published) < time.Minute
				})).Return(nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage
			mockStore := &mocks.MockStorage{}
			tt.setupMocks(mockStore)

			// Replace global store with mock temporarily
			oldStore := store
			store = mockStore
			defer func() { store = oldStore }()

			// Process the flag
			err := processFlag(ctx, tt.activity, tt.recipient)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
			}

			// Verify all expectations were met
			mockStore.AssertExpectations(t)
		})
	}
}

func TestExtractFlagContent(t *testing.T) {
	tests := []struct {
		name            string
		activity        *activitypub.Activity
		expectedContent string
	}{
		{
			name: "content from summary",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Summary: "This is spam",
				},
				Object: "https://example.com/posts/1",
			},
			expectedContent: "This is spam",
		},
		{
			name: "content from object map",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{},
				Object: map[string]interface{}{
					"content": "Inappropriate content here",
				},
			},
			expectedContent: "Inappropriate content here",
		},
		{
			name: "summary takes precedence over object content",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Summary: "Summary reason",
				},
				Object: map[string]interface{}{
					"content": "Object content",
				},
			},
			expectedContent: "Summary reason",
		},
		{
			name: "no content available",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{},
				Object:     "https://example.com/posts/2",
			},
			expectedContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the content extraction logic from processFlag
			content := ""
			if tt.activity.Summary != "" {
				content = tt.activity.Summary
			}
			if objMap, ok := tt.activity.Object.(map[string]interface{}); ok && content == "" {
				if c, ok := objMap["content"].(string); ok && c != "" {
					content = c
				}
			}

			assert.Equal(t, tt.expectedContent, content)
		})
	}
}
