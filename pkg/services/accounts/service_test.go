package accounts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
)

// Mock implementations

type MockAccountRepository struct {
	mock.Mock
}

func (m *MockAccountRepository) CreateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	args := m.Called(ctx, actorURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) UpdateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *MockAccountRepository) DeleteAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockAccountRepository) GetSuggestedAccounts(ctx context.Context, forUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	args := m.Called(ctx, forUserID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.AccountSuggestion]), args.Error(1)
}

func (m *MockAccountRepository) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockAccountRepository) ApproveAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) SuspendAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

func (m *MockAccountRepository) UnsuspendAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) SilenceAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

func (m *MockAccountRepository) UnsilenceAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockAccountRepository) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	args := m.Called(ctx, username, features)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]bool), args.Error(1)
}

func (m *MockAccountRepository) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	args := m.Called(ctx, username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) UpdatePassword(ctx context.Context, username, newPasswordHash string) error {
	args := m.Called(ctx, username, newPasswordHash)
	return args.Error(0)
}

func (m *MockAccountRepository) CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error {
	args := m.Called(ctx, reset)
	return args.Error(0)
}

func (m *MockAccountRepository) GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PasswordReset), args.Error(1)
}

func (m *MockAccountRepository) UsePasswordReset(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockAccountRepository) RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *MockAccountRepository) GetLoginHistory(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.LoginAttempt]), args.Error(1)
}

func (m *MockAccountRepository) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	args := m.Called(ctx, username, activity)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	args := m.Called(ctx, usernames)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) GetAccountsCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

type MockFederationService struct {
	mock.Mock
}

func (m *MockFederationService) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

type MockPublisher struct {
	mock.Mock
	events []streaming.Event
}

func (m *MockPublisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	args := m.Called(ctx, userID, event)
	if args.Error(0) == nil {
		m.events = append(m.events, *event)
	}
	return args.Error(0)
}

func (m *MockPublisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	args := m.Called(ctx, streamName, event)
	if args.Error(0) == nil {
		m.events = append(m.events, *event)
	}
	return args.Error(0)
}

func (m *MockPublisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	args := m.Called(ctx, conversationID, event)
	if args.Error(0) == nil {
		m.events = append(m.events, *event)
	}
	return args.Error(0)
}

func (m *MockPublisher) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPublisher) GetEvents() []streaming.Event {
	return m.events
}

func (m *MockPublisher) Reset() {
	m.events = nil
}

// Test Suite

type AccountsServiceTestSuite struct {
	suite.Suite
	service       *Service
	accountRepo   *MockAccountRepository
	publisher     *MockPublisher
	federation    *MockFederationService
	ctx           context.Context
}

func (suite *AccountsServiceTestSuite) SetupTest() {
	suite.accountRepo = new(MockAccountRepository)
	suite.publisher = new(MockPublisher)
	suite.federation = new(MockFederationService)
	suite.ctx = context.Background()

	logger := zaptest.NewLogger(suite.T())
	suite.service = NewService(
		suite.accountRepo,
		suite.publisher,
		suite.federation,
		logger,
		"example.com",
	)
}

func (suite *AccountsServiceTestSuite) TearDownTest() {
	suite.accountRepo.AssertExpectations(suite.T())
	suite.publisher.AssertExpectations(suite.T())
	suite.federation.AssertExpectations(suite.T())
}

// Helper methods

func (suite *AccountsServiceTestSuite) createTestAccount(username string) *storage.Account {
	now := time.Now()
	return &storage.Account{
		User: &storage.User{
			Username:    username,
			Email:       username + "@example.com",
			DisplayName: "Test " + username,
			CreatedAt:   now,
			UpdatedAt:   now,
			Approved:    true,
			Role:        "user",
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/" + username,
				Type: "Person",
			},
			PreferredUsername: username,
			Name:              "Test " + username,
			Summary:           "Test bio for " + username,
			URL:               "https://example.com/@" + username,
		},
	}
}

// UpdateProfile Tests

func (suite *AccountsServiceTestSuite) TestUpdateProfile_Success() {
	account := suite.createTestAccount("testuser")
	
	cmd := &UpdateProfileCommand{
		Username:    "testuser",
		DisplayName: "Updated Name",
		Bio:         "Updated bio",
		Avatar:      "https://example.com/avatar.jpg",
		Header:      "https://example.com/header.jpg",
		Locked:      true,
		Bot:         false,
		Fields: []ProfileField{
			{Name: "Website", Value: "https://example.com"},
			{Name: "Location", Value: "Test City"},
		},
		Discoverable: true,
		UpdaterID:    "testuser",
	}

	// Setup expectations
	suite.accountRepo.On("GetAccount", suite.ctx, "testuser").Return(account, nil)
	suite.accountRepo.On("UpdateAccount", suite.ctx, mock.MatchedBy(func(acc *storage.Account) bool {
		return acc.User.DisplayName == "Updated Name" &&
			acc.Actor.Name == "Updated Name" &&
			acc.Actor.Summary == "Updated bio" &&
			acc.Actor.ManuallyApprovesFollowers == true &&
			acc.Actor.Type == "Person" // Bot = false should result in Type = "Person"
	})).Return(nil)

	suite.publisher.On("PublishToUser", suite.ctx, "testuser", mock.AnythingOfType("*streaming.Event")).Return(nil)
	suite.publisher.On("PublishToStream", suite.ctx, "followers:testuser", mock.AnythingOfType("*streaming.Event")).Return(nil)

	suite.federation.On("QueueActivity", suite.ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Execute
	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	// Assertions
	suite.NoError(err)
	suite.NotNil(result)
	suite.NotNil(result.Account)
	suite.Equal("Updated Name", result.Account.User.DisplayName)
	suite.Equal("Updated Name", result.Account.Actor.Name)
	suite.Equal("Updated bio", result.Account.Actor.Summary)
	suite.True(result.Account.Actor.ManuallyApprovesFollowers)
	suite.Len(result.Events, 2) // user event + followers event
}

func (suite *AccountsServiceTestSuite) TestUpdateProfile_ValidationFailure() {
	cmd := &UpdateProfileCommand{
		Username:    "", // Missing username
		DisplayName: "Test",
		UpdaterID:   "testuser",
	}

	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "validation failed")
}

func (suite *AccountsServiceTestSuite) TestUpdateProfile_UnauthorizedUser() {
	account := suite.createTestAccount("testuser")
	
	cmd := &UpdateProfileCommand{
		Username:    "testuser",
		DisplayName: "Updated Name",
		UpdaterID:   "otheruser", // Different user trying to update
	}

	suite.accountRepo.On("GetAccount", suite.ctx, "testuser").Return(account, nil)

	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "unauthorized")
}

func (suite *AccountsServiceTestSuite) TestUpdateProfile_AccountNotFound() {
	cmd := &UpdateProfileCommand{
		Username:    "nonexistent",
		DisplayName: "Test",
		UpdaterID:   "nonexistent",
	}

	suite.accountRepo.On("GetAccount", suite.ctx, "nonexistent").Return(nil, errors.New("not found"))

	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "failed to get account")
}

func (suite *AccountsServiceTestSuite) TestUpdateProfile_DisplayNameTooLong() {
	longName := strings.Repeat("a", 101) // 101 characters
	cmd := &UpdateProfileCommand{
		Username:    "testuser",
		DisplayName: longName,
		UpdaterID:   "testuser",
	}

	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "display name too long")
}

func (suite *AccountsServiceTestSuite) TestUpdateProfile_BioTooLong() {
	longBio := strings.Repeat("a", 5001) // 5001 characters
	cmd := &UpdateProfileCommand{
		Username:  "testuser",
		Bio:       longBio,
		UpdaterID: "testuser",
	}

	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "bio too long")
}

func (suite *AccountsServiceTestSuite) TestUpdateProfile_TooManyFields() {
	cmd := &UpdateProfileCommand{
		Username:  "testuser",
		UpdaterID: "testuser",
		Fields: []ProfileField{
			{Name: "Field1", Value: "Value1"},
			{Name: "Field2", Value: "Value2"},
			{Name: "Field3", Value: "Value3"},
			{Name: "Field4", Value: "Value4"},
			{Name: "Field5", Value: "Value5"}, // 5th field - should fail
		},
	}

	result, err := suite.service.UpdateProfile(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "too many profile fields")
}

// UpdatePreferences Tests

func (suite *AccountsServiceTestSuite) TestUpdatePreferences_Success() {
	cmd := &UpdatePreferencesCommand{
		Username:                  "testuser",
		Language:                  "en",
		DefaultPostingVisibility:  "public",
		DefaultMediaSensitive:     false,
		ExpandSpoilers:            true,
		ExpandMedia:               "default",
		AutoplayGifs:              true,
		ShowFollowCounts:          true,
		PreferredTimelineOrder:    "newest",
		SearchSuggestionsEnabled:  true,
		PersonalizedSearchEnabled: true,
		ReblogFilters:             map[string]bool{"reblogs": true},
		UpdaterID:                 "testuser",
	}

	expectedPrefs := map[string]interface{}{
		"language":                    "en",
		"default_posting_visibility":  "public",
		"default_media_sensitive":     false,
		"expand_spoilers":             true,
		"expand_media":                "default",
		"autoplay_gifs":               true,
		"show_follow_counts":          true,
		"preferred_timeline_order":    "newest",
		"search_suggestions_enabled":  true,
		"personalized_search_enabled": true,
		"reblog_filters":              map[string]bool{"reblogs": true},
	}

	suite.accountRepo.On("UpdateAccountPreferences", suite.ctx, "testuser", expectedPrefs).Return(nil)
	suite.publisher.On("PublishToUser", suite.ctx, "testuser", mock.AnythingOfType("*streaming.Event")).Return(nil)

	result, err := suite.service.UpdatePreferences(suite.ctx, cmd)

	suite.NoError(err)
	suite.NotNil(result)
	suite.Equal(expectedPrefs, result.Preferences)
	suite.Len(result.Events, 1) // Only user event, no federation
}

func (suite *AccountsServiceTestSuite) TestUpdatePreferences_UnauthorizedUser() {
	cmd := &UpdatePreferencesCommand{
		Username:  "testuser",
		UpdaterID: "otheruser", // Different user trying to update
	}

	result, err := suite.service.UpdatePreferences(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "unauthorized")
}

func (suite *AccountsServiceTestSuite) TestUpdatePreferences_InvalidVisibility() {
	cmd := &UpdatePreferencesCommand{
		Username:                 "testuser",
		DefaultPostingVisibility: "invalid", // Invalid visibility
		UpdaterID:                "testuser",
	}

	result, err := suite.service.UpdatePreferences(suite.ctx, cmd)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "invalid default posting visibility")
}

// GetAccount Tests

func (suite *AccountsServiceTestSuite) TestGetAccount_Success() {
	account := suite.createTestAccount("testuser")

	suite.accountRepo.On("GetAccount", suite.ctx, "testuser").Return(account, nil)

	query := &GetAccountQuery{
		Username: "testuser",
		ViewerID: "viewer",
	}

	result, err := suite.service.GetAccount(suite.ctx, query)

	suite.NoError(err)
	suite.NotNil(result)
	suite.Equal("testuser", result.User.Username)
	suite.Empty(result.User.Email) // Email should be hidden from other viewers
}

func (suite *AccountsServiceTestSuite) TestGetAccount_ViewOwnAccount() {
	account := suite.createTestAccount("testuser")

	suite.accountRepo.On("GetAccount", suite.ctx, "testuser").Return(account, nil)

	query := &GetAccountQuery{
		Username: "testuser",
		ViewerID: "testuser", // Viewing own account
	}

	result, err := suite.service.GetAccount(suite.ctx, query)

	suite.NoError(err)
	suite.NotNil(result)
	suite.Equal("testuser", result.User.Username)
	suite.Equal("testuser@example.com", result.User.Email) // Email visible to self
}

func (suite *AccountsServiceTestSuite) TestGetAccount_SuspendedAccount() {
	account := suite.createTestAccount("testuser")
	account.User.Suspended = true

	suite.accountRepo.On("GetAccount", suite.ctx, "testuser").Return(account, nil)

	query := &GetAccountQuery{
		Username: "testuser",
		ViewerID: "viewer", // Different viewer
	}

	result, err := suite.service.GetAccount(suite.ctx, query)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "account not found") // Don't reveal suspension
}

func (suite *AccountsServiceTestSuite) TestGetAccount_NotFound() {
	suite.accountRepo.On("GetAccount", suite.ctx, "nonexistent").Return(nil, errors.New("not found"))

	query := &GetAccountQuery{
		Username: "nonexistent",
		ViewerID: "viewer",
	}

	result, err := suite.service.GetAccount(suite.ctx, query)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "failed to get account")
}

// SearchAccounts Tests

func (suite *AccountsServiceTestSuite) TestSearchAccounts_Success() {
	accounts := []*storage.Account{
		suite.createTestAccount("user1"),
		suite.createTestAccount("user2"),
	}

	searchResult := &interfaces.PaginatedResult[*storage.Account]{
		Items:      accounts,
		NextCursor: "cursor123",
		HasMore:    false,
		Total:      2,
	}

	pagination := interfaces.PaginationOptions{Limit: 20}
	suite.accountRepo.On("SearchAccounts", suite.ctx, "test", pagination).Return(searchResult, nil)

	query := &SearchAccountsQuery{
		Query:      "test",
		ViewerID:   "viewer",
		Pagination: pagination,
	}

	result, err := suite.service.SearchAccounts(suite.ctx, query)

	suite.NoError(err)
	suite.NotNil(result)
	suite.Len(result.Accounts, 2)
	suite.Equal("cursor123", result.Pagination.NextCursor)
	suite.False(result.Pagination.HasMore)
}

func (suite *AccountsServiceTestSuite) TestSearchAccounts_EmptyQuery() {
	query := &SearchAccountsQuery{
		Query:    "", // Empty query
		ViewerID: "viewer",
	}

	result, err := suite.service.SearchAccounts(suite.ctx, query)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "search query cannot be empty")
}

func (suite *AccountsServiceTestSuite) TestSearchAccounts_FiltersSuspendedAccounts() {
	user1 := suite.createTestAccount("user1")
	user2 := suite.createTestAccount("user2")
	user2.User.Suspended = true // Suspended user

	accounts := []*storage.Account{user1, user2}

	searchResult := &interfaces.PaginatedResult[*storage.Account]{
		Items:      accounts,
		NextCursor: "",
		HasMore:    false,
		Total:      2,
	}

	pagination := interfaces.PaginationOptions{Limit: 20}
	suite.accountRepo.On("SearchAccounts", suite.ctx, "test", pagination).Return(searchResult, nil)

	query := &SearchAccountsQuery{
		Query:      "test",
		ViewerID:   "viewer", // Not the suspended user
		Pagination: pagination,
	}

	result, err := suite.service.SearchAccounts(suite.ctx, query)

	suite.NoError(err)
	suite.NotNil(result)
	suite.Len(result.Accounts, 1) // Only user1, user2 filtered out
	suite.Equal("user1", result.Accounts[0].User.Username)
}

// Run the test suite

func TestAccountsServiceSuite(t *testing.T) {
	suite.Run(t, new(AccountsServiceTestSuite))
}

// Individual test functions for better test discovery

func TestNewService(t *testing.T) {
	accountRepo := new(MockAccountRepository)
	publisher := new(MockPublisher)
	federation := new(MockFederationService)
	logger := zaptest.NewLogger(t)

	service := NewService(accountRepo, publisher, federation, logger, "example.com")

	assert.NotNil(t, service)
	assert.Equal(t, accountRepo, service.accountRepo)
	assert.Equal(t, publisher, service.publisher)
	assert.Equal(t, federation, service.federation)
	assert.NotNil(t, service.logger)
	assert.Equal(t, "example.com", service.domainName)
}

func TestNewService_NilLogger(t *testing.T) {
	accountRepo := new(MockAccountRepository)
	publisher := new(MockPublisher)
	federation := new(MockFederationService)

	service := NewService(accountRepo, publisher, federation, nil, "example.com")

	assert.NotNil(t, service)
	assert.NotNil(t, service.logger) // Should create a no-op logger
}