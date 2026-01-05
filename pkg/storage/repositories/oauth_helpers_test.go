package repositories

import (
	"context"
	"testing"
	"time"

	dynmormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ============================================================================
// Pure Function Tests: NormalizeRedirectURI
// ============================================================================

func TestNormalizeRedirectURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "simple_uri_no_change",
			uri:      "https://example.com/callback",
			expected: "https://example.com/callback",
		},
		{
			name:     "removes_trailing_slash",
			uri:      "https://example.com/callback/",
			expected: "https://example.com/callback",
		},
		{
			name:     "removes_multiple_trailing_slashes",
			uri:      "https://example.com/callback///",
			expected: "https://example.com/callback",
		},
		{
			name:     "lowercases_scheme",
			uri:      "HTTPS://example.com/callback",
			expected: "https://example.com/callback",
		},
		{
			name:     "lowercases_host",
			uri:      "https://EXAMPLE.COM/callback",
			expected: "https://example.com/callback",
		},
		{
			name:     "preserves_path_case",
			uri:      "https://example.com/CallBack",
			expected: "https://example.com/CallBack",
		},
		{
			name:     "preserves_query_parameters",
			uri:      "https://example.com/callback?state=abc",
			expected: "https://example.com/callback?state=abc",
		},
		{
			name:     "preserves_fragment",
			uri:      "https://example.com/callback#section",
			expected: "https://example.com/callback#section",
		},
		{
			name:     "handles_http_scheme",
			uri:      "HTTP://LOCALHOST:8080/oauth/callback",
			expected: "http://localhost:8080/oauth/callback",
		},
		{
			name:     "handles_custom_port",
			uri:      "https://EXAMPLE.COM:8443/callback",
			expected: "https://example.com:8443/callback",
		},
		{
			name:     "handles_localhost",
			uri:      "http://LOCALHOST:3000/",
			expected: "http://localhost:3000",
		},
		{
			name:     "handles_uri_without_path",
			uri:      "https://EXAMPLE.COM",
			expected: "https://example.com",
		},
		{
			name:     "handles_uri_without_scheme_separator",
			uri:      "noscheme.com/callback",
			expected: "noscheme.com/callback",
		},
		{
			name:     "handles_empty_string",
			uri:      "",
			expected: "",
		},
		{
			name:     "handles_just_trailing_slash",
			uri:      "/",
			expected: "",
		},
		{
			name:     "handles_complex_path",
			uri:      "https://APP.Example.COM/auth/oauth2/callback",
			expected: "https://app.example.com/auth/oauth2/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeRedirectURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Pure Function Tests: ValidateRedirectURI
// ============================================================================

func TestValidateRedirectURI(t *testing.T) {
	tests := []struct {
		name           string
		registeredURIs []string
		redirectURI    string
		expected       bool
	}{
		{
			name:           "exact_match",
			registeredURIs: []string{"https://example.com/callback"},
			redirectURI:    "https://example.com/callback",
			expected:       true,
		},
		{
			name:           "match_with_trailing_slash_normalization",
			registeredURIs: []string{"https://example.com/callback/"},
			redirectURI:    "https://example.com/callback",
			expected:       true,
		},
		{
			name:           "match_ignoring_scheme_case",
			registeredURIs: []string{"HTTPS://example.com/callback"},
			redirectURI:    "https://example.com/callback",
			expected:       true,
		},
		{
			name:           "match_ignoring_host_case",
			registeredURIs: []string{"https://EXAMPLE.COM/callback"},
			redirectURI:    "https://example.com/callback",
			expected:       true,
		},
		{
			name:           "no_match_different_path",
			registeredURIs: []string{"https://example.com/callback"},
			redirectURI:    "https://example.com/other",
			expected:       false,
		},
		{
			name:           "no_match_different_host",
			registeredURIs: []string{"https://example.com/callback"},
			redirectURI:    "https://attacker.com/callback",
			expected:       false,
		},
		{
			name:           "match_in_multiple_registered",
			registeredURIs: []string{"https://example.com/one", "https://example.com/two", "https://example.com/callback"},
			redirectURI:    "https://example.com/callback",
			expected:       true,
		},
		{
			name:           "no_match_empty_registered",
			registeredURIs: []string{},
			redirectURI:    "https://example.com/callback",
			expected:       false,
		},
		{
			name:           "match_with_port",
			registeredURIs: []string{"http://localhost:8080/callback"},
			redirectURI:    "http://LOCALHOST:8080/callback",
			expected:       true,
		},
		{
			name:           "no_match_different_port",
			registeredURIs: []string{"http://localhost:8080/callback"},
			redirectURI:    "http://localhost:9090/callback",
			expected:       false,
		},
		{
			name:           "no_match_different_scheme",
			registeredURIs: []string{"https://example.com/callback"},
			redirectURI:    "http://example.com/callback",
			expected:       false,
		},
		{
			name:           "match_preserves_path_case_sensitivity",
			registeredURIs: []string{"https://example.com/CallBack"},
			redirectURI:    "https://example.com/CallBack",
			expected:       true,
		},
		{
			name:           "no_match_path_case_mismatch",
			registeredURIs: []string{"https://example.com/CallBack"},
			redirectURI:    "https://example.com/callback",
			expected:       false,
		},
		{
			name:           "match_with_query_string",
			registeredURIs: []string{"https://example.com/callback?state=fixed"},
			redirectURI:    "https://example.com/callback?state=fixed",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateRedirectURI(tt.registeredURIs, tt.redirectURI)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// DB Flow Tests: StoreOAuthStateGeneric
// ============================================================================

func TestStoreOAuthStateGeneric_DefaultExpiresAt(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	state := "test-state-123"
	data := &storage.OAuthState{
		State:       state,
		Provider:    "mastodon",
		RedirectURI: "https://example.com/callback",
		ClientID:    "client-123",
		// ExpiresAt is intentionally zero to test default behavior
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	beforeCall := time.Now()
	err := helper.StoreOAuthStateGeneric(ctx, state, data)
	afterCall := time.Now()

	// Assert
	require.NoError(t, err)

	// Verify ExpiresAt was set to approximately 10 minutes from now
	assert.False(t, data.ExpiresAt.IsZero(), "ExpiresAt should be set")
	expectedMin := beforeCall.Add(9 * time.Minute)
	expectedMax := afterCall.Add(11 * time.Minute)
	assert.True(t, data.ExpiresAt.After(expectedMin) && data.ExpiresAt.Before(expectedMax),
		"ExpiresAt should be ~10 minutes from now")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestStoreOAuthStateGeneric_PreserveExistingExpiresAt(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	customExpiry := time.Now().Add(30 * time.Minute)
	data := &storage.OAuthState{
		State:       "test-state-456",
		Provider:    "mastodon",
		RedirectURI: "https://example.com/callback",
		ExpiresAt:   customExpiry,
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := helper.StoreOAuthStateGeneric(ctx, "test-state-456", data)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, customExpiry, data.ExpiresAt, "ExpiresAt should not be modified")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestStoreOAuthStateGeneric_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	data := &storage.OAuthState{
		State:    "test-state-789",
		Provider: "mastodon",
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	// Execute
	err := helper.StoreOAuthStateGeneric(ctx, "test-state-789", data)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OAuth state")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// DB Flow Tests: GetOAuthStateGeneric
// ============================================================================

func TestGetOAuthStateGeneric_Found(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	stateValue := "test-state-abc"
	futureExpiry := time.Now().Add(5 * time.Minute)

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_STATE#test-state-abc").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "STATE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.OAuthState")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.OAuthState)
		model.State = stateValue
		model.Provider = "mastodon"
		model.RedirectURI = "https://example.com/callback"
		model.ClientID = "client-123"
		model.Scopes = []string{"read", "write"}
		model.ExpiresAt = futureExpiry
	}).Return(nil)

	// Execute
	result, err := helper.GetOAuthStateGeneric(ctx, stateValue)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, stateValue, result.State)
	assert.Equal(t, "mastodon", result.Provider)
	assert.Equal(t, "https://example.com/callback", result.RedirectURI)
	assert.Equal(t, "client-123", result.ClientID)
	assert.Equal(t, []string{"read", "write"}, result.Scopes)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetOAuthStateGeneric_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_STATE#nonexistent").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "STATE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.OAuthState")).Return(dynmormerrors.ErrItemNotFound)

	// Execute
	result, err := helper.GetOAuthStateGeneric(ctx, "nonexistent")

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetOAuthStateGeneric_ExpiredTriggersDelete(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	deleteQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	pastExpiry := time.Now().Add(-5 * time.Minute) // Expired 5 minutes ago

	// Set up expectations for Get
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "OAUTH_STATE#expired-state").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "STATE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.OAuthState")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.OAuthState)
		model.State = "expired-state"
		model.ExpiresAt = pastExpiry
	}).Return(nil).Once()

	// Set up expectations for Delete (triggered by expiration)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthState")).Return(deleteQuery).Once()
	deleteQuery.On("Delete").Return(nil)

	// Execute
	result, err := helper.GetOAuthStateGeneric(ctx, "expired-state")

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
}

// ============================================================================
// DB Flow Tests: CreateOAuthClientGeneric
// ============================================================================

func TestCreateOAuthClientGeneric_GeneratesSecret(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	client := &storage.OAuthClient{
		ClientID:     "new-client-123",
		Name:         "Test Application",
		RedirectURIs: []string{"https://example.com/callback"},
		// ClientSecret intentionally empty to test generation
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := helper.CreateOAuthClientGeneric(ctx, client)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, client.ClientSecret, "ClientSecret should be generated")
	// Base64 URL encoded 32 bytes = 43 characters
	assert.GreaterOrEqual(t, len(client.ClientSecret), 40, "ClientSecret should be sufficiently long")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateOAuthClientGeneric_PreservesProvidedSecret(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	existingSecret := "my-custom-secret-12345"
	client := &storage.OAuthClient{
		ClientID:     "new-client-456",
		ClientSecret: existingSecret,
		Name:         "Test Application",
		RedirectURIs: []string{"https://example.com/callback"},
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := helper.CreateOAuthClientGeneric(ctx, client)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, existingSecret, client.ClientSecret, "ClientSecret should not be changed")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateOAuthClientGeneric_SetsTimestamps(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	client := &storage.OAuthClient{
		ClientID:     "new-client-789",
		ClientSecret: "test-secret",
		Name:         "Test Application",
		// CreatedAt and UpdatedAt are intentionally zero
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	beforeCall := time.Now()
	err := helper.CreateOAuthClientGeneric(ctx, client)
	afterCall := time.Now()

	// Assert
	require.NoError(t, err)
	assert.False(t, client.CreatedAt.IsZero(), "CreatedAt should be set")
	assert.False(t, client.UpdatedAt.IsZero(), "UpdatedAt should be set")
	assert.True(t, client.CreatedAt.After(beforeCall.Add(-time.Second)) && client.CreatedAt.Before(afterCall.Add(time.Second)))
	assert.True(t, client.UpdatedAt.After(beforeCall.Add(-time.Second)) && client.UpdatedAt.Before(afterCall.Add(time.Second)))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateOAuthClientGeneric_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	client := &storage.OAuthClient{
		ClientID:     "fail-client",
		ClientSecret: "test-secret",
		Name:         "Failing Application",
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	// Execute
	err := helper.CreateOAuthClientGeneric(ctx, client)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OAuth client")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// DB Flow Tests: UpdateOAuthClientGeneric
// ============================================================================

func TestUpdateOAuthClientGeneric_AppliesUpdates(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	clientID := "existing-client-123"

	// Set up expectations for Get
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#existing-client-123").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "CLIENT").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.OAuthClient)
		model.ClientID = clientID
		model.ClientSecret = "original-secret"
		model.Name = "Original Name"
		model.Description = "Original Description"
		model.RedirectURIs = []string{"https://old.example.com/callback"}
		model.CreatedAt = time.Now().Add(-24 * time.Hour)
	}).Return(nil).Once()

	// Set up expectations for Update
	mockQuery.On("Update", mock.AnythingOfType("[]string")).Return(nil).Once()

	// Execute
	updates := map[string]interface{}{
		"name":          "Updated Name",
		"description":   "Updated Description",
		"redirect_uris": []string{"https://new.example.com/callback"},
		"website":       "https://example.com",
		"confidential":  true,
	}
	err := helper.UpdateOAuthClientGeneric(ctx, clientID, updates)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpdateOAuthClientGeneric_ClientNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#nonexistent").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "CLIENT").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Return(dynmormerrors.ErrItemNotFound)

	// Execute
	err := helper.UpdateOAuthClientGeneric(ctx, "nonexistent", map[string]interface{}{"name": "New Name"})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpdateOAuthClientGeneric_UpdateSpecificFields(t *testing.T) {
	tests := []struct {
		name    string
		updates map[string]interface{}
	}{
		{
			name:    "update_name_only",
			updates: map[string]interface{}{"name": "New Name"},
		},
		{
			name:    "update_grant_types",
			updates: map[string]interface{}{"grant_types": []string{"authorization_code", "refresh_token"}},
		},
		{
			name:    "update_scopes",
			updates: map[string]interface{}{"scopes": []string{"read", "write", "follow"}},
		},
		{
			name:    "update_confidential_to_true",
			updates: map[string]interface{}{"confidential": true},
		},
		{
			name:    "update_confidential_to_false",
			updates: map[string]interface{}{"confidential": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			logger := zap.NewNop()
			helper := NewOAuthHelper(mockDB, logger)

			ctx := context.Background()
			clientID := "test-client"

			// Set up expectations for Get
			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
			mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#test-client").Return(mockQuery).Once()
			mockQuery.On("Where", "SK", "=", "CLIENT").Return(mockQuery).Once()
			mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Run(func(args mock.Arguments) {
				model := args.Get(0).(*models.OAuthClient)
				model.ClientID = clientID
				model.ClientSecret = "secret"
				model.Name = "Original"
			}).Return(nil).Once()

			// Set up expectations for Update
			mockQuery.On("Update", mock.AnythingOfType("[]string")).Return(nil).Once()

			// Execute
			err := helper.UpdateOAuthClientGeneric(ctx, clientID, tt.updates)

			// Assert
			require.NoError(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

// ============================================================================
// DB Flow Tests: ListOAuthClientsGeneric
// ============================================================================

func TestListOAuthClientsGeneric_ReturnsClients(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	ownerID := "owner-123"
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*[]models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "OWNER#owner-123").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.OAuthClient")).Run(func(args mock.Arguments) {
		clients := args.Get(0).(*[]models.OAuthClient)
		*clients = []models.OAuthClient{
			{
				ClientID:     "client-1",
				ClientSecret: "secret-1",
				Name:         "App One",
				OwnerID:      ownerID,
			},
			{
				ClientID:     "client-2",
				ClientSecret: "secret-2",
				Name:         "App Two",
				OwnerID:      ownerID,
			},
		}
	}).Return(nil)

	// Execute
	clients, cursor, err := helper.ListOAuthClientsGeneric(ctx, ownerID, limit)

	// Assert
	require.NoError(t, err)
	assert.Len(t, clients, 2)
	assert.Equal(t, "client-1", clients[0].ClientID)
	assert.Equal(t, "client-2", clients[1].ClientID)
	assert.Empty(t, cursor, "No cursor when results < limit")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListOAuthClientsGeneric_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	ownerID := "owner-no-clients"
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*[]models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "OWNER#owner-no-clients").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.OAuthClient")).Run(func(args mock.Arguments) {
		clients := args.Get(0).(*[]models.OAuthClient)
		*clients = []models.OAuthClient{}
	}).Return(nil)

	// Execute
	clients, cursor, err := helper.ListOAuthClientsGeneric(ctx, ownerID, limit)

	// Assert
	require.NoError(t, err)
	assert.Len(t, clients, 0)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListOAuthClientsGeneric_HasMoreCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	ownerID := "owner-many"
	limit := 2

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*[]models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "OWNER#owner-many").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.OAuthClient")).Run(func(args mock.Arguments) {
		clients := args.Get(0).(*[]models.OAuthClient)
		// Return exactly `limit` items to indicate there might be more
		*clients = []models.OAuthClient{
			{ClientID: "client-1", Name: "App One", OwnerID: ownerID},
			{ClientID: "client-2", Name: "App Two", OwnerID: ownerID},
		}
	}).Return(nil)

	// Execute
	clients, cursor, err := helper.ListOAuthClientsGeneric(ctx, ownerID, limit)

	// Assert
	require.NoError(t, err)
	assert.Len(t, clients, 2)
	assert.Equal(t, "has_more", cursor, "Cursor should indicate more results")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListOAuthClientsGeneric_ZeroLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	ownerID := "owner-unlimited"
	limit := 0 // No limit

	// Set up expectations - when limit is 0, Limit should not be called
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*[]models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "OWNER#owner-unlimited").Return(mockQuery)
	// Note: Limit is NOT called when limit <= 0
	mockQuery.On("All", mock.AnythingOfType("*[]models.OAuthClient")).Run(func(args mock.Arguments) {
		clients := args.Get(0).(*[]models.OAuthClient)
		*clients = []models.OAuthClient{
			{ClientID: "client-1", Name: "App One"},
			{ClientID: "client-2", Name: "App Two"},
			{ClientID: "client-3", Name: "App Three"},
		}
	}).Return(nil)

	// Execute
	clients, cursor, err := helper.ListOAuthClientsGeneric(ctx, ownerID, limit)

	// Assert
	require.NoError(t, err)
	assert.Len(t, clients, 3)
	assert.Empty(t, cursor, "No cursor when limit is 0")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListOAuthClientsGeneric_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*[]models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "OWNER#owner-error").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.OAuthClient")).Return(ErrTestMockError)

	// Execute
	clients, cursor, err := helper.ListOAuthClientsGeneric(ctx, "owner-error", 10)

	// Assert
	require.Error(t, err)
	assert.Nil(t, clients)
	assert.Empty(t, cursor)
	assert.Contains(t, err.Error(), "OAuth client")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Helper: NewOAuthHelper test
// ============================================================================

func TestNewOAuthHelper(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	helper := NewOAuthHelper(mockDB, logger)

	assert.NotNil(t, helper)
	assert.Equal(t, mockDB, helper.db)
	assert.Equal(t, logger, helper.logger)
}
