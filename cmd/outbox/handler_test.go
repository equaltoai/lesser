package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of storage.Storage
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// Implement other required methods as stubs
func (m *MockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	return nil
}
func (m *MockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return "", nil
}
func (m *MockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	return nil
}
func (m *MockStorage) DeleteActor(ctx context.Context, username string) error {
	return nil
}
func (m *MockStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	return nil, nil
}
func (m *MockStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	return nil, "", nil
}
func (m *MockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	return nil, "", nil
}
func (m *MockStorage) CreateObject(ctx context.Context, object interface{}) error {
	return nil
}
func (m *MockStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	return nil, nil
}
func (m *MockStorage) UpdateObject(ctx context.Context, object interface{}) error {
	return nil
}
func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	return nil
}
func (m *MockStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	return nil
}
func (m *MockStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	return nil
}
func (m *MockStorage) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	return nil
}
func (m *MockStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	return nil
}
func (m *MockStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return nil, "", nil
}
func (m *MockStorage) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return nil, "", nil
}
func (m *MockStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	return false, nil
}
func (m *MockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	return nil, nil
}

func TestHandler(t *testing.T) {
	// Initialize logger for tests
	logger = zap.NewNop()

	tests := []struct {
		name           string
		method         string
		username       string
		body           string
		setupMocks     func(*MockStorage)
		expectedStatus int
		expectedError  bool
		validateBody   func(*testing.T, string)
	}{
		{
			name:     "successful activity creation",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Follow",
				"actor": "https://example.com/users/alice",
				"object": "https://remote.example/users/bob"
			}`,
			setupMocks: func(m *MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Inbox:             "https://example.com/users/alice/inbox",
					Outbox:            "https://example.com/users/alice/outbox",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Follow", activity.Type)
				assert.Equal(t, "https://example.com/users/alice", activity.Actor)
				assert.NotEmpty(t, activity.ID)
			},
		},
		{
			name:     "activity with auto-generated ID",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Like",
				"actor": "https://example.com/users/alice",
				"object": "https://remote.example/notes/123"
			}`,
			setupMocks: func(m *MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.NotEmpty(t, activity.ID)
				assert.Contains(t, activity.ID, "https://example.com/activities/")
			},
		},
		{
			name:     "activity with auto-filled actor",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Hello world!"
				}
			}`,
			setupMocks: func(m *MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "https://example.com/users/alice", activity.Actor)
			},
		},
		{
			name:     "method not allowed",
			method:   http.MethodGet,
			username: "alice",
			setupMocks: func(m *MockStorage) {
				// No mocks needed
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "missing username",
			method: http.MethodPost,
			body:   `{"type": "Follow"}`,
			setupMocks: func(m *MockStorage) {
				// No mocks needed
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "actor not found",
			method:   http.MethodPost,
			username: "unknown",
			body:     `{"type": "Follow"}`,
			setupMocks: func(m *MockStorage) {
				m.On("GetActor", mock.Anything, "unknown").Return(nil, common.ActorNotFoundError{Username: "unknown"})
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:     "invalid JSON",
			method:   http.MethodPost,
			username: "alice",
			body:     `{invalid json`,
			setupMocks: func(m *MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "actor mismatch",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Follow",
				"actor": "https://example.com/users/bob",
				"object": "https://remote.example/users/charlie"
			}`,
			setupMocks: func(m *MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "storage error",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Follow",
				"actor": "https://example.com/users/alice",
				"object": "https://remote.example/users/bob"
			}`,
			setupMocks: func(m *MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(fmt.Errorf("storage error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage
			mockStore := new(MockStorage)
			tt.setupMocks(mockStore)
			store = mockStore

			// Create request
			request := events.APIGatewayProxyRequest{
				HTTPMethod: tt.method,
				Body:       tt.body,
				Headers:    map[string]string{"Content-Type": "application/activity+json"},
				PathParameters: map[string]string{
					"username": tt.username,
				},
			}

			// Call handler
			response, err := handler(context.Background(), request)

			// Verify
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, response.StatusCode)

				if tt.validateBody != nil && response.StatusCode == http.StatusCreated {
					tt.validateBody(t, response.Body)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestGenerateActivityID(t *testing.T) {
	tests := []struct {
		name         string
		actorID      string
		activityType string
		validateID   func(*testing.T, string)
	}{
		{
			name:         "standard actor ID",
			actorID:      "https://example.com/users/alice",
			activityType: "Follow",
			validateID: func(t *testing.T, id string) {
				assert.Contains(t, id, "https://example.com/activities/")
				assert.NotContains(t, id, "/users/")
			},
		},
		{
			name:         "non-standard actor ID",
			actorID:      "https://example.com/actors/alice",
			activityType: "Like",
			validateID: func(t *testing.T, id string) {
				assert.Contains(t, id, "https://example.com/actors/alice/activities/")
			},
		},
		{
			name:         "actor ID with port",
			actorID:      "https://example.com:8080/users/alice",
			activityType: "Create",
			validateID: func(t *testing.T, id string) {
				assert.Contains(t, id, "https://example.com:8080/activities/")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateActivityID(tt.actorID, tt.activityType)
			assert.NotEmpty(t, id)
			tt.validateID(t, id)
		})
	}
}

func TestGenerateRandomString(t *testing.T) {
	// Test that it generates strings of correct length
	lengths := []int{4, 8, 16}
	for _, length := range lengths {
		result := generateRandomString(length)
		assert.Equal(t, length, len(result))
		// Verify it only contains allowed characters
		for _, char := range result {
			assert.Contains(t, "abcdefghijklmnopqrstuvwxyz0123456789", string(char))
		}
	}
}
