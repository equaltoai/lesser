package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of storage.Storage
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	args := m.Called(ctx, actor, privateKey)
	return args.Error(0)
}

func (m *MockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockStorage) DeleteActor(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func (m *MockStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Activity), args.Error(1)
}

func (m *MockStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

func (m *MockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

func (m *MockStorage) CreateObject(ctx context.Context, object interface{}) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) UpdateObject(ctx context.Context, object interface{}) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	args := m.Called(ctx, followerUsername, followedUsername, followActivityID)
	return args.Error(0)
}

func (m *MockStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorage) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

func TestParseActivityRecord(t *testing.T) {
	tests := []struct {
		name          string
		image         map[string]events.DynamoDBAttributeValue
		wantActivity  *activitypub.Activity
		wantDirection ActivityDirection
		wantUsername  string
		wantError     bool
	}{
		{
			name: "valid inbox activity",
			image: map[string]events.DynamoDBAttributeValue{
				"PK":     events.NewStringAttribute("ACTOR#alice"),
				"SK":     events.NewStringAttribute("ACTIVITY#2024-01-01T00:00:00Z#12345"),
				"GSI1PK": events.NewStringAttribute("INBOX#alice"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"id":     events.NewStringAttribute("https://example.com/activities/12345"),
					"type":   events.NewStringAttribute("Follow"),
					"actor":  events.NewStringAttribute("https://example.com/users/bob"),
					"object": events.NewStringAttribute("https://myserver.com/users/alice"),
				}),
			},
			wantActivity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/12345",
					Type: "Follow",
				},
				Actor:  "https://example.com/users/bob",
				Object: "https://myserver.com/users/alice",
			},
			wantDirection: ActivityDirectionInbox,
			wantUsername:  "alice",
			wantError:     false,
		},
		{
			name: "valid outbox activity",
			image: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#alice"),
				"SK": events.NewStringAttribute("ACTIVITY#2024-01-01T00:00:00Z#12345"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"id":    events.NewStringAttribute("https://myserver.com/activities/12345"),
					"type":  events.NewStringAttribute("Create"),
					"actor": events.NewStringAttribute("https://myserver.com/users/alice"),
					"object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
						"type":    events.NewStringAttribute("Note"),
						"content": events.NewStringAttribute("Hello world!"),
					}),
				}),
			},
			wantActivity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://myserver.com/activities/12345",
					Type: "Create",
				},
				Actor: "https://myserver.com/users/alice",
				Object: map[string]interface{}{
					"type":    "Note",
					"content": "Hello world!",
				},
			},
			wantDirection: ActivityDirectionOutbox,
			wantUsername:  "alice",
			wantError:     false,
		},
		{
			name: "missing PK",
			image: map[string]events.DynamoDBAttributeValue{
				"SK": events.NewStringAttribute("ACTIVITY#2024-01-01T00:00:00Z#12345"),
			},
			wantError: true,
		},
		{
			name: "not an actor record",
			image: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("OBJECT#12345"),
				"SK": events.NewStringAttribute("VERSION#1"),
			},
			wantError: true,
		},
		{
			name: "not an activity record",
			image: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#alice"),
				"SK": events.NewStringAttribute("PROFILE"),
			},
			wantError: true,
		},
		{
			name: "missing activity data",
			image: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#alice"),
				"SK": events.NewStringAttribute("ACTIVITY#2024-01-01T00:00:00Z#12345"),
			},
			wantError: true,
		},
		{
			name: "activity with array of recipients",
			image: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#alice"),
				"SK": events.NewStringAttribute("ACTIVITY#2024-01-01T00:00:00Z#12345"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"id":    events.NewStringAttribute("https://myserver.com/activities/12345"),
					"type":  events.NewStringAttribute("Create"),
					"actor": events.NewStringAttribute("https://myserver.com/users/alice"),
					"to": events.NewListAttribute([]events.DynamoDBAttributeValue{
						events.NewStringAttribute("https://example.com/users/bob"),
						events.NewStringAttribute(activitypub.PublicAddress),
					}),
					"object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
						"type":    events.NewStringAttribute("Note"),
						"content": events.NewStringAttribute("Hello world!"),
					}),
				}),
			},
			wantActivity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://myserver.com/activities/12345",
					Type: "Create",
					To:   []string{"https://example.com/users/bob", activitypub.PublicAddress},
				},
				Actor: "https://myserver.com/users/alice",
				Object: map[string]interface{}{
					"type":    "Note",
					"content": "Hello world!",
				},
			},
			wantDirection: ActivityDirectionOutbox,
			wantUsername:  "alice",
			wantError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity, direction, username, err := parseActivityRecord(tt.image)

			if tt.wantError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantUsername, username)
			assert.Equal(t, tt.wantDirection, direction)

			// Compare activities
			if tt.wantActivity != nil {
				assert.Equal(t, tt.wantActivity.ID, activity.ID)
				assert.Equal(t, tt.wantActivity.Type, activity.Type)
				assert.Equal(t, tt.wantActivity.Actor, activity.Actor)

				// Special handling for To field which comes as []interface{} from DynamoDB
				if activity.To != nil && tt.wantActivity.To != nil {
					// The parsed activity might have []interface{}, so we compare lengths
					assert.Equal(t, len(tt.wantActivity.To), len(activity.To))
				}
			}
		})
	}
}

func TestExtractAllRecipients(t *testing.T) {
	tests := []struct {
		name           string
		activity       *activitypub.Activity
		wantRecipients []string
	}{
		{
			name: "single recipient in To",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{"https://example.com/users/bob"},
				},
			},
			wantRecipients: []string{"https://example.com/users/bob"},
		},
		{
			name: "multiple fields with public address",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{"https://example.com/users/bob", activitypub.PublicAddress},
					CC: []string{"https://example.com/users/charlie"},
				},
			},
			wantRecipients: []string{"https://example.com/users/bob", "https://example.com/users/charlie"},
		},
		{
			name: "skip local users",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{"https://example.com/users/bob", "/users/alice"},
				},
			},
			wantRecipients: []string{"https://example.com/users/bob"},
		},
		{
			name: "interface{} recipients",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{"https://example.com/users/bob", "https://example.com/users/charlie"},
				},
			},
			wantRecipients: []string{"https://example.com/users/bob", "https://example.com/users/charlie"},
		},
		{
			name: "all addressing fields",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To:  []string{"https://example.com/users/bob"},
					CC:  []string{"https://example.com/users/charlie"},
					BTo: []string{"https://example.com/users/dave"},
					BCC: []string{"https://example.com/users/eve"},
				},
			},
			wantRecipients: []string{
				"https://example.com/users/bob",
				"https://example.com/users/charlie",
				"https://example.com/users/dave",
				"https://example.com/users/eve",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipients := extractAllRecipients(tt.activity)

			// Convert to map for order-independent comparison
			gotMap := make(map[string]bool)
			for _, r := range recipients {
				gotMap[r] = true
			}

			wantMap := make(map[string]bool)
			for _, r := range tt.wantRecipients {
				wantMap[r] = true
			}

			assert.Equal(t, len(wantMap), len(gotMap))
			for k := range wantMap {
				assert.True(t, gotMap[k], "missing recipient: %s", k)
			}
		})
	}
}

func TestProcessInboxActivity(t *testing.T) {
	// Set up mock storage
	mockStore := &MockStorage{}
	originalStore := store
	store = mockStore
	defer func() { store = originalStore }()

	ctx := context.Background()

	tests := []struct {
		name              string
		activity          *activitypub.Activity
		recipientUsername string
		setupMocks        func()
		wantError         bool
	}{
		{
			name: "process follow activity",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/follow-1",
					Type: activitypub.FollowType,
				},
				Actor:  "https://example.com/users/bob",
				Object: "https://myserver.com/users/alice",
			},
			recipientUsername: "alice",
			setupMocks: func() {
				mockStore.On("CreateFollow", ctx, "bob", "alice", "https://example.com/activities/follow-1").
					Return(nil).Once()
			},
			wantError: false,
		},
		{
			name: "process accept follow activity",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/accept-1",
					Type: activitypub.AcceptType,
				},
				Actor: "https://example.com/users/bob",
				Object: map[string]interface{}{
					"type":   activitypub.FollowType,
					"actor":  "https://myserver.com/users/alice",
					"object": "https://example.com/users/bob",
				},
			},
			recipientUsername: "bob",
			setupMocks: func() {
				mockStore.On("AcceptFollow", ctx, "alice", "bob").
					Return(nil).Once()
			},
			wantError: false,
		},
		{
			name: "process create activity",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/create-1",
					Type: activitypub.CreateType,
				},
				Actor: "https://example.com/users/bob",
				Object: map[string]interface{}{
					"type":    "Note",
					"content": "Hello!",
				},
			},
			recipientUsername: "alice",
			setupMocks: func() {
				mockStore.On("CreateObject", ctx, map[string]interface{}{
					"type":    "Note",
					"content": "Hello!",
				}).Return(nil).Once()
			},
			wantError: false,
		},
		{
			name: "process like activity",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/like-1",
					Type: activitypub.LikeType,
				},
				Actor:  "https://example.com/users/bob",
				Object: "https://myserver.com/objects/note-1",
			},
			recipientUsername: "alice",
			setupMocks: func() {
				// Like storage not implemented yet
			},
			wantError: false,
		},
		{
			name: "unknown activity type",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/unknown-1",
					Type: "UnknownType",
				},
				Actor:  "https://example.com/users/bob",
				Object: "https://myserver.com/users/alice",
			},
			recipientUsername: "alice",
			setupMocks:        func() {},
			wantError:         false, // Unknown types are logged but don't error
		},
		{
			name: "follow with invalid actor",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/follow-2",
					Type: activitypub.FollowType,
				},
				Actor:  "invalid-actor-id",
				Object: "https://myserver.com/users/alice",
			},
			recipientUsername: "alice",
			setupMocks:        func() {},
			wantError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore.ExpectedCalls = nil
			mockStore.Calls = nil

			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			err := processInboxActivity(ctx, tt.activity, tt.recipientUsername)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestProcessOutboxActivity(t *testing.T) {
	// Create a test server to simulate remote inboxes
	deliveryCount := 0
	var serverURL string
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/bob":
			// Return actor profile
			actor := &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   serverURL + "/users/bob",
					Type: activitypub.PersonType,
				},
				PreferredUsername: "bob",
				Inbox:             serverURL + "/users/bob/inbox",
			}
			json.NewEncoder(w).Encode(actor)
		case "/users/bob/inbox":
			// Accept activity delivery
			deliveryCount++
			// Debug logging
			t.Logf("Inbox request: Method=%s, Content-Type=%s, Has-Signature=%v, Has-Date=%v, Has-Digest=%v",
				r.Method, r.Header.Get("Content-Type"),
				r.Header.Get("Signature") != "",
				r.Header.Get("Date") != "",
				r.Header.Get("Digest") != "")

			// Verify required headers are present
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Content-Type") != "application/activity+json" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// Just check that signature headers are present, don't verify them in tests
			if r.Header.Get("Signature") == "" || r.Header.Get("Date") == "" || r.Header.Get("Digest") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer testServer.Close()
	serverURL = testServer.URL

	// Set up mock storage
	mockStore := &MockStorage{}
	originalStore := store
	store = mockStore
	defer func() { store = originalStore }()

	// Override HTTP client
	originalClient := httpClient
	httpClient = testServer.Client()
	defer func() { httpClient = originalClient }()

	ctx := context.Background()

	tests := []struct {
		name       string
		activity   *activitypub.Activity
		setupMocks func()
		wantError  bool
	}{
		{
			name: "deliver to single recipient",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://myserver.com/activities/12345",
					Type: activitypub.CreateType,
					To:   []string{testServer.URL + "/users/bob"},
				},
				Actor: "https://myserver.com/users/alice",
				Object: map[string]interface{}{
					"type":    "Note",
					"content": "Hello!",
				},
			},
			setupMocks: func() {
				mockStore.On("GetActorPrivateKey", ctx, "alice").
					Return(testPrivateKeyPEM, nil).Once()
			},
			wantError: false,
		},
		{
			name: "skip public address",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://myserver.com/activities/12345",
					Type: activitypub.CreateType,
					To:   []string{activitypub.PublicAddress},
				},
				Actor: "https://myserver.com/users/alice",
				Object: map[string]interface{}{
					"type":    "Note",
					"content": "Hello!",
				},
			},
			setupMocks: func() {},
			wantError:  false,
		},
		{
			name: "no recipients",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://myserver.com/activities/12345",
					Type: activitypub.CreateType,
				},
				Actor: "https://myserver.com/users/alice",
				Object: map[string]interface{}{
					"type":    "Note",
					"content": "Hello!",
				},
			},
			setupMocks: func() {},
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore.ExpectedCalls = nil
			mockStore.Calls = nil
			deliveryCount = 0

			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			err := processOutboxActivity(ctx, tt.activity)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandler(t *testing.T) {
	// Set up mock storage
	mockStore := &MockStorage{}
	originalStore := store
	store = mockStore
	defer func() { store = originalStore }()

	// Disable logging for tests
	originalLogger := logger
	logger = zap.NewNop()
	defer func() { logger = originalLogger }()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		event      events.DynamoDBEvent
		setupMocks func()
		wantError  bool
	}{
		{
			name: "process INSERT event",
			event: events.DynamoDBEvent{
				Records: []events.DynamoDBEventRecord{
					{
						EventName: "INSERT",
						Change: events.DynamoDBStreamRecord{
							NewImage: map[string]events.DynamoDBAttributeValue{
								"PK":     events.NewStringAttribute("ACTOR#alice"),
								"SK":     events.NewStringAttribute("ACTIVITY#" + now.Format(time.RFC3339Nano) + "#12345"),
								"GSI1PK": events.NewStringAttribute("INBOX#alice"),
								"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
									"id":     events.NewStringAttribute("https://example.com/activities/12345"),
									"type":   events.NewStringAttribute("Follow"),
									"actor":  events.NewStringAttribute("https://example.com/users/bob"),
									"object": events.NewStringAttribute("https://myserver.com/users/alice"),
								}),
							},
						},
					},
				},
			},
			setupMocks: func() {
				mockStore.On("CreateFollow", ctx, "bob", "alice", "https://example.com/activities/12345").
					Return(nil).Once()
			},
			wantError: false,
		},
		{
			name: "skip REMOVE event",
			event: events.DynamoDBEvent{
				Records: []events.DynamoDBEventRecord{
					{
						EventName: "REMOVE",
						Change: events.DynamoDBStreamRecord{
							OldImage: map[string]events.DynamoDBAttributeValue{
								"PK": events.NewStringAttribute("ACTOR#alice"),
								"SK": events.NewStringAttribute("ACTIVITY#2024-01-01T00:00:00Z#12345"),
							},
						},
					},
				},
			},
			setupMocks: func() {},
			wantError:  false,
		},
		{
			name: "handle parse error gracefully",
			event: events.DynamoDBEvent{
				Records: []events.DynamoDBEventRecord{
					{
						EventName: "INSERT",
						Change: events.DynamoDBStreamRecord{
							NewImage: map[string]events.DynamoDBAttributeValue{
								"PK": events.NewStringAttribute("INVALID"),
							},
						},
					},
				},
			},
			setupMocks: func() {},
			wantError:  false, // Should continue processing despite individual errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore.ExpectedCalls = nil
			mockStore.Calls = nil

			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			err := handler(ctx, tt.event)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

// Test key for signing
const testPrivateKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAqoPfYe5t6VrVWBo4jrpVL+DB43NqLmMyNMQKHHIzP2rJY5Wy
ux6/yiDd3u8UgPxtVMKyMKLvyjG3f9Sje4eNVwWWuLpExl4kP7ofKDXLZ3nS/pRf
BH3Sk3DpEFhInfBm0fuEPPYLp1zj+on8YndiHZlHsXAGmXYhrI52h63YV0OGPsFD
qrjn80coNjLzYWOU3E/mR4EQ3gKz07SSZ2YJb32o7OZpUB26KGSEQWnu98i90PaJ
SaDg3GTPegKSKMukWV3SRMYbO4nUt1hDC3e01MiYSi6MPOo7dHDxoBZsgDeoup6O
XwGv0pgCbt+mGcvzwMosAi6a63upLRH2Fx/t8wIDAQABAoIBADjDh8S5M9vAQk9/
Ax74hs1WfBU04b8phJguPtNzbP4KlZpSRlqmhOBMCrBhVKkP33GdEubAByV/YX/r
kLTZzkKO+LrsP2LuChEw65heOCVtV8EqMWt0W3p71wp66UmysvfqS/5jRkPj130b
HGrHJWGHGmfGTFwgCFvCXVETnXaG1/kXvoYXDSwz0cXMFykbEgV3KLVcn2TTGNga
uqZQ3wgECL0J9AxTOFGufcOx5K5YCCtuIbYC6h5Zgxd4DSpdoYWSnoUjyQ0HlEhk
11cRKN4BgKKfv5B5+Hxqigcf/VOCnit+NfKgOSWpbEXawqQ00+n0iAJWZJi+BYE5
O/+G3WkCgYEA0iVp/3AJyyecfRupW32I7ZPzBQ+KQB4BFPq4K6ujqzoXy8960Dwi
Icu3PeOPNFbY5QdEUlrhO4nsp9vA+BUeVNYYQPoW6njFbXgPOiH+ryOpCdvXifPQ
Pp1ksguKb92qxOfqUL4TPru+iRE0/oeQLoo6PDJOs1G1qhfw75vAje0CgYEAz7i0
By3pJUa4jMmfIyuCwLczI9aFd9klfsC5oEy88V2ZA9YO1AO3Y8iDpjUeOunR6E52
ZKV/cp3XSeGwkAQVvoaFOSYkJAUXiOmLMa1Iicp83n2K5/OvBW+93zmPuYXEgD9P
KL58vgS8wAZLi4EDpTeW8T6+grXeeHeyiSF3718CgYEAupWnqLqEp5GTG24NEAPF
KRR86RhkKwu48DSwg23RUz2wVTDyHaPWtmUXXOcIhnM5/xhVrD2uz9tleaDflCXE
GZVCUab749G5kbnQ40+9vymNdAhzNrR5SK8c8gzXLP4HGu/Dl0887S1rPm49vGUH
OptWm44bXJIHF3BMZ6LF8/0CgYAPXAsD1OM+fGI9FtOLmDYM5f8EEWLBH+9j1gBj
2AjImDEuVW+3QacX28XQTnEzzgJVeSfL/WjVItK+hc+2dnbdJblIJofZmf7Jgutl
+vg4KB7fnMzepeg1MLQLg4gbIccL7KJ/0sYKjvMeB9kiMaIBX4DrycXwiF4w7jjn
ZvAlZQKBgQDBz2xa0LGFJnXcLiUlvYeA4JBpj8sMaFs7QO3YeOighsxdNGhQJL2O
phuAIYp2mUSu5Z9Q45JLKfWH6MqIjfeq4PNx79ohuZm+rjk2x6UdmM83NEtiQewy
PGAiTkqQWDatyB+DePM+sG9QD83zAWat0G7+rJIwvDG7+WMnHa0JFg==
-----END RSA PRIVATE KEY-----`
