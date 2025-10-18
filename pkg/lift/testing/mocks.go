package testing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

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

// MockStorage provides a mock implementation of the storage interface
type MockStorage struct {
	mock.Mock
}

// CreateActor mocks the CreateActor method
func (m *MockStorage) CreateActor(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

// GetActor mocks the GetActor method
func (m *MockStorage) GetActor(ctx context.Context, id string) (*models.Actor, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Actor), args.Error(1)
}

// UpdateActor mocks the UpdateActor method
func (m *MockStorage) UpdateActor(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

// DeleteActor mocks the DeleteActor method
func (m *MockStorage) DeleteActor(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// CreateStatus mocks the CreateStatus method
func (m *MockStorage) CreateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// GetStatus mocks the GetStatus method
func (m *MockStorage) GetStatus(ctx context.Context, id string) (*models.Status, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

// UpdateStatus mocks the UpdateStatus method
func (m *MockStorage) UpdateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// DeleteStatus mocks the DeleteStatus method
func (m *MockStorage) DeleteStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// CreateActivity mocks the CreateActivity method
func (m *MockStorage) CreateActivity(ctx context.Context, actorID, activityType string) error {
	args := m.Called(ctx, actorID, activityType)
	return args.Error(0)
}

// GetActivity mocks the GetActivity method
func (m *MockStorage) GetActivity(ctx context.Context, id string) (map[string]interface{}, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// GetTimeline mocks the GetTimeline method
func (m *MockStorage) GetTimeline(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// AddToTimeline mocks the AddToTimeline method
func (m *MockStorage) AddToTimeline(ctx context.Context, actorID string, item *models.Timeline) error {
	args := m.Called(ctx, actorID, item)
	return args.Error(0)
}

// Follow mocks the Follow method
func (m *MockStorage) Follow(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

// Unfollow mocks the Unfollow method
func (m *MockStorage) Unfollow(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

// GetFollowers mocks the GetFollowers method
func (m *MockStorage) GetFollowers(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Actor, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Actor), args.String(1), args.Error(2)
}

// GetFollowing mocks the GetFollowing method
func (m *MockStorage) GetFollowing(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Actor, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Actor), args.String(1), args.Error(2)
}

// MockActorRepository provides a mock actor repository
type MockActorRepository struct {
	mock.Mock
}

// Create mocks the MockActorRepository.Create method
func (m *MockActorRepository) Create(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

// GetByID mocks the MockActorRepository.GetByID method
func (m *MockActorRepository) GetByID(ctx context.Context, id string) (*models.Actor, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Actor), args.Error(1)
}

// GetByUsername mocks the MockActorRepository.GetByUsername method
func (m *MockActorRepository) GetByUsername(ctx context.Context, username string) (*models.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Actor), args.Error(1)
}

// Update mocks the MockActorRepository.Update method
func (m *MockActorRepository) Update(ctx context.Context, actor *models.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

// Delete mocks the MockActorRepository.Delete method
func (m *MockActorRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// List mocks the MockActorRepository.List method
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

// Create mocks the MockStatusRepository.Create method
func (m *MockStatusRepository) Create(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// GetByID mocks the MockStatusRepository.GetByID method
func (m *MockStatusRepository) GetByID(ctx context.Context, id string) (*models.Status, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

// Update mocks the MockStatusRepository.Update method
func (m *MockStatusRepository) Update(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// Delete mocks the MockStatusRepository.Delete method
func (m *MockStatusRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetByActor mocks the MockStatusRepository.GetByActor method
func (m *MockStatusRepository) GetByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Status, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Status), args.String(1), args.Error(2)
}

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

// Get mocks the MockHTTPClient.Get method
func (m *MockHTTPClient) Get(url string, headers map[string]string) (*Response, error) {
	args := m.Called(url, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Post mocks the MockHTTPClient.Post method
func (m *MockHTTPClient) Post(url string, body []byte, headers map[string]string) (*Response, error) {
	args := m.Called(url, body, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Put mocks the MockHTTPClient.Put method
func (m *MockHTTPClient) Put(url string, body []byte, headers map[string]string) (*Response, error) {
	args := m.Called(url, body, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

// Delete mocks the MockHTTPClient.Delete method
func (m *MockHTTPClient) Delete(url string, headers map[string]string) (*Response, error) {
	args := m.Called(url, headers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Response), args.Error(1)
}

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
		PK: fmt.Sprintf("ACTOR#%s", actorID),
		SK: fmt.Sprintf("ACTIVITY#%s#%s", time.Now().Format(time.RFC3339), fmt.Sprintf("activity-%s-%s", actorID, activityType)),
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://example.com/activities/%s-%s", actorID, activityType),
				Type: activityType,
			},
			Actor: fmt.Sprintf("https://example.com/users/%s", actorID),
		},
		CreatedAt: time.Now(),
	}
}
