package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// UserModelTestSuite contains tests for User model
type UserModelTestSuite struct {
	suite.Suite
}

// Test BeforeCreate

func (suite *UserModelTestSuite) TestBeforeCreate_SetsTimestamps() {
	user := &User{
		Username: "testuser",
		Email:    "test@example.com",
	}

	err := user.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.False(suite.T(), user.CreatedAt.IsZero())
	assert.False(suite.T(), user.UpdatedAt.IsZero())
	assert.WithinDuration(suite.T(), time.Now(), user.CreatedAt, time.Second)
	assert.WithinDuration(suite.T(), time.Now(), user.UpdatedAt, time.Second)
}

func (suite *UserModelTestSuite) TestBeforeCreate_SetsDefaultRole() {
	user := &User{
		Username: "testuser",
		Email:    "test@example.com",
	}

	err := user.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "user", user.Role)
}

func (suite *UserModelTestSuite) TestBeforeCreate_PreservesExistingRole() {
	user := &User{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "admin",
	}

	err := user.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "admin", user.Role)
}

func (suite *UserModelTestSuite) TestBeforeCreate_SetsPrimaryKeys() {
	user := &User{
		Username: "testuser",
		Email:    "test@example.com",
	}

	err := user.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "user#testuser", user.PK)
	assert.Equal(suite.T(), "user#testuser", user.SK)
}

func (suite *UserModelTestSuite) TestBeforeCreate_SetsGSIKeys() {
	user := &User{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "moderator",
		Approved: true,
	}

	err := user.BeforeCreate()

	assert.NoError(suite.T(), err)

	// GSI1 - Email index
	assert.Equal(suite.T(), "EMAIL#test@example.com", user.GSI1PK)
	assert.Equal(suite.T(), "user#testuser", user.GSI1SK)

	// GSI2 - User list index
	assert.Equal(suite.T(), "USERS", user.GSI2PK)
	assert.Contains(suite.T(), user.GSI2SK, "#testuser")

	// GSI3 - Role index
	assert.Equal(suite.T(), "ROLE#moderator", user.GSI3PK)
	assert.Equal(suite.T(), "testuser", user.GSI3SK)

	// GSI4 - Status index
	assert.Equal(suite.T(), "STATUS#active", user.GSI4PK)
	assert.Equal(suite.T(), "testuser", user.GSI4SK)
}

// Test BeforeUpdate

func (suite *UserModelTestSuite) TestBeforeUpdate_UpdatesTimestamp() {
	user := &User{
		Username:  "testuser",
		Email:     "test@example.com",
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}

	err := user.BeforeUpdate()

	assert.NoError(suite.T(), err)
	assert.WithinDuration(suite.T(), time.Now(), user.UpdatedAt, time.Second)
}

func (suite *UserModelTestSuite) TestBeforeUpdate_UpdatesGSIKeys() {
	user := &User{
		Username: "testuser",
		Email:    "newemail@example.com",
		Role:     "admin",
		Approved: false,
	}

	err := user.BeforeUpdate()

	assert.NoError(suite.T(), err)

	// GSI1 - Email index should be updated
	assert.Equal(suite.T(), "EMAIL#newemail@example.com", user.GSI1PK)
	assert.Equal(suite.T(), "user#testuser", user.GSI1SK)

	// GSI3 - Role index should be updated
	assert.Equal(suite.T(), "ROLE#admin", user.GSI3PK)
	assert.Equal(suite.T(), "testuser", user.GSI3SK)

	// GSI4 - Status index should be updated
	assert.Equal(suite.T(), "STATUS#pending", user.GSI4PK)
	assert.Equal(suite.T(), "testuser", user.GSI4SK)
}

// Test setupGSIKeys

func (suite *UserModelTestSuite) TestSetupGSIKeys_WithEmail() {
	user := &User{
		Username: "testuser",
		Email:    "Test@Example.Com", // Mixed case to test normalization
		Role:     "user",
		Approved: true,
	}

	user.setupGSIKeys()

	// GSI1 - Email should be normalized to lowercase
	assert.Equal(suite.T(), "EMAIL#test@example.com", user.GSI1PK)
	assert.Equal(suite.T(), "user#testuser", user.GSI1SK)
}

func (suite *UserModelTestSuite) TestSetupGSIKeys_WithoutEmail() {
	user := &User{
		Username: "testuser",
		Role:     "user",
		Approved: true,
	}

	user.setupGSIKeys()

	// GSI1 - Should be empty when no email
	assert.Empty(suite.T(), user.GSI1PK)
	assert.Empty(suite.T(), user.GSI1SK)
}

func (suite *UserModelTestSuite) TestSetupGSIKeys_UserListIndex() {
	user := &User{
		Username:  "testuser",
		CreatedAt: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	user.setupGSIKeys()

	// GSI2 - User list index
	assert.Equal(suite.T(), "USERS", user.GSI2PK)
	assert.Equal(suite.T(), "2023-01-01T12:00:00Z#testuser", user.GSI2SK)
}

func (suite *UserModelTestSuite) TestSetupGSIKeys_RoleIndex() {
	testCases := []struct {
		role     string
		expected string
	}{
		{"user", "ROLE#user"},
		{"moderator", "ROLE#moderator"},
		{"admin", "ROLE#admin"},
		{"", "ROLE#"},
	}

	for _, tc := range testCases {
		user := &User{
			Username: "testuser",
			Role:     tc.role,
		}

		user.setupGSIKeys()

		assert.Equal(suite.T(), tc.expected, user.GSI3PK)
		assert.Equal(suite.T(), "testuser", user.GSI3SK)
	}
}

func (suite *UserModelTestSuite) TestSetupGSIKeys_StatusIndex() {
	testCases := []struct {
		name      string
		approved  bool
		suspended bool
		silenced  bool
		expected  string
	}{
		{"active user", true, false, false, "STATUS#active"},
		{"pending user", false, false, false, "STATUS#pending"},
		{"suspended user", true, true, false, "STATUS#suspended"},
		{"silenced user", true, false, true, "STATUS#silenced"},
		{"suspended takes precedence", false, true, true, "STATUS#suspended"},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{
				Username:  "testuser",
				Approved:  tc.approved,
				Suspended: tc.suspended,
				Silenced:  tc.silenced,
			}

			user.setupGSIKeys()

			assert.Equal(t, tc.expected, user.GSI4PK)
			assert.Equal(t, "testuser", user.GSI4SK)
		})
	}
}

// Test getStatusString

func (suite *UserModelTestSuite) TestGetStatusString() {
	testCases := []struct {
		name      string
		approved  bool
		suspended bool
		silenced  bool
		expected  string
	}{
		{"active user", true, false, false, "active"},
		{"pending user", false, false, false, "pending"},
		{"suspended user", true, true, false, "suspended"},
		{"silenced user", true, false, true, "silenced"},
		{"suspended takes precedence over silenced", true, true, true, "suspended"},
		{"suspended pending user", false, true, false, "suspended"},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{
				Approved:  tc.approved,
				Suspended: tc.suspended,
				Silenced:  tc.silenced,
			}

			status := user.getStatusString()
			assert.Equal(t, tc.expected, status)
		})
	}
}

// Test helper methods

func (suite *UserModelTestSuite) TestIsActive() {
	testCases := []struct {
		name      string
		approved  bool
		suspended bool
		silenced  bool
		expected  bool
	}{
		{"active user", true, false, false, true},
		{"pending user", false, false, false, false},
		{"suspended user", true, true, false, false},
		{"silenced user", true, false, true, false},
		{"suspended and silenced", true, true, true, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{
				Approved:  tc.approved,
				Suspended: tc.suspended,
				Silenced:  tc.silenced,
			}

			assert.Equal(t, tc.expected, user.IsActive())
		})
	}
}

func (suite *UserModelTestSuite) TestHasEmail() {
	testCases := []struct {
		name     string
		email    string
		expected bool
	}{
		{"with email", "test@example.com", true},
		{"empty email", "", false},
		{"whitespace email", "   ", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{Email: tc.email}
			assert.Equal(t, tc.expected, user.HasEmail())
		})
	}
}

func (suite *UserModelTestSuite) TestHasPassword() {
	testCases := []struct {
		name         string
		passwordHash string
		expected     bool
	}{
		{"with password", "hashedpassword", true},
		{"empty password", "", false},
		{"whitespace password", "   ", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{PasswordHash: tc.passwordHash}
			assert.Equal(t, tc.expected, user.HasPassword())
		})
	}
}

func (suite *UserModelTestSuite) TestIsAdmin() {
	testCases := []struct {
		name     string
		role     string
		expected bool
	}{
		{"admin user", "admin", true},
		{"moderator user", "moderator", false},
		{"regular user", "user", false},
		{"empty role", "", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{Role: tc.role}
			assert.Equal(t, tc.expected, user.IsAdmin())
		})
	}
}

func (suite *UserModelTestSuite) TestIsModerator() {
	testCases := []struct {
		name     string
		role     string
		expected bool
	}{
		{"admin user", "admin", true},
		{"moderator user", "moderator", true},
		{"regular user", "user", false},
		{"empty role", "", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			user := &User{Role: tc.role}
			assert.Equal(t, tc.expected, user.IsModerator())
		})
	}
}

// Test TableName

func (suite *UserModelTestSuite) TestTableName() {
	user := &User{}
	assert.Equal(suite.T(), "lesser-main", user.TableName())
}

// Run the test suite
func TestUserModelTestSuite(t *testing.T) {
	suite.Run(t, new(UserModelTestSuite))
}
