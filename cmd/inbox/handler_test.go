package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of storage.Storage
type MockStorage struct {
	mock.Mock
	mocks.BaseMockStorage
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
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

func (m *MockStorage) CreateObject(ctx context.Context, object any) error {
	return nil
}

func (m *MockStorage) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
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

// OAuth-related methods (needed for auth middleware)
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

// Test helper functions

func createTestActor(username string) *activitypub.Actor {
	baseURL := "https://example.com"
	actorID := fmt.Sprintf("%s/users/%s", baseURL, username)

	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: username,
		Name:              "Test User",
		Summary:           "A test user",
		Inbox:             actorID + "/inbox",
		Outbox:            actorID + "/outbox",
		PublicKey: &activitypub.PublicKey{
			ID:           actorID + "#main-key",
			Owner:        actorID,
			PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----",
		},
	}
}

func createTestActivity(actorURL string, toURL string) *activitypub.Activity {
	now := time.Now()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/activities/123",
			Type:      activitypub.FollowType,
			Published: &now,
			To:        []string{toURL},
		},
		Actor:  actorURL,
		Object: toURL,
	}
}

func generateTestKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	return privateKey, &privateKey.PublicKey, nil
}

func encodePublicKeyPEM(publicKey *rsa.PublicKey) (string, error) {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", err
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

func createSignedRequest(method, path string, body []byte, privateKey *rsa.PrivateKey, keyID string) (*events.APIGatewayV2HTTPRequest, error) {
	// Create a real HTTP request for signing
	req, err := http.NewRequest(method, "https://example.com"+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Host", "example.com")
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Content-Type", "application/activity+json")

	// Sign the request
	if err := federation.SignHTTPRequest(req, privateKey, keyID); err != nil {
		return nil, err
	}

	// Convert to Lambda request
	headers := make(map[string]string)
	for k, v := range req.Header {
		headers[k] = v[0]
	}

	return &events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: method,
			},
		},
		RawPath:        path,
		Headers:        headers,
		Body:           string(body),
		PathParameters: map[string]string{"username": "alice"},
	}, nil
}

// Mock HTTP server for fetching remote actor profiles
func setupMockActorServer(t *testing.T, actor *activitypub.Actor) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check Accept header
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/activity+json") &&
			!strings.Contains(accept, "application/ld+json") {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}

		// Return actor profile
		w.Header().Set("Content-Type", "application/activity+json")
		if err := json.NewEncoder(w).Encode(actor); err != nil {
			t.Fatal(err)
		}
	}))
}

func TestHandler(t *testing.T) {
	// Setup
	mockStore := new(mocks.MockStorage)
	store = mockStore

	// Create test actor (recipient)
	recipient := createTestActor("alice")

	// Generate test key pair for sender
	privateKey, publicKey, err := generateTestKeyPair()
	require.NoError(t, err)

	publicKeyPEM, err := encodePublicKeyPEM(publicKey)
	require.NoError(t, err)

	tests := []struct {
		name          string
		setupRequest  func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error)
		setupMock     func(*mocks.MockStorage)
		setupServer   func() (*httptest.Server, *activitypub.Actor)
		expectedCode  int
		expectedError string
	}{
		{
			name: "successful activity delivery",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   senderBaseURL + "/users/bob",
						Type: activitypub.PersonType,
					},
					PreferredUsername: "bob",
					PublicKey: &activitypub.PublicKey{
						ID:           senderBaseURL + "/users/bob#main-key",
						Owner:        senderBaseURL + "/users/bob",
						PublicKeyPem: publicKeyPEM,
					},
				}
				activity := createTestActivity(sender.ID, recipient.ID)
				body, _ := json.Marshal(activity)
				return createSignedRequest("POST", "/users/alice/inbox", body, privateKey, sender.PublicKey.ID)
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(recipient, nil)
				m.On("CreateActivity", mock.Anything, mock.MatchedBy(func(a *activitypub.Activity) bool {
					return strings.Contains(a.ID, "/activities/123")
				})).Return(nil)
			},
			setupServer: func() (*httptest.Server, *activitypub.Actor) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Type: activitypub.PersonType,
					},
					PreferredUsername: "bob",
					PublicKey: &activitypub.PublicKey{
						PublicKeyPem: publicKeyPEM,
					},
				}
				server := setupMockActorServer(t, sender)
				// Update sender with server URL
				sender.ID = server.URL + "/users/bob"
				sender.PublicKey.ID = server.URL + "/users/bob#main-key"
				sender.PublicKey.Owner = server.URL + "/users/bob"
				return server, sender
			},
			expectedCode: http.StatusAccepted,
		},
		{
			name: "wrong HTTP method",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				return &events.APIGatewayV2HTTPRequest{
					RequestContext: events.APIGatewayV2HTTPRequestContext{
						HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
							Method: "DELETE",
						},
					},
					PathParameters: map[string]string{"username": "alice"},
				}, nil
			},
			setupMock:    func(m *mocks.MockStorage) {},
			setupServer:  func() (*httptest.Server, *activitypub.Actor) { return nil, nil },
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "missing username",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				return &events.APIGatewayV2HTTPRequest{
					RequestContext: events.APIGatewayV2HTTPRequestContext{
						HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
							Method: "POST",
						},
					},
					PathParameters: map[string]string{},
				}, nil
			},
			setupMock:    func(m *mocks.MockStorage) {},
			setupServer:  func() (*httptest.Server, *activitypub.Actor) { return nil, nil },
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "actor not found",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example/users/bob",
						Type: activitypub.PersonType,
					},
				}
				activity := createTestActivity(sender.ID, recipient.ID)
				body, _ := json.Marshal(activity)
				return createSignedRequest("POST", "/users/alice/inbox", body, privateKey, "https://remote.example/users/bob#main-key")
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(nil, common.ActorNotFoundError{Username: "alice"})
			},
			setupServer:  func() (*httptest.Server, *activitypub.Actor) { return nil, nil },
			expectedCode: http.StatusNotFound,
		},
		{
			name: "invalid JSON",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				return &events.APIGatewayV2HTTPRequest{
					RequestContext: events.APIGatewayV2HTTPRequestContext{
						HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
							Method: "POST",
						},
					},
					PathParameters: map[string]string{"username": "alice"},
					Body:           "invalid json",
				}, nil
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(recipient, nil)
			},
			setupServer:  func() (*httptest.Server, *activitypub.Actor) { return nil, nil },
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "missing activity ID",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example/users/bob",
						Type: activitypub.PersonType,
					},
				}
				activity := createTestActivity(sender.ID, recipient.ID)
				activity.ID = ""
				body, _ := json.Marshal(activity)
				return createSignedRequest("POST", "/users/alice/inbox", body, privateKey, "https://remote.example/users/bob#main-key")
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(recipient, nil)
			},
			setupServer:  func() (*httptest.Server, *activitypub.Actor) { return nil, nil },
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "activity not addressed to actor",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example/users/bob",
						Type: activitypub.PersonType,
					},
				}
				activity := createTestActivity(sender.ID, "https://example.com/users/charlie")
				activity.To = []string{"https://example.com/users/charlie"}
				body, _ := json.Marshal(activity)
				return createSignedRequest("POST", "/users/alice/inbox", body, privateKey, "https://remote.example/users/bob#main-key")
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(recipient, nil)
			},
			setupServer:  func() (*httptest.Server, *activitypub.Actor) { return nil, nil },
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "signature verification failure",
			setupRequest: func(senderBaseURL string) (*events.APIGatewayV2HTTPRequest, error) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   senderBaseURL + "/users/bob",
						Type: activitypub.PersonType,
					},
					PublicKey: &activitypub.PublicKey{
						ID:           senderBaseURL + "/users/bob#main-key",
						Owner:        senderBaseURL + "/users/bob",
						PublicKeyPem: publicKeyPEM,
					},
				}
				activity := createTestActivity(sender.ID, recipient.ID)
				body, _ := json.Marshal(activity)
				req, _ := createSignedRequest("POST", "/users/alice/inbox", body, privateKey, sender.PublicKey.ID)
				// Tamper with the signature
				req.Headers["Signature"] = "keyId=\"test\",algorithm=\"rsa-sha256\",signature=\"invalid\""
				return req, nil
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(recipient, nil)
			},
			setupServer: func() (*httptest.Server, *activitypub.Actor) {
				sender := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Type: activitypub.PersonType,
					},
					PreferredUsername: "bob",
					PublicKey: &activitypub.PublicKey{
						PublicKeyPem: publicKeyPEM,
					},
				}
				server := setupMockActorServer(t, sender)
				// Update sender with server URL
				sender.ID = server.URL + "/users/bob"
				sender.PublicKey.ID = server.URL + "/users/bob#main-key"
				sender.PublicKey.Owner = server.URL + "/users/bob"
				return server, sender
			},
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore.ExpectedCalls = nil
			mockStore.Calls = nil

			// Setup mock expectations
			tt.setupMock(mockStore)

			// Setup test server if needed
			server, _ := tt.setupServer()
			if server != nil {
				defer server.Close()
			}

			// Determine sender base URL
			senderBaseURL := "https://remote.example"
			if server != nil {
				senderBaseURL = server.URL
			}

			// Create request
			req, err := tt.setupRequest(senderBaseURL)
			require.NoError(t, err)

			// Execute handler
			resp, err := handler(context.Background(), *req)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			if tt.expectedError != "" {
				var errResp common.ErrorResponse
				err := json.Unmarshal([]byte(resp.Body), &errResp)
				assert.NoError(t, err)
				assert.Contains(t, errResp.Message, tt.expectedError)
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}

func TestIsAddressedTo(t *testing.T) {
	actor := createTestActor("alice")

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
			name: "addressed in cc field",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					CC: []string{actor.ID},
				},
			},
			expected: true,
		},
		{
			name: "addressed in bto field",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					BTo: []string{actor.ID},
				},
			},
			expected: true,
		},
		{
			name: "addressed in bcc field",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					BCC: []string{actor.ID},
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
			result := isAddressedTo(tt.activity, actor)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertLambdaRequest(t *testing.T) {
	lambdaReq := &events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "POST",
			},
		},
		RawPath: "/users/alice/inbox",
		Headers: map[string]string{
			"Host":         "example.com",
			"Content-Type": "application/activity+json",
			"Date":         time.Now().UTC().Format(http.TimeFormat),
		},
		QueryStringParameters: map[string]string{
			"test": "value",
		},
		Body: `{"test": "body"}`,
	}

	httpReq, err := convertLambdaRequest(lambdaReq, []byte(lambdaReq.Body))
	require.NoError(t, err)

	assert.Equal(t, "POST", httpReq.Method)
	assert.Equal(t, "https://example.com/users/alice/inbox?test=value", httpReq.URL.String())
	assert.Equal(t, "example.com", httpReq.Host)
	assert.Equal(t, "application/activity+json", httpReq.Header.Get("Content-Type"))

	// Read body
	body, err := io.ReadAll(httpReq.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"test": "body"}`, string(body))
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

func TestGetInbox(t *testing.T) {
	// Initialize logger and auth middleware
	logger = zap.NewNop()
	if err := os.Setenv("JWT_SECRET", "test-secret"); err != nil {
		t.Fatalf("Failed to set JWT_SECRET: %v", err)
	}
	authMiddleware = auth.NewMiddleware()

	// Setup
	mockStore := new(mocks.MockStorage)
	store = mockStore

	// Create test actor
	alice := createTestActor("alice")

	tests := []struct {
		name          string
		username      string
		headers       map[string]string
		queryParams   map[string]string
		setupMock     func(*mocks.MockStorage)
		expectedCode  int
		expectedError string
		validateBody  func(*testing.T, string)
	}{
		{
			name:     "get inbox collection - authenticated",
			username: "alice",
			headers:  createTestAuthHeader("alice"),
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(alice, nil)
				m.On("GetInboxActivities", mock.Anything, "alice", 1, "").Return([]*activitypub.Activity{}, "", nil)
			},
			expectedCode: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var collection activitypub.OrderedCollection
				err := json.Unmarshal([]byte(body), &collection)
				assert.NoError(t, err)
				assert.Equal(t, "OrderedCollection", collection.Type)
				assert.Equal(t, alice.Inbox, collection.ID)
				assert.Equal(t, alice.Inbox+"?page=true", collection.First)
			},
		},
		{
			name:     "get inbox page with activities",
			username: "alice",
			headers:  createTestAuthHeader("alice"),
			queryParams: map[string]string{
				"page": "true",
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(alice, nil)

				activities := []*activitypub.Activity{
					{
						BaseObject: activitypub.BaseObject{
							ID:   "https://remote.example/activities/1",
							Type: "Follow",
						},
						Actor:  "https://remote.example/users/bob",
						Object: alice.ID,
					},
					{
						BaseObject: activitypub.BaseObject{
							ID:   "https://remote.example/activities/2",
							Type: "Create",
						},
						Actor:  "https://remote.example/users/charlie",
						Object: "https://remote.example/objects/note1",
					},
				}
				m.On("GetInboxActivities", mock.Anything, "alice", 20, "").Return(activities, "", nil)
				// Mock object fetching for Create activity
				m.On("GetObject", mock.Anything, "https://remote.example/objects/note1").Return(
					map[string]any{
						"id":      "https://remote.example/objects/note1",
						"type":    "Note",
						"content": "Hello Alice!",
					}, nil)
			},
			expectedCode: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
				assert.Equal(t, "OrderedCollectionPage", page.Type)
				assert.Equal(t, alice.Inbox+"?page=true", page.ID)
				assert.Equal(t, alice.Inbox, page.PartOf)

				// Check activities
				items, ok := page.OrderedItems.([]any)
				assert.True(t, ok)
				assert.Len(t, items, 2)

				// Check that Create activity has enriched object
				createActivity := items[1].(map[string]any)
				assert.Equal(t, "Create", createActivity["type"])
				obj, ok := createActivity["object"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "Note", obj["type"])
				assert.Equal(t, "Hello Alice!", obj["content"])
			},
		},
		{
			name:     "get inbox with pagination",
			username: "alice",
			headers:  createTestAuthHeader("alice"),
			queryParams: map[string]string{
				"page":   "true",
				"cursor": "some-cursor",
				"limit":  "10",
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(alice, nil)
				m.On("GetInboxActivities", mock.Anything, "alice", 10, "some-cursor").Return([]*activitypub.Activity{}, "next-cursor", nil)
			},
			expectedCode: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
				assert.Equal(t, alice.Inbox+"?page=true&cursor=next-cursor&limit=10", page.Next)
				assert.Equal(t, alice.Inbox+"?page=true&limit=10", page.Prev)
			},
		},
		{
			name:     "get inbox - not authenticated",
			username: "alice",
			headers:  map[string]string{},
			setupMock: func(m *mocks.MockStorage) {
				// No mocks needed - should fail at auth
			},
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:     "get inbox - wrong user",
			username: "alice",
			headers:  createTestAuthHeader("bob"),
			setupMock: func(m *mocks.MockStorage) {
				// No mocks needed - should fail at user check
			},
			expectedCode: http.StatusForbidden,
		},
		{
			name:     "get inbox - no read scope",
			username: "alice",
			headers: func() map[string]string {
				jwtSecret := []byte("test-secret")
				now := time.Now()
				claims := auth.Claims{
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   "alice",
						IssuedAt:  jwt.NewNumericDate(now),
						ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
						NotBefore: jwt.NewNumericDate(now),
					},
					Username: "alice",
					ClientID: "test-client",
					Scopes:   []string{auth.ScopeWrite}, // Only write scope
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, _ := token.SignedString(jwtSecret)
				return map[string]string{
					"Authorization": "Bearer " + tokenString,
				}
			}(),
			setupMock: func(m *mocks.MockStorage) {
				// No mocks needed - should fail at scope check
			},
			expectedCode: http.StatusForbidden,
		},
		{
			name:     "get inbox - actor not found",
			username: "unknown",
			headers:  createTestAuthHeader("unknown"),
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "unknown").Return(nil, common.ActorNotFoundError{Username: "unknown"})
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "get inbox - storage error",
			username: "alice",
			headers:  createTestAuthHeader("alice"),
			queryParams: map[string]string{
				"page": "true",
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(alice, nil)
				m.On("GetInboxActivities", mock.Anything, "alice", 20, "").Return(nil, "", fmt.Errorf("storage error"))
			},
			expectedCode: http.StatusInternalServerError,
		},
		{
			name:     "get inbox - object fetch error ignored",
			username: "alice",
			headers:  createTestAuthHeader("alice"),
			queryParams: map[string]string{
				"page": "true",
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(alice, nil)

				activities := []*activitypub.Activity{
					{
						BaseObject: activitypub.BaseObject{
							ID:   "https://remote.example/activities/1",
							Type: "Create",
						},
						Actor:  "https://remote.example/users/bob",
						Object: "https://remote.example/objects/note1",
					},
				}
				m.On("GetInboxActivities", mock.Anything, "alice", 20, "").Return(activities, "", nil)
				// Mock object fetching failure
				m.On("GetObject", mock.Anything, "https://remote.example/objects/note1").Return(nil, fmt.Errorf("not found"))
			},
			expectedCode: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)

				// Activity should still be returned, just without enriched object
				items, ok := page.OrderedItems.([]any)
				assert.True(t, ok)
				assert.Len(t, items, 1)

				createActivity := items[0].(map[string]any)
				assert.Equal(t, "Create", createActivity["type"])
				// Object should still be the ID string
				assert.Equal(t, "https://remote.example/objects/note1", createActivity["object"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore.ExpectedCalls = nil
			mockStore.Calls = nil

			// Setup mock expectations
			tt.setupMock(mockStore)

			// Create request
			request := events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				PathParameters: map[string]string{
					"username": tt.username,
				},
				Headers:               tt.headers,
				QueryStringParameters: tt.queryParams,
			}

			// Execute handler
			resp, err := handler(context.Background(), request)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			if tt.expectedError != "" {
				var errResp common.ErrorResponse
				err := json.Unmarshal([]byte(resp.Body), &errResp)
				assert.NoError(t, err)
				assert.Contains(t, errResp.Message, tt.expectedError)
			}

			if tt.validateBody != nil && resp.StatusCode == http.StatusOK {
				tt.validateBody(t, resp.Body)
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}

func (m *MockStorage) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// Object cascade operations
func (m *MockStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Collection operations
func (m *MockStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

func (m *MockStorage) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

func (m *MockStorage) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	args := m.Called(ctx, collection, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CollectionItem), args.String(1), args.Error(2)
}

func (m *MockStorage) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	args := m.Called(ctx, collection, itemID)
	return args.Bool(0), args.Error(1)
}

// Timeline operations
func (m *MockStorage) WriteToTimeline(ctx context.Context, timeline *storage.TimelineEntry) error {
	args := m.Called(ctx, timeline)
	return args.Error(0)
}

func (m *MockStorage) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	args := m.Called(ctx, entries)
	return args.Error(0)
}

func (m *MockStorage) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

func (m *MockStorage) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

func (m *MockStorage) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorage) CountFollowers(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}
