package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStorage is a mock implementation of the storage interface
type MockStorage struct {
	mock.Mock
	mocks.BaseMockStorage
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

func (m *MockStorage) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// Override only the methods that need mock expectations in these tests
func (m *MockStorage) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) UpdateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// Implement remaining interface methods as stubs...
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

func (m *MockStorage) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

func (m *MockStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AuthorizationCode), args.Error(1)
}

func (m *MockStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

func (m *MockStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RefreshToken), args.Error(1)
}

func (m *MockStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func TestHandler(t *testing.T) {
	// Save original store and restore after test
	originalStore := store
	defer func() { store = originalStore }()

	tests := []struct {
		name         string
		request      events.APIGatewayV2HTTPRequest
		setupMock    func(*mocks.MockStorage)
		expectedCode int
		checkBody    func(t *testing.T, body string)
	}{
		{
			name: "missing object ID",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				PathParameters: map[string]string{},
			},
			setupMock:    func(m *mocks.MockStorage) {},
			expectedCode: 400,
		},
		{
			name: "object not found",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				PathParameters: map[string]string{
					"id": "https://example.com/objects/999",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetObject", mock.Anything, "https://example.com/objects/999").Return(nil, fmt.Errorf("object not found"))
			},
			expectedCode: 404,
		},
		{
			name: "success - JSON response",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				PathParameters: map[string]string{
					"id": "https://example.com/objects/123",
				},
				Headers: map[string]string{
					"Accept": "application/json",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				obj := &dynamodb.Object{
					ID:           "https://example.com/objects/123",
					Type:         "Note",
					Content:      "Hello, world!",
					AttributedTo: "https://example.com/users/alice",
					Published:    time.Now(),
				}
				m.On("GetObject", mock.Anything, "https://example.com/objects/123").Return(obj, nil)
			},
			expectedCode: 200,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "Hello, world!")
				assert.Contains(t, body, "Note")
			},
		},
		{
			name: "success - HTML response",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				PathParameters: map[string]string{
					"id": "https://example.com/objects/123",
				},
				Headers: map[string]string{
					"Accept": "text/html",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				obj := &dynamodb.Object{
					ID:           "https://example.com/objects/123",
					Type:         "Note",
					Content:      "Hello, world!",
					AttributedTo: "https://example.com/users/alice",
					Published:    time.Now(),
					Sensitive:    false,
				}
				m.On("GetObject", mock.Anything, "https://example.com/objects/123").Return(obj, nil)
			},
			expectedCode: 200,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "<!DOCTYPE html>")
				assert.Contains(t, body, "Hello, world!")
				assert.Contains(t, body, "Note")
				assert.Contains(t, body, "@alice")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(mocks.MockStorage)
			tt.setupMock(mockStore)
			store = mockStore

			resp, err := handler(context.Background(), tt.request)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			if tt.checkBody != nil {
				tt.checkBody(t, resp.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestGenerateObjectHTML(t *testing.T) {
	obj := &dynamodb.Object{
		ID:           "https://example.com/objects/123",
		Type:         "Note",
		Content:      "Test content with <b>HTML</b>",
		AttributedTo: "https://example.com/users/alice",
		Published:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Sensitive:    true,
		Summary:      "Content warning: test",
		Tag: []dynamodb.ObjectTag{
			{Type: "Hashtag", Name: "#test", Href: "https://example.com/tags/test"},
		},
		Attachment: []dynamodb.ObjectAttachment{
			{Type: "Image", URL: "https://example.com/image.jpg", Name: "Test image"},
		},
	}

	html := generateObjectHTML(obj)

	// Check that HTML is properly escaped
	assert.Contains(t, html, "Test content with &lt;b&gt;HTML&lt;/b&gt;")

	// Check content warning
	assert.Contains(t, html, "Content Warning:")
	assert.Contains(t, html, "Content warning: test")

	// Check hashtag
	assert.Contains(t, html, "#test")

	// Check attachment
	assert.Contains(t, html, "Test image")
	assert.Contains(t, html, "https://example.com/image.jpg")

	// Check metadata
	assert.Contains(t, html, "January 1, 2024 at 12:00 PM")
	assert.Contains(t, html, "@alice")
}

func TestExtractUsernameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "standard URL",
			url:      "https://example.com/users/alice",
			expected: "@alice",
		},
		{
			name:     "URL with path",
			url:      "https://example.com/users/bob/profile",
			expected: "@profile",
		},
		{
			name:     "just username",
			url:      "charlie",
			expected: "@charlie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
