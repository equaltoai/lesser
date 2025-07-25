package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of storage.Storage
type MockStorage struct {
	mock.Mock
	mocks.BaseMockStorage
}

// Override only the methods that need mock expectations in these tests
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
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

func (m *MockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	return nil, "", nil
}

func (m *MockStorage) CreateObject(ctx context.Context, object any) error {
	return nil
}

func (m *MockStorage) GetObject(ctx context.Context, id string) (any, error) {
	return nil, nil
}

func (m *MockStorage) UpdateObject(ctx context.Context, object any) error {
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
	return nil, nil
}

// OAuth-related methods
func (m *MockStorage) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	return nil
}

func (m *MockStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	return nil, nil
}

func (m *MockStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	return nil
}

func (m *MockStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	return nil
}

func (m *MockStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	return nil, nil
}

func (m *MockStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	return nil, "", nil
}

// User-related methods
func (m *MockStorage) CreateUser(ctx context.Context, user *storage.User) error {
	return nil
}

func (m *MockStorage) GetUser(ctx context.Context, username string) (*storage.User, error) {
	return nil, nil
}

func (m *MockStorage) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	return nil, nil
}

func (m *MockStorage) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	return nil
}

func (m *MockStorage) DeleteUser(ctx context.Context, username string) error {
	return nil
}

func (m *MockStorage) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	return nil, "", nil
}

// OAuth Client methods
func (m *MockStorage) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	return nil
}

func (m *MockStorage) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	return nil, nil
}

func (m *MockStorage) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	return nil
}

func (m *MockStorage) DeleteOAuthClient(ctx context.Context, clientID string) error {
	return nil
}

func (m *MockStorage) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	return nil, "", nil
}

// Like methods
func (m *MockStorage) CreateLike(ctx context.Context, like *storage.Like) error {
	args := m.Called(ctx, like)
	return args.Error(0)
}

func (m *MockStorage) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Like), args.Error(1)
}

func (m *MockStorage) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockStorage) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Like), args.String(1), args.Error(2)
}

func (m *MockStorage) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Like), args.String(1), args.Error(2)
}

func (m *MockStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Announce methods
func (m *MockStorage) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

func (m *MockStorage) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announce), args.Error(1)
}

func (m *MockStorage) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockStorage) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

func (m *MockStorage) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

func (m *MockStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Tombstone methods
func (m *MockStorage) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

func (m *MockStorage) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Tombstone), args.Error(1)
}

func (m *MockStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Update history methods
func (m *MockStorage) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

func (m *MockStorage) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	args := m.Called(ctx, objectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.UpdateHistory), args.Error(1)
}

// Block methods
func (m *MockStorage) CreateBlock(ctx context.Context, block *storage.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorage) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	args := m.Called(ctx, actor, blockedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Block), args.Error(1)
}

func (m *MockStorage) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	args := m.Called(ctx, actor, blockedActor)
	return args.Error(0)
}

func (m *MockStorage) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

func (m *MockStorage) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

func (m *MockStorage) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	args := m.Called(ctx, actor1, actor2)
	return args.Bool(0), args.Error(1)
}

// Helper function to create test JWT token
func createTestAuthHeader(username string) map[string]string {
	// Create JWT token directly since generateAccessToken is not exported
	jwtSecret := []byte("test-secret")
	now := time.Now()

	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(now),
		},
		Username: username,
		ClientID: "test-client",
		Scopes:   []string{auth.ScopeRead, auth.ScopeWrite},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(jwtSecret)

	return map[string]string{
		"Authorization": "Bearer " + tokenString,
	}
}

func TestHandler(t *testing.T) {
	// Initialize logger for tests
	logger = zap.NewNop()

	// Initialize auth middleware with test secret
	originalAuthMiddleware := authMiddleware
	if err := os.Setenv("JWT_SECRET", "test-secret"); err != nil {
		t.Fatalf("Failed to set JWT_SECRET: %v", err)
	}
	authMiddleware = auth.NewMiddleware()
	defer func() {
		authMiddleware = originalAuthMiddleware
	}()

	tests := []struct {
		name           string
		method         string
		username       string
		body           string
		queryParams    map[string]string
		headers        map[string]string
		setupMocks     func(*mocks.MockStorage)
		expectedStatus int
		expectedError  bool
		validateBody   func(*testing.T, string)
	}{
		// GET request tests
		{
			name:     "get outbox collection",
			method:   http.MethodGet,
			username: "alice",
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Inbox:             "https://example.com/users/alice/inbox",
					Outbox:            "https://example.com/users/alice/outbox",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)

				// Return empty activities for collection metadata
				m.On("GetOutboxActivities", mock.Anything, "alice", 1, "").Return([]*activitypub.Activity{}, "", nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var collection activitypub.OrderedCollection
				err := json.Unmarshal([]byte(body), &collection)
				assert.NoError(t, err)
				assert.Equal(t, "OrderedCollection", collection.Type)
				assert.Equal(t, "https://example.com/users/alice/outbox", collection.ID)
				assert.Equal(t, "https://example.com/users/alice/outbox?page=true", collection.First)
			},
		},
		{
			name:     "get outbox page with activities",
			method:   http.MethodGet,
			username: "alice",
			queryParams: map[string]string{
				"page": "true",
			},
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Outbox:            "https://example.com/users/alice/outbox",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)

				activities := []*activitypub.Activity{
					{
						BaseObject: activitypub.BaseObject{
							ID:   "https://example.com/activities/1",
							Type: "Follow",
						},
						Actor:  "https://example.com/users/alice",
						Object: "https://remote.example/users/bob",
					},
					{
						BaseObject: activitypub.BaseObject{
							ID:   "https://example.com/activities/2",
							Type: "Like",
						},
						Actor:  "https://example.com/users/alice",
						Object: "https://remote.example/notes/123",
					},
				}
				m.On("GetOutboxActivities", mock.Anything, "alice", 20, "").Return(activities, "next-cursor", nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
				assert.Equal(t, "OrderedCollectionPage", page.Type)
				assert.Equal(t, "https://example.com/users/alice/outbox?page=true", page.ID)
				assert.Equal(t, "https://example.com/users/alice/outbox", page.PartOf)
				assert.Equal(t, "https://example.com/users/alice/outbox?page=true&cursor=next-cursor&limit=20", page.Next)

				// Check activities
				items, ok := page.OrderedItems.([]any)
				assert.True(t, ok)
				assert.Len(t, items, 2)
			},
		},
		{
			name:     "get outbox page with cursor",
			method:   http.MethodGet,
			username: "alice",
			queryParams: map[string]string{
				"page":   "true",
				"cursor": "some-cursor",
				"limit":  "10",
			},
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Outbox:            "https://example.com/users/alice/outbox",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("GetOutboxActivities", mock.Anything, "alice", 10, "some-cursor").Return([]*activitypub.Activity{}, "", nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
				assert.Equal(t, "https://example.com/users/alice/outbox?page=true&limit=10", page.Prev)
			},
		},
		{
			name:     "get outbox with invalid limit",
			method:   http.MethodGet,
			username: "alice",
			queryParams: map[string]string{
				"page":  "true",
				"limit": "invalid",
			},
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Outbox:            "https://example.com/users/alice/outbox",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				// Should use default limit of 20
				m.On("GetOutboxActivities", mock.Anything, "alice", 20, "").Return([]*activitypub.Activity{}, "", nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "get outbox actor not found",
			method:   http.MethodGet,
			username: "unknown",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "unknown").Return(nil, common.ActorNotFoundError{Username: "unknown"})
			},
			expectedStatus: http.StatusNotFound,
		},
		// POST request tests (existing tests)
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
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "like activity with string object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Like",
				"object": "https://remote.example/notes/123"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					return activity.Type == "Like" &&
						activity.Object == "https://remote.example/notes/123" &&
						activity.Actor == "https://example.com/users/alice" &&
						activity.Published != nil
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Like", activity.Type)
				assert.Equal(t, "https://remote.example/notes/123", activity.Object)
				assert.NotNil(t, activity.Published)
			},
		},
		{
			name:     "like activity with object map",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Like",
				"object": {
					"id": "https://remote.example/notes/123",
					"type": "Note"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					if activity.Type != "Like" {
						return false
					}
					objMap, ok := activity.Object.(map[string]any)
					return ok && objMap["id"] == "https://remote.example/notes/123"
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "like activity with invalid object URL",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Like",
				"object": "not-a-url"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "like activity without object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Like"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			method:   http.MethodDelete,
			username: "alice",
			setupMocks: func(m *mocks.MockStorage) {
				// No mocks needed
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "missing username",
			method: http.MethodPost,
			body:   `{"type": "Follow"}`,
			setupMocks: func(m *mocks.MockStorage) {
				// No mocks needed
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "actor not found",
			method:   http.MethodPost,
			username: "unknown",
			body:     `{"type": "Follow"}`,
			headers:  createTestAuthHeader("unknown"),
			setupMocks: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "unknown").Return(nil, common.ActorNotFoundError{Username: "unknown"})
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:     "invalid JSON",
			method:   http.MethodPost,
			username: "alice",
			body:     `{invalid json`,
			headers:  createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
		// New tests for Create activity functionality
		{
			name:     "create note - simple format",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Hello, Fediverse!"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Create", activity.Type)
				assert.Equal(t, "https://example.com/users/alice", activity.Actor)
				assert.NotEmpty(t, activity.ID)
				assert.NotNil(t, activity.Published)

				// Check object
				obj, ok := activity.Object.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "Note", obj["type"])
				assert.Equal(t, "Hello, Fediverse!", obj["content"])
				assert.NotEmpty(t, obj["id"])
				assert.Equal(t, "https://example.com/users/alice", obj["attributedTo"])
				assert.NotEmpty(t, obj["published"])

				// Check default addressing
				to, ok := obj["to"].([]any)
				assert.True(t, ok)
				assert.Contains(t, to, activitypub.PublicAddress)

				cc, ok := obj["cc"].([]any)
				assert.True(t, ok)
				assert.Contains(t, cc, "https://example.com/users/alice/followers")
			},
		},
		{
			name:     "create article",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"to": ["https://example.com/users/bob"],
				"cc": ["https://www.w3.org/ns/activitystreams#Public"],
				"object": {
					"type": "Article",
					"name": "My First Article",
					"content": "This is the article content.",
					"summary": "A brief summary"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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

				// Check object
				obj, ok := activity.Object.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "Article", obj["type"])
				assert.Equal(t, "My First Article", obj["name"])
				assert.Equal(t, "This is the article content.", obj["content"])
				assert.Equal(t, "A brief summary", obj["summary"])

				// Check addressing copied from activity to object
				to, ok := obj["to"].([]any)
				assert.True(t, ok)
				assert.Contains(t, to, "https://example.com/users/bob")

				cc, ok := obj["cc"].([]any)
				assert.True(t, ok)
				assert.Contains(t, cc, activitypub.PublicAddress)
			},
		},
		{
			name:     "create note with existing ID",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"id": "https://example.com/objects/custom-123",
					"type": "Note",
					"content": "Note with custom ID"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)

				obj, ok := activity.Object.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "https://example.com/objects/custom-123", obj["id"])
			},
		},
		{
			name:     "create note without type defaults to Note",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"content": "A note without explicit type"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)

				obj, ok := activity.Object.(map[string]any)
				assert.True(t, ok)
				assert.NotEmpty(t, obj["id"]) // Should have generated ID
			},
		},
		{
			name:     "create without object fails",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create note without content fails",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create article without name fails",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Article",
					"content": "Content without name"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		// Enhanced validation tests
		{
			name:     "create note with content exceeding limit",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "` + strings.Repeat("a", 501) + `"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create article with name exceeding limit",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Article",
					"name": "` + strings.Repeat("a", 201) + `",
					"content": "Article content"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create note with valid attachments",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Check out this photo!",
					"attachment": [{
						"type": "Image",
						"url": "https://example.com/photo.jpg",
						"mediaType": "image/jpeg",
						"name": "A beautiful sunset"
					}]
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "create note with invalid attachment URL",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Check out this photo!",
					"attachment": [{
						"type": "Image",
						"url": "not-a-valid-url",
						"mediaType": "image/jpeg"
					}]
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create note with unsupported media type",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Check out this file!",
					"attachment": [{
						"type": "Document",
						"url": "https://example.com/file.exe",
						"mediaType": "application/x-msdownload"
					}]
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create note with valid contentMap",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Hello world!",
					"contentMap": {
						"en": "Hello world!",
						"es": "¡Hola mundo!",
						"fr-CA": "Bonjour le monde!"
					}
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "create note with invalid language code",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Hello world!",
					"contentMap": {
						"english": "Hello world!"
					}
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create note with valid hashtags",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Check out #photography and #nature",
					"tag": [
						{
							"type": "Hashtag",
							"name": "#photography",
							"href": "https://example.com/tags/photography"
						},
						{
							"type": "Hashtag",
							"name": "#nature",
							"href": "https://example.com/tags/nature"
						}
					]
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "create note with invalid hashtag format",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Check out photography",
					"tag": [{
						"type": "Hashtag",
						"name": "photography",
						"href": "https://example.com/tags/photography"
					}]
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "create note with mention missing href",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"type": "Create",
				"object": {
					"type": "Note",
					"content": "Hey @bob!",
					"tag": [{
						"type": "Mention",
						"name": "@bob"
					}]
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "announce activity with string object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Announce",
				"object": "https://remote.example/notes/456"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					return activity.Type == "Announce" &&
						activity.Object == "https://remote.example/notes/456" &&
						activity.Actor == "https://example.com/users/alice" &&
						activity.Published != nil &&
						activity.CC != nil && len(activity.CC) == 1 && activity.CC[0] == actor.Followers
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Announce", activity.Type)
				assert.Equal(t, "https://remote.example/notes/456", activity.Object)
				assert.NotNil(t, activity.Published)
				assert.Contains(t, activity.CC, "https://example.com/users/alice/followers")
			},
		},
		{
			name:     "announce activity with object map",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Announce",
				"object": {
					"id": "https://remote.example/notes/456",
					"type": "Note"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					if activity.Type != "Announce" {
						return false
					}
					objMap, ok := activity.Object.(map[string]any)
					return ok && objMap["id"] == "https://remote.example/notes/456"
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "announce activity with invalid object URL",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Announce",
				"object": "not-a-url"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "announce activity without object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Announce"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
		// Undo Activity tests
		{
			name:     "undo follow activity",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Follow",
					"actor": "https://example.com/users/alice",
					"object": "https://example.com/users/bob"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("IsFollowing", mock.Anything, "alice", "bob").Return(true, nil)
				m.On("RemoveFollow", mock.Anything, "alice", "bob").Return(nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					if activity.Type != "Undo" {
						return false
					}
					objMap, ok := activity.Object.(map[string]any)
					return ok && objMap["type"] == "Follow" &&
						objMap["actor"] == "https://example.com/users/alice" &&
						objMap["object"] == "https://example.com/users/bob"
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Undo", activity.Type)
				assert.Contains(t, activity.To, "https://example.com/users/bob")
			},
		},
		{
			name:     "undo follow - not following",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Follow",
					"actor": "https://example.com/users/alice",
					"object": "https://example.com/users/bob"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("IsFollowing", mock.Anything, "alice", "bob").Return(false, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "undo like activity",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Like",
					"actor": "https://example.com/users/alice",
					"object": "https://example.com/objects/123"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("GetLike", mock.Anything, "https://example.com/users/alice", "https://example.com/objects/123").Return(&storage.Like{}, nil)
				m.On("DeleteLike", mock.Anything, "https://example.com/users/alice", "https://example.com/objects/123").Return(nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					if activity.Type != "Undo" {
						return false
					}
					objMap, ok := activity.Object.(map[string]any)
					return ok && objMap["type"] == "Like" &&
						objMap["object"] == "https://example.com/objects/123"
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "undo like - like not found",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Like",
					"actor": "https://example.com/users/alice",
					"object": "https://example.com/objects/123"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("GetLike", mock.Anything, "https://example.com/users/alice", "https://example.com/objects/123").Return(nil, errors.New("not found"))
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "undo announce activity",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Announce",
					"actor": "https://example.com/users/alice",
					"object": "https://example.com/objects/456"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
					Followers:         "https://example.com/users/alice/followers",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("GetAnnounce", mock.Anything, "https://example.com/users/alice", "https://example.com/objects/456").Return(&storage.Announce{}, nil)
				m.On("DeleteAnnounce", mock.Anything, "https://example.com/users/alice", "https://example.com/objects/456").Return(nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					if activity.Type != "Undo" {
						return false
					}
					objMap, ok := activity.Object.(map[string]any)
					return ok && objMap["type"] == "Announce" &&
						objMap["object"] == "https://example.com/objects/456" &&
						activity.CC != nil && len(activity.CC) > 0
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:     "undo without object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "undo with object as string",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": "https://example.com/activities/123"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "undo someone else's activity",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Follow",
					"actor": "https://example.com/users/bob",
					"object": "https://example.com/users/alice"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "undo unsupported activity type",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Undo",
				"object": {
					"type": "Create",
					"actor": "https://example.com/users/alice",
					"object": "https://example.com/objects/123"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		// Block Activity tests
		{
			name:     "block activity with string object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Block",
				"object": "https://example.com/users/bob"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateBlock", mock.Anything, mock.MatchedBy(func(block *storage.Block) bool {
					return block.Actor == "https://example.com/users/alice" &&
						block.Object == "https://example.com/users/bob" &&
						block.ID != "" &&
						!block.Published.IsZero()
				})).Return(nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(activity *activitypub.Activity) bool {
					return activity.Type == "Block" &&
						activity.Object == "https://example.com/users/bob" &&
						activity.To != nil && len(activity.To) > 0
				})).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Block", activity.Type)
				assert.Equal(t, "https://example.com/users/bob", activity.Object)
				assert.Contains(t, activity.To, "https://example.com/users/bob")
			},
		},
		{
			name:     "block activity with object map",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Block",
				"object": {
					"id": "https://example.com/users/bob",
					"type": "Person"
				}
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateBlock", mock.Anything, mock.AnythingOfType("*storage.Block")).Return(nil)
				m.On("CreateActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			validateBody: func(t *testing.T, body string) {
				var activity activitypub.Activity
				err := json.Unmarshal([]byte(body), &activity)
				assert.NoError(t, err)
				assert.Equal(t, "Block", activity.Type)
				// Object should be normalized to string
				assert.Equal(t, "https://example.com/users/bob", activity.Object)
			},
		},
		{
			name:     "block activity with invalid object URL",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Block",
				"object": "not-a-url"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "block activity without object",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Block"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
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
			name:     "block activity - already blocked",
			method:   http.MethodPost,
			username: "alice",
			body: `{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type": "Block",
				"object": "https://example.com/users/bob"
			}`,
			headers: createTestAuthHeader("alice"),
			setupMocks: func(m *mocks.MockStorage) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						Type:    "Person",
						ID:      "https://example.com/users/alice",
					},
					PreferredUsername: "alice",
				}
				m.On("GetActor", mock.Anything, "alice").Return(actor, nil)
				m.On("CreateBlock", mock.Anything, mock.AnythingOfType("*storage.Block")).
					Return(errors.New("already exists"))
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage
			mockStore := new(mocks.MockStorage)
			tt.setupMocks(mockStore)
			store = mockStore

			// Create request
			request := events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: tt.method,
					},
				},
				PathParameters: map[string]string{
					"username": tt.username,
				},
				Body:                  tt.body,
				Headers:               tt.headers,
				QueryStringParameters: tt.queryParams,
			}

			// Add Content-Type header if not present
			if request.Headers == nil {
				request.Headers = make(map[string]string)
			}
			if request.Headers["Content-Type"] == "" {
				request.Headers["Content-Type"] = "application/activity+json"
			}

			// Call handler
			response, err := handler(context.Background(), request)

			// Verify
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, response.StatusCode)

				if tt.validateBody != nil && (response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusOK) {
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
