package testing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// Mock Auth Service

// MockAuthService provides a mock implementation of the auth service
type MockAuthService struct {
	mock.Mock
}

// ValidateAccessToken mocks token validation
func (m *MockAuthService) ValidateAccessToken(token string) (*auth.EnhancedClaims, error) {
	args := m.Called(token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*auth.EnhancedClaims), args.Error(1)
}

// GenerateAccessToken mocks token generation
func (m *MockAuthService) GenerateAccessToken(claims *auth.Claims) (string, error) {
	args := m.Called(claims)
	return args.String(0), args.Error(1)
}

// RefreshToken mocks token refresh
func (m *MockAuthService) RefreshToken(refreshToken string) (string, error) {
	args := m.Called(refreshToken)
	return args.String(0), args.Error(1)
}

// RevokeToken mocks token revocation
func (m *MockAuthService) RevokeToken(token string) error {
	args := m.Called(token)
	return args.Error(0)
}

// Mock Storage Interface

// MockStorage provides a mock implementation of the storage interface
type MockStorage struct {
	mock.Mock
}

// Actor methods
func (m *MockStorage) CreateActor(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockStorage) GetActor(ctx context.Context, id string) (*models.Actor, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Actor), args.Error(1)
}

func (m *MockStorage) UpdateActor(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockStorage) DeleteActor(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Status methods
func (m *MockStorage) CreateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockStorage) GetStatus(ctx context.Context, id string) (*models.Status, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *MockStorage) UpdateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockStorage) DeleteStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Activity methods - simplified since Activity may not exist as a model
func (m *MockStorage) CreateActivity(ctx context.Context, actorID, activityType string) error {
	args := m.Called(ctx, actorID, activityType)
	return args.Error(0)
}

func (m *MockStorage) GetActivity(ctx context.Context, id string) (map[string]interface{}, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// Timeline methods
func (m *MockStorage) GetTimeline(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

func (m *MockStorage) AddToTimeline(ctx context.Context, actorID string, item *models.Timeline) error {
	args := m.Called(ctx, actorID, item)
	return args.Error(0)
}

// Relationship methods
func (m *MockStorage) Follow(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *MockStorage) Unfollow(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *MockStorage) GetFollowers(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Actor, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Actor), args.String(1), args.Error(2)
}

func (m *MockStorage) GetFollowing(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Actor, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Actor), args.String(1), args.Error(2)
}

// Mock Repository Pattern

// MockActorRepository provides a mock actor repository
type MockActorRepository struct {
	mock.Mock
}

func (m *MockActorRepository) Create(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockActorRepository) GetByID(ctx context.Context, id string) (*models.Actor, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Actor), args.Error(1)
}

func (m *MockActorRepository) GetByUsername(ctx context.Context, username string) (*models.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Actor), args.Error(1)
}

func (m *MockActorRepository) Update(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockActorRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockActorRepository) List(ctx context.Context, limit int, cursor string) ([]*models.Actor, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Actor), args.String(1), args.Error(2)
}

// MockStatusRepository provides a mock status repository
type MockStatusRepository struct {
	mock.Mock
}

func (m *MockStatusRepository) Create(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockStatusRepository) GetByID(ctx context.Context, id string) (*models.Status, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *MockStatusRepository) Update(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockStatusRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStatusRepository) GetByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Status, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Status), args.String(1), args.Error(2)
}

// Mock HTTP Client for external services

// MockHTTPClient provides a mock HTTP client
type MockHTTPClient struct {
	mock.Mock
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// Get mocks HTTP GET requests
func (m *MockHTTPClient) Get(url string, headers map[string]string) (*Response, error) {
	args := m.Called(url, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Post mocks HTTP POST requests
func (m *MockHTTPClient) Post(url string, body []byte, headers map[string]string) (*Response, error) {
	args := m.Called(url, body, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Put mocks HTTP PUT requests
func (m *MockHTTPClient) Put(url string, body []byte, headers map[string]string) (*Response, error) {
	args := m.Called(url, body, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Delete mocks HTTP DELETE requests
func (m *MockHTTPClient) Delete(url string, headers map[string]string) (*Response, error) {
	args := m.Called(url, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Mock Setup Helpers

// SetupMockAuthService creates a configured mock auth service
func SetupMockAuthService() *MockAuthService {
	return &MockAuthService{}
}

// SetupMockStorage creates a configured mock storage
func SetupMockStorage() *MockStorage {
	return &MockStorage{}
}

// SetupMockRepositories creates mock repositories
func SetupMockRepositories() (*MockActorRepository, *MockStatusRepository) {
	return &MockActorRepository{}, &MockStatusRepository{}
}

// Common Mock Expectations

// ExpectValidToken sets up expectations for a valid token
func ExpectValidToken(mockAuth *MockAuthService, token string, claims *auth.EnhancedClaims) {
	mockAuth.On("ValidateAccessToken", token).Return(claims, nil)
}

// ExpectInvalidToken sets up expectations for an invalid token
func ExpectInvalidToken(mockAuth *MockAuthService, token string) {
	mockAuth.On("ValidateAccessToken", token).Return(nil, auth.ErrInvalidToken)
}

// ExpectActorExists sets up expectations for an existing actor
func ExpectActorExists(mockStorage *MockStorage, actorID string, actor *models.Actor) {
	mockStorage.On("GetActor", mock.Anything, actorID).Return(actor, nil)
}

// ExpectActorNotFound sets up expectations for a non-existent actor
func ExpectActorNotFound(mockStorage *MockStorage, actorID string) {
	mockStorage.On("GetActor", mock.Anything, actorID).Return(nil, errors.New("not found"))
}

// ExpectStatusExists sets up expectations for an existing status
func ExpectStatusExists(mockStorage *MockStorage, statusID string, status *models.Status) {
	mockStorage.On("GetStatus", mock.Anything, statusID).Return(status, nil)
}

// ExpectStatusNotFound sets up expectations for a non-existent status
func ExpectStatusNotFound(mockStorage *MockStorage, statusID string) {
	mockStorage.On("GetStatus", mock.Anything, statusID).Return(nil, errors.New("not found"))
}

// Mock Error Types

var (
	// ErrMockDatabase represents a mock database error
	ErrMockDatabase = errors.New("mock database error")

	// ErrMockAuth represents a mock authentication error
	ErrMockAuth = errors.New("mock authentication error")

	// ErrMockNetwork represents a mock network error
	ErrMockNetwork = errors.New("mock network error")

	// ErrMockTimeout represents a mock timeout error
	ErrMockTimeout = errors.New("mock timeout error")
)

// Mock Behavioral Patterns

// MockWithDelay simulates network delays
type MockWithDelay struct {
	mock.Mock
	DelayMs int
}

// SimulateDelay adds artificial delay to mock calls
func (m *MockWithDelay) SimulateDelay() {
	if m.DelayMs > 0 {
		// In real implementation, would use time.Sleep
		// For tests, we can track that delay was requested
		m.Called(m.DelayMs)
	}
}

// MockWithFailure simulates intermittent failures
type MockWithFailure struct {
	mock.Mock
	FailureRate float64 // 0.0 to 1.0
	CallCount   int
}

// ShouldFail determines if this call should fail
func (m *MockWithFailure) ShouldFail() bool {
	m.CallCount++
	// Simple deterministic failure pattern for testing
	return float64(m.CallCount%10) < (m.FailureRate * 10)
}

// Test Data Builders for Mocks

// BuildTestActor creates a test actor for mocking
func BuildTestActor(username string) *models.Actor {
	return &models.Actor{
		PK:        fmt.Sprintf("actor#%s", username),
		SK:        fmt.Sprintf("actor#%s", username),
		Username:  username,
		NumericID: fmt.Sprintf("%d", 123456),
		GSI1PK:    fmt.Sprintf("USERNAME_SEARCH#%s", username[:2]),
		GSI1SK:    strings.ToLower(username),
		GSI3PK:    "DOMAIN#test.example.com",
		GSI3SK:    username,
	}
}

// BuildTestStatus creates a test status for mocking
func BuildTestStatus(actorID string, content string) *models.Status {
	statusID := fmt.Sprintf("status-%s-12345", actorID)
	return &models.Status{
		PK:             fmt.Sprintf("status#%s", statusID),
		SK:             fmt.Sprintf("status#%s", statusID),
		StatusID:       statusID,
		AuthorID:       actorID,
		AuthorUsername: actorID,
		Content:        content,
		Visibility:     "public",
		GSI1PK:         fmt.Sprintf("AUTHOR#%s", actorID),
		GSI1SK:         fmt.Sprintf("2024-01-01T00:00:00Z#%s", statusID),
	}
}

// BuildTestActivity creates a test activity for mocking
func BuildTestActivity(actorID string, activityType string) *models.Activity {
	return &models.Activity{
		PK:         fmt.Sprintf("activity#%s-%s", actorID, activityType),
		SK:         fmt.Sprintf("inbox#%s#123456", actorID),
		ActivityID: fmt.Sprintf("activity-%s-%s", actorID, activityType),
		Username:   actorID,
		Direction:  "inbox",
	}
}
