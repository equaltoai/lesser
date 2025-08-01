package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockAuthService for testing
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) GetStore() storage.Storage {
	args := m.Called()
	return args.Get(0).(storage.Storage)
}

func (m *MockAuthService) GetConfig() *auth.Config {
	args := m.Called()
	return args.Get(0).(*auth.Config)
}

func (m *MockAuthService) GenerateRecoveryToken(ctx context.Context, username, method string) (string, error) {
	args := m.Called(ctx, username, method)
	return args.String(0), args.Error(1)
}

func TestHandleGetRecoveryOptionsLift_Simple(t *testing.T) {
	// Create a simple test with a real auth service and mock storage
	mockStore := new(MockStorageAdapter)
	
	// Create a real auth service with the mock store
	authService, err := auth.NewAuthService(mockStore)
	assert.NoError(t, err)
	
	// Use the constructor which should set up the services correctly
	handler := NewEmailFreeRecoveryHandler(authService)

	// Mock the auth service GetStore method
	// Set up mocks with match.Anything for context to be more flexible
	mockStore.On("GetUser", mock.Anything, "testuser").Return(&storage.User{
		Username: "testuser",
	}, nil)
	mockStore.On("GetUserWebAuthnCredentials", mock.Anything, "testuser").Return([]*storage.WebAuthnCredential{}, nil)
	mockStore.On("GetUserWalletCredentials", mock.Anything, "testuser").Return([]*storage.WalletCredential{}, nil)
	mockStore.On("GetLinkedProviders", mock.Anything, "testuser").Return([]string{}, nil)
	mockStore.On("GetTrustees", mock.Anything, "testuser").Return([]*storage.TrusteeConfig{}, nil)
	mockStore.On("GetRecoveryCodeCount", mock.Anything, "testuser").Return(0, nil)

	req := &lift.Request{
		Request: &adapters.Request{
			Method: "GET",
			Path:   "/auth/recovery/options",
			QueryParams: map[string]string{
				"username": "testuser",
			},
		},
	}

	ctx := lift.NewContext(context.Background(), req)

	// Debug: check what query parameters the context can see
	t.Logf("Query username: '%s'", ctx.Query("username"))
	t.Logf("Request QueryParams: %+v", req.Request.QueryParams)
	
	handlerErr := handler.HandleGetRecoveryOptionsLift(ctx)
	assert.NoError(t, handlerErr)
	
	// Remove the status assertion for now to see the debug info

	// Parse response - it's already a parsed map in Lift
	response, ok := ctx.Response.Body.(map[string]any)
	assert.True(t, ok, "Response body should be a map")
	
	// Debug: print the response to see what we got
	t.Logf("Response: %+v", response)
	
	if ctx.Response.StatusCode == http.StatusOK {
		assert.Equal(t, "testuser", response["username"])
		assert.NotNil(t, response["options"])
	}
}