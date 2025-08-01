package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetPollLift(t *testing.T) {
	tests := []struct {
		name           string
		pollID         string
		authHeader     string
		testUsername   string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		checkResponse  func(*testing.T, interface{})
	}{
		{
			name:   "successful poll retrieval without auth",
			pollID: "poll123",
			setupMocks: func(m *MockStorageAdapter) {
				expiresAt := time.Now().Add(24 * time.Hour)
				m.On("GetPoll", mock.Anything, "poll123").Return(&storage.Poll{
					ID:          "poll123",
					StatusID:    "status123",
					Options:     []string{"Option A", "Option B", "Option C"},
					ExpiresAt:   expiresAt,
					Multiple:    false,
					HideTotals:  false,
					VotesCount:  5,
					VotersCount: 5,
					Votes: map[string][]int{
						"user1": {0},
						"user2": {1},
						"user3": {1},
						"user4": {2},
						"user5": {0},
					},
					CreatedBy: "https://example.com/users/creator",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				poll, ok := resp.(models.Poll)
				assert.True(t, ok)
				assert.Equal(t, "poll123", poll.ID)
				assert.Equal(t, 5, poll.VotesCount)
				assert.Equal(t, 5, poll.VotersCount)
				assert.False(t, poll.Voted)
				assert.False(t, poll.Expired)
				assert.Len(t, poll.OptionsData, 3)
				assert.Equal(t, 2, poll.OptionsData[0].VotesCount)
				assert.Equal(t, 2, poll.OptionsData[1].VotesCount)
				assert.Equal(t, 1, poll.OptionsData[2].VotesCount)
			},
		},
		{
			name:         "successful poll retrieval with test user who voted",
			pollID:       "poll123",
			testUsername: "testuser",
			setupMocks: func(m *MockStorageAdapter) {
				// Mock getting test user's actor
				m.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
				}, nil)

				expiresAt := time.Now().Add(24 * time.Hour)
				m.On("GetPoll", mock.Anything, "poll123").Return(&storage.Poll{
					ID:          "poll123",
					StatusID:    "status123",
					Options:     []string{"Option A", "Option B"},
					ExpiresAt:   expiresAt,
					Multiple:    true,
					HideTotals:  false,
					VotesCount:  3,
					VotersCount: 2,
					Votes: map[string][]int{
						"https://example.com/users/testuser": {0, 1},
						"user2":                              {1},
					},
					CreatedBy: "https://example.com/users/creator",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				poll, ok := resp.(models.Poll)
				assert.True(t, ok)
				assert.True(t, poll.Voted)
				assert.Equal(t, []int{0, 1}, poll.OwnVotes)
				assert.True(t, poll.Multiple)
			},
		},
		{
			name:   "poll with hidden totals not expired",
			pollID: "poll123",
			setupMocks: func(m *MockStorageAdapter) {
				expiresAt := time.Now().Add(24 * time.Hour)
				m.On("GetPoll", mock.Anything, "poll123").Return(&storage.Poll{
					ID:          "poll123",
					StatusID:    "status123",
					Options:     []string{"Option A", "Option B"},
					ExpiresAt:   expiresAt,
					Multiple:    false,
					HideTotals:  true,
					VotesCount:  10,
					VotersCount: 10,
					Votes: map[string][]int{
						"user1": {0},
						"user2": {1},
					},
					CreatedBy: "https://example.com/users/creator",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				poll, ok := resp.(models.Poll)
				assert.True(t, ok)
				// Totals should be hidden
				assert.Equal(t, 0, poll.VotesCount)
				assert.Equal(t, 0, poll.VotersCount)
				for _, option := range poll.OptionsData {
					assert.Equal(t, 0, option.VotesCount)
				}
			},
		},
		{
			name:   "expired poll shows totals even if hidden",
			pollID: "poll123",
			setupMocks: func(m *MockStorageAdapter) {
				expiresAt := time.Now().Add(-1 * time.Hour) // Expired
				m.On("GetPoll", mock.Anything, "poll123").Return(&storage.Poll{
					ID:          "poll123",
					StatusID:    "status123",
					Options:     []string{"Option A", "Option B"},
					ExpiresAt:   expiresAt,
					Multiple:    false,
					HideTotals:  true,
					VotesCount:  5,
					VotersCount: 5,
					Votes: map[string][]int{
						"user1": {0},
						"user2": {0},
						"user3": {1},
						"user4": {1},
						"user5": {1},
					},
					CreatedBy: "https://example.com/users/creator",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				poll, ok := resp.(models.Poll)
				assert.True(t, ok)
				assert.True(t, poll.Expired)
				// Totals should be shown for expired polls
				assert.Equal(t, 5, poll.VotesCount)
				assert.Equal(t, 5, poll.VotersCount)
				assert.Equal(t, 2, poll.OptionsData[0].VotesCount)
				assert.Equal(t, 3, poll.OptionsData[1].VotesCount)
			},
		},
		{
			name:   "poll not found",
			pollID: "nonexistent",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetPoll", mock.Anything, "nonexistent").Return(nil, fmt.Errorf("poll not found"))
			},
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "poll not found", errResp["error"])
			},
		},
		{
			name:           "missing poll ID",
			pollID:         "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "poll ID required", errResp["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStore)
			}

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   fmt.Sprintf("/api/v1/polls/%s", tt.pollID),
					Headers: map[string]string{
						"Authorization":   tt.authHeader,
						"X-Test-Username": tt.testUsername,
					},
					PathParams: map[string]string{"id": tt.pollID},
				},
			}
			
			ctx := lift.NewContext(context.Background(), req)
			ctx.SetParam("id", tt.pollID)

			err := handler.HandleGetPollLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx.Response.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleVoteOnPollLift(t *testing.T) {
	tests := []struct {
		name           string
		pollID         string
		authHeader     string
		testUsername   string
		requestBody    interface{}
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		checkResponse  func(*testing.T, interface{})
	}{
		{
			name:         "successful vote submission",
			pollID:       "poll123",
			testUsername: "testuser",
			requestBody: models.PollVoteRequest{
				Choices: []int{1},
			},
			setupMocks: func(m *MockStorageAdapter) {
				// Mock getting test user's actor
				m.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
				}, nil)

				// Mock vote submission
				m.On("VoteOnPoll", mock.Anything, "poll123", "https://example.com/users/testuser", []int{1}).Return(nil)

				// Mock getting updated poll
				expiresAt := time.Now().Add(24 * time.Hour)
				m.On("GetPoll", mock.Anything, "poll123").Return(&storage.Poll{
					ID:          "poll123",
					StatusID:    "status123",
					Options:     []string{"Option A", "Option B"},
					ExpiresAt:   expiresAt,
					Multiple:    false,
					HideTotals:  false,
					VotesCount:  6,
					VotersCount: 6,
					Votes: map[string][]int{
						"https://example.com/users/testuser": {1},
						"user2":                              {0},
						"user3":                              {1},
						"user4":                              {0},
						"user5":                              {1},
						"user6":                              {0},
					},
					CreatedBy: "https://example.com/users/creator",
				}, nil)

				// Mock notification creation
				m.On("CreateNotification", mock.Anything, mock.MatchedBy(func(n *storage.Notification) bool {
					return n.Type == "poll" &&
						n.Username == "creator" &&
						n.AccountID == "https://example.com/users/testuser" &&
						n.StatusID == "status123"
				})).Return(nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				poll, ok := resp.(models.Poll)
				assert.True(t, ok)
				assert.Equal(t, "poll123", poll.ID)
				assert.True(t, poll.Voted)
				assert.Equal(t, []int{1}, poll.OwnVotes)
				assert.Equal(t, 6, poll.VotesCount)
				assert.Equal(t, 3, poll.OptionsData[0].VotesCount)
				assert.Equal(t, 3, poll.OptionsData[1].VotesCount)
			},
		},
		{
			name:         "multiple choice vote",
			pollID:       "poll123",
			testUsername: "testuser",
			requestBody: models.PollVoteRequest{
				Choices: []int{0, 2},
			},
			setupMocks: func(m *MockStorageAdapter) {
				// Mock getting test user's actor
				m.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
				}, nil)

				// Mock vote submission
				m.On("VoteOnPoll", mock.Anything, "poll123", "https://example.com/users/testuser", []int{0, 2}).Return(nil)

				// Mock getting updated poll
				expiresAt := time.Now().Add(24 * time.Hour)
				m.On("GetPoll", mock.Anything, "poll123").Return(&storage.Poll{
					ID:          "poll123",
					StatusID:    "status123",
					Options:     []string{"Option A", "Option B", "Option C"},
					ExpiresAt:   expiresAt,
					Multiple:    true,
					HideTotals:  false,
					VotesCount:  3,
					VotersCount: 2,
					Votes: map[string][]int{
						"https://example.com/users/testuser": {0, 2},
						"user2":                              {1},
					},
					CreatedBy: "https://example.com/users/testuser", // Self vote - no notification
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				poll, ok := resp.(models.Poll)
				assert.True(t, ok)
				assert.True(t, poll.Multiple)
				assert.Equal(t, []int{0, 2}, poll.OwnVotes)
			},
		},
		{
			name:           "missing authentication",
			pollID:         "poll123",
			authHeader:     "",
			testUsername:   "",
			requestBody:    models.PollVoteRequest{Choices: []int{0}},
			expectedStatus: http.StatusUnauthorized,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "unauthorized", errResp["error"])
			},
		},
		{
			name:         "invalid request body",
			pollID:       "poll123",
			testUsername: "testuser",
			requestBody:  "invalid json",
			setupMocks: func(m *MockStorageAdapter) {
				// Mock getting test user's actor
				m.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
				}, nil)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "invalid request body", errResp["error"])
			},
		},
		{
			name:         "no choices provided",
			pollID:       "poll123",
			testUsername: "testuser",
			requestBody: models.PollVoteRequest{
				Choices: []int{},
			},
			setupMocks: func(m *MockStorageAdapter) {
				// Mock getting test user's actor
				m.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
				}, nil)
			},
			expectedStatus: http.StatusUnprocessableEntity,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "no choices provided", errResp["error"])
			},
		},
		{
			name:         "vote submission failure",
			pollID:       "poll123",
			testUsername: "testuser",
			requestBody: models.PollVoteRequest{
				Choices: []int{0},
			},
			setupMocks: func(m *MockStorageAdapter) {
				// Mock getting test user's actor
				m.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
				}, nil)

				// Mock vote submission failure
				m.On("VoteOnPoll", mock.Anything, "poll123", "https://example.com/users/testuser", []int{0}).Return(fmt.Errorf("already voted"))
			},
			expectedStatus: http.StatusUnprocessableEntity,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "already voted", errResp["error"])
			},
		},
		{
			name:           "missing poll ID",
			pollID:         "",
			testUsername:   "testuser",
			requestBody:    models.PollVoteRequest{Choices: []int{0}},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp interface{}) {
				errResp, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "poll ID required", errResp["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStore)
			}

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			// Convert request body based on type
			var bodyBytes []byte
			if tt.requestBody != nil {
				switch body := tt.requestBody.(type) {
				case string:
					bodyBytes = []byte(body)
				case []byte:
					bodyBytes = body
				default:
					bodyBytes, _ = json.Marshal(tt.requestBody)
				}
			}
			
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "POST",
					Path:   fmt.Sprintf("/api/v1/polls/%s/votes", tt.pollID),
					Headers: map[string]string{
						"Authorization":   tt.authHeader,
						"X-Test-Username": tt.testUsername,
					},
					PathParams: map[string]string{"id": tt.pollID},
				},
				Body: bodyBytes,
			}
			
			ctx := lift.NewContext(context.Background(), req)
			ctx.SetParam("id", tt.pollID)

			err := handler.HandleVoteOnPollLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx.Response.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestExtractUsernameFromActorIDLift(t *testing.T) {
	tests := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "standard actor ID",
			actorID:  "https://example.com/users/johndoe",
			expected: "johndoe",
		},
		{
			name:     "actor ID with port",
			actorID:  "https://example.com:8080/users/janedoe",
			expected: "janedoe",
		},
		{
			name:     "actor ID with subdomain",
			actorID:  "https://social.example.com/users/testuser",
			expected: "testuser",
		},
		{
			name:     "empty actor ID",
			actorID:  "",
			expected: "",
		},
		{
			name:     "malformed actor ID",
			actorID:  "not-a-url",
			expected: "not-a-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameFromActorIDLift(tt.actorID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidEmojiCodeLift(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name:     "valid emoji code",
			code:     "custom_emoji",
			expected: true,
		},
		{
			name:     "valid emoji code with numbers",
			code:     "emoji123",
			expected: true,
		},
		{
			name:     "valid emoji code uppercase",
			code:     "EMOJI_CODE",
			expected: true,
		},
		{
			name:     "too short",
			code:     "e",
			expected: false,
		},
		{
			name:     "too long",
			code:     "this_is_a_very_long_emoji_code_that_exceeds_limit",
			expected: false,
		},
		{
			name:     "contains invalid characters",
			code:     "emoji-code",
			expected: false,
		},
		{
			name:     "contains spaces",
			code:     "emoji code",
			expected: false,
		},
		{
			name:     "empty code",
			code:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidEmojiCodeLift(tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindEmojiCodesLift(t *testing.T) {
	handler := &Handler{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "single emoji code",
			text:     "This is :custom_emoji: text",
			expected: []string{"custom_emoji"},
		},
		{
			name:     "multiple emoji codes",
			text:     ":emoji1: and :emoji2: and :emoji3:",
			expected: []string{"emoji1", "emoji2", "emoji3"},
		},
		{
			name:     "no emoji codes",
			text:     "This is plain text",
			expected: []string{},
		},
		{
			name:     "incomplete emoji code",
			text:     "This is :incomplete",
			expected: []string{},
		},
		{
			name:     "nested colons",
			text:     "This is ::double:: colons",
			expected: []string{},
		},
		{
			name:     "invalid emoji code",
			text:     "This is :invalid-emoji: code",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.findEmojiCodesLift(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}