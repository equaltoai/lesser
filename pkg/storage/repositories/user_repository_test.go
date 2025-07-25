package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

func TestCreateUser_MissingUsername(t *testing.T) {
	repo := &UserRepository{}

	user := &storage.User{
		Email: "test@example.com",
	}

	err := repo.CreateUser(context.Background(), user)

	assert.Error(t, err)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestGetUserByEmail_EmptyEmail(t *testing.T) {
	repo := &UserRepository{}

	user, err := repo.GetUserByEmail(context.Background(), "")

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestUpdateUser_EmptyUpdates(t *testing.T) {
	repo := &UserRepository{}

	err := repo.UpdateUser(context.Background(), "testuser", map[string]any{})

	assert.Error(t, err)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestGetUserByProviderID_NotImplemented(t *testing.T) {
	repo := &UserRepository{}

	user, err := repo.GetUserByProviderID(context.Background(), "google", "123")

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestLinkProviderAccount_NotImplemented(t *testing.T) {
	repo := &UserRepository{}

	err := repo.LinkProviderAccount(context.Background(), "testuser", "google", "123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestUnlinkProviderAccount_NotImplemented(t *testing.T) {
	repo := &UserRepository{}

	err := repo.UnlinkProviderAccount(context.Background(), "testuser", "google")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestGetLinkedProviders_ReturnsEmpty(t *testing.T) {
	repo := &UserRepository{}

	providers, err := repo.GetLinkedProviders(context.Background(), "testuser")

	assert.NoError(t, err)
	assert.Empty(t, providers)
}

func TestModelToStorage(t *testing.T) {
	repo := &UserRepository{}
	now := time.Now()

	userModel := &models.User{
		Username:        "testuser",
		Email:           "test@example.com",
		PasswordHash:    "hashedpassword",
		DisplayName:     "Test User",
		CreatedAt:       now,
		UpdatedAt:       now,
		Approved:        true,
		Suspended:       false,
		Silenced:        false,
		Role:            "user",
		Locale:          "en",
		RecoveryMethods: []string{"email", "passkey"},
	}

	storageUser := repo.modelToStorage(userModel)

	assert.Equal(t, userModel.Username, storageUser.Username)
	assert.Equal(t, userModel.Email, storageUser.Email)
	assert.Equal(t, userModel.PasswordHash, storageUser.PasswordHash)
	assert.Equal(t, userModel.DisplayName, storageUser.DisplayName)
	assert.Equal(t, userModel.CreatedAt, storageUser.CreatedAt)
	assert.Equal(t, userModel.UpdatedAt, storageUser.UpdatedAt)
	assert.Equal(t, userModel.Approved, storageUser.Approved)
	assert.Equal(t, userModel.Suspended, storageUser.Suspended)
	assert.Equal(t, userModel.Silenced, storageUser.Silenced)
	assert.Equal(t, userModel.Role, storageUser.Role)
	assert.Equal(t, userModel.Locale, storageUser.Locale)
	assert.Equal(t, userModel.RecoveryMethods, storageUser.RecoveryMethods)
}

func TestApplyUpdates(t *testing.T) {
	repo := &UserRepository{}
	userModel := &models.User{
		Username: "testuser",
		Email:    "old@example.com",
		Role:     "user",
		Approved: false,
	}

	updates := map[string]any{
		"email":            "new@example.com",
		"approved":         true,
		"role":             "moderator",
		"display_name":     "New Display Name",
		"suspended":        true,
		"silenced":         false,
		"locale":           "es",
		"password_hash":    "newhash",
		"recovery_methods": []string{"passkey", "wallet"},
		"invalid_field":    "should be ignored",
	}

	repo.applyUpdates(userModel, updates)

	assert.Equal(t, "new@example.com", userModel.Email)
	assert.True(t, userModel.Approved)
	assert.Equal(t, "moderator", userModel.Role)
	assert.Equal(t, "New Display Name", userModel.DisplayName)
	assert.True(t, userModel.Suspended)
	assert.False(t, userModel.Silenced)
	assert.Equal(t, "es", userModel.Locale)
	assert.Equal(t, "newhash", userModel.PasswordHash)
	assert.Equal(t, []string{"passkey", "wallet"}, userModel.RecoveryMethods)
}

func TestNewUserRepository(t *testing.T) {
	repo := NewUserRepository(nil)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
}
