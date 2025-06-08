package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHandler(t *testing.T) {
	// Initialize config
	cfg = &config.Config{
		Domain: "https://example.com",
	}

	// Create test actor
	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Alice Test",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
		Followers:         "https://example.com/users/alice/followers",
		Following:         "https://example.com/users/alice/following",
	}

	tests := []struct {
		name              string
		request           events.APIGatewayV2HTTPRequest
		setupMock         func(*mocks.MockStorage)
		expectedStatus    int
		expectedBodyCheck func(*testing.T, string)
	}{
		{
			name: "GET followers collection metadata",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/followers",
				PathParameters: map[string]string{
					"username": "alice",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowers", mock.Anything, "alice", 1, "").Return([]string{"bob"}, "", nil)
			},
			expectedStatus: http.StatusOK,
			expectedBodyCheck: func(t *testing.T, body string) {
				var collection activitypub.OrderedCollection
				err := json.Unmarshal([]byte(body), &collection)
				assert.NoError(t, err)
				assert.Equal(t, "https://example.com/users/alice/followers", collection.ID)
				assert.Equal(t, activitypub.OrderedCollectionType, collection.Type)
				assert.Equal(t, 1, collection.TotalItems)
				assert.Equal(t, "https://example.com/users/alice/followers?page=true", collection.First)
			},
		},
		{
			name: "GET following collection metadata - empty",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/following",
				PathParameters: map[string]string{
					"username": "alice",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowing", mock.Anything, "alice", 1, "").Return([]string{}, "", nil)
			},
			expectedStatus: http.StatusOK,
			expectedBodyCheck: func(t *testing.T, body string) {
				var collection activitypub.OrderedCollection
				err := json.Unmarshal([]byte(body), &collection)
				assert.NoError(t, err)
				assert.Equal(t, "https://example.com/users/alice/following", collection.ID)
				assert.Equal(t, 0, collection.TotalItems)
				assert.Empty(t, collection.First)
			},
		},
		{
			name: "GET followers page",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/followers",
				PathParameters: map[string]string{
					"username": "alice",
				},
				QueryStringParameters: map[string]string{
					"page": "true",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowers", mock.Anything, "alice", 20, "").Return(
					[]string{"bob", "carol", "dave"}, "nextcursor123", nil)
			},
			expectedStatus: http.StatusOK,
			expectedBodyCheck: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
				assert.Equal(t, "https://example.com/users/alice/followers?page=true", page.ID)
				assert.Equal(t, "OrderedCollectionPage", page.Type)
				assert.Equal(t, "https://example.com/users/alice/followers", page.PartOf)
				assert.Len(t, page.OrderedItems, 3)
				assert.Equal(t, "https://example.com/users/alice/followers?page=true&cursor=nextcursor123&limit=20", page.Next)
			},
		},
		{
			name: "GET following page with cursor",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/following",
				PathParameters: map[string]string{
					"username": "alice",
				},
				QueryStringParameters: map[string]string{
					"page":   "true",
					"cursor": "cursor456",
					"limit":  "10",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowing", mock.Anything, "alice", 10, "cursor456").Return(
					[]string{"eve", "frank"}, "", nil)
			},
			expectedStatus: http.StatusOK,
			expectedBodyCheck: func(t *testing.T, body string) {
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
				assert.Equal(t, "https://example.com/users/alice/following?page=true&cursor=cursor456", page.ID)
				assert.Len(t, page.OrderedItems, 2)
				assert.Empty(t, page.Next)    // No next cursor
				assert.NotEmpty(t, page.Prev) // Has previous
			},
		},
		{
			name: "Unknown collection type",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/unknown",
				PathParameters: map[string]string{
					"username": "alice",
				},
			},
			setupMock:      func(m *mocks.MockStorage) {},
			expectedStatus: http.StatusNotFound,
			expectedBodyCheck: func(t *testing.T, body string) {
				assert.Contains(t, body, "unknown collection")
			},
		},
		{
			name: "Method not allowed",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodPost,
					},
				},
				RawPath: "/users/alice/followers",
				PathParameters: map[string]string{
					"username": "alice",
				},
			},
			setupMock:      func(m *mocks.MockStorage) {},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBodyCheck: func(t *testing.T, body string) {
				assert.Contains(t, body, "METHOD_NOT_ALLOWED")
			},
		},
		{
			name: "Actor not found",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/nonexistent/followers",
				PathParameters: map[string]string{
					"username": "nonexistent",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "nonexistent").Return(nil, common.ActorNotFoundError{Username: "nonexistent"})
			},
			expectedStatus: http.StatusNotFound,
			expectedBodyCheck: func(t *testing.T, body string) {
				assert.Contains(t, body, "NOT_FOUND")
			},
		},
		{
			name: "Storage error",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/followers",
				PathParameters: map[string]string{
					"username": "alice",
				},
				QueryStringParameters: map[string]string{
					"page": "true",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowers", mock.Anything, "alice", 20, "").Return(nil, "", errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBodyCheck: func(t *testing.T, body string) {
				assert.Contains(t, body, "INTERNAL_ERROR")
			},
		},
		{
			name: "Invalid limit parameter",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/followers",
				PathParameters: map[string]string{
					"username": "alice",
				},
				QueryStringParameters: map[string]string{
					"page":  "true",
					"limit": "invalid",
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowers", mock.Anything, "alice", 20, "").Return([]string{}, "", nil) // Default limit
			},
			expectedStatus: http.StatusOK,
			expectedBodyCheck: func(t *testing.T, body string) {
				// Should still work with default limit
				var page activitypub.OrderedCollectionPage
				err := json.Unmarshal([]byte(body), &page)
				assert.NoError(t, err)
			},
		},
		{
			name: "Limit exceeds maximum",
			request: events.APIGatewayV2HTTPRequest{
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: http.MethodGet,
					},
				},
				RawPath: "/users/alice/followers",
				PathParameters: map[string]string{
					"username": "alice",
				},
				QueryStringParameters: map[string]string{
					"page":  "true",
					"limit": "200", // Over 100
				},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
				m.On("GetFollowers", mock.Anything, "alice", 20, "").Return([]string{}, "", nil) // Default limit
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage
			mockStore := new(mocks.MockStorage)
			tt.setupMock(mockStore)
			store = mockStore

			// Call handler
			resp, err := handler(context.Background(), tt.request)
			assert.NoError(t, err)

			// Check status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Check body if provided
			if tt.expectedBodyCheck != nil {
				tt.expectedBodyCheck(t, resp.Body)
			}

			// Verify all expectations were met
			mockStore.AssertExpectations(t)
		})
	}
}

func TestReturnCollection(t *testing.T) {
	cfg = &config.Config{
		Domain: "https://example.com",
	}

	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		PreferredUsername: "alice",
	}

	t.Run("followers with items", func(t *testing.T) {
		mockStore := new(mocks.MockStorage)
		mockStore.On("GetFollowers", mock.Anything, "alice", 1, "").Return([]string{"bob"}, "", nil)
		store = mockStore

		resp, err := returnCollection(context.Background(), testActor, "followers")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var collection activitypub.OrderedCollection
		err = json.Unmarshal([]byte(resp.Body), &collection)
		assert.NoError(t, err)
		assert.Equal(t, 1, collection.TotalItems)
		assert.NotEmpty(t, collection.First)
	})

	t.Run("following empty", func(t *testing.T) {
		mockStore := new(mocks.MockStorage)
		mockStore.On("GetFollowing", mock.Anything, "alice", 1, "").Return([]string{}, "", nil)
		store = mockStore

		resp, err := returnCollection(context.Background(), testActor, "following")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var collection activitypub.OrderedCollection
		err = json.Unmarshal([]byte(resp.Body), &collection)
		assert.NoError(t, err)
		assert.Equal(t, 0, collection.TotalItems)
		assert.Empty(t, collection.First)
	})
}

func TestReturnCollectionPage(t *testing.T) {
	cfg = &config.Config{
		Domain: "https://example.com",
	}

	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		PreferredUsername: "alice",
	}

	t.Run("page with next cursor", func(t *testing.T) {
		usernames := []string{"bob", "carol", "dave"}
		resp, err := returnCollectionPage(context.Background(), testActor, "followers", usernames, nil, "", "nextcursor", 20)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var page activitypub.OrderedCollectionPage
		err = json.Unmarshal([]byte(resp.Body), &page)
		assert.NoError(t, err)
		assert.Len(t, page.OrderedItems, 3)
		assert.Contains(t, page.Next, "nextcursor")
		assert.Empty(t, page.Prev)
	})

	t.Run("page with cursor (not first page)", func(t *testing.T) {
		usernames := []string{"eve", "frank"}
		resp, err := returnCollectionPage(context.Background(), testActor, "following", usernames, nil, "prevcursor", "", 10)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var page activitypub.OrderedCollectionPage
		err = json.Unmarshal([]byte(resp.Body), &page)
		assert.NoError(t, err)
		assert.Len(t, page.OrderedItems, 2)
		assert.Empty(t, page.Next)
		assert.NotEmpty(t, page.Prev)
	})
}
