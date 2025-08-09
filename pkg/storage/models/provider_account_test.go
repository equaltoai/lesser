package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProviderAccount_TableName(t *testing.T) {
	pa := &ProviderAccount{}
	assert.Equal(t, "lesser-main", pa.TableName())
}

func TestProviderAccount_BeforeCreate(t *testing.T) {
	tests := []struct {
		name    string
		pa      *ProviderAccount
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid provider account",
			pa: &ProviderAccount{
				UserID:     "user123",
				Provider:   "google",
				ProviderID: "google123",
			},
			wantErr: false,
		},
		{
			name: "missing UserID",
			pa: &ProviderAccount{
				Provider:   "google",
				ProviderID: "google123",
			},
			wantErr: true,
			errMsg:  "UserID is required",
		},
		{
			name: "missing Provider",
			pa: &ProviderAccount{
				UserID:     "user123",
				ProviderID: "google123",
			},
			wantErr: true,
			errMsg:  "Provider is required",
		},
		{
			name: "missing ProviderID",
			pa: &ProviderAccount{
				UserID:   "user123",
				Provider: "google",
			},
			wantErr: true,
			errMsg:  "ProviderID is required",
		},
		{
			name: "invalid provider",
			pa: &ProviderAccount{
				UserID:     "user123",
				Provider:   "invalid",
				ProviderID: "invalid123",
			},
			wantErr: true,
			errMsg:  "invalid provider: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pa.BeforeCreate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)

				// Check that timestamps were set
				assert.False(t, tt.pa.CreatedAt.IsZero())
				assert.False(t, tt.pa.UpdatedAt.IsZero())
				assert.True(t, tt.pa.IsActive)

				// Check that keys were set correctly
				assert.Equal(t, "user#user123", tt.pa.PK)
				assert.Equal(t, "provider#google#google123", tt.pa.SK)
				assert.Equal(t, "PROVIDER#google", tt.pa.GSI1PK)
				assert.Equal(t, "google123#user123", tt.pa.GSI1SK)
				assert.Equal(t, "USER_PROVIDERS#user123", tt.pa.GSI2PK)
				assert.Contains(t, tt.pa.GSI2SK, "google#")
			}
		})
	}
}

func TestProviderAccount_BeforeUpdate(t *testing.T) {
	pa := &ProviderAccount{
		UserID:     "user123",
		Provider:   "google",
		ProviderID: "google123",
		CreatedAt:  time.Now().Add(-1 * time.Hour),
		UpdatedAt:  time.Now().Add(-1 * time.Hour),
	}

	// Set up initial keys
	err := pa.BeforeCreate()
	assert.NoError(t, err)

	originalUpdatedAt := pa.UpdatedAt

	// Wait a bit and call BeforeUpdate
	time.Sleep(10 * time.Millisecond)
	err = pa.BeforeUpdate()
	assert.NoError(t, err)

	// Check that UpdatedAt was updated
	assert.True(t, pa.UpdatedAt.After(originalUpdatedAt))
}

func TestProviderAccount_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pa      *ProviderAccount
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid provider account",
			pa: &ProviderAccount{
				UserID:     "user123",
				Provider:   "google",
				ProviderID: "google123",
			},
			wantErr: false,
		},
		{
			name: "empty UserID",
			pa: &ProviderAccount{
				UserID:     "",
				Provider:   "google",
				ProviderID: "google123",
			},
			wantErr: true,
			errMsg:  "UserID cannot be empty",
		},
		{
			name: "whitespace UserID",
			pa: &ProviderAccount{
				UserID:     "   ",
				Provider:   "google",
				ProviderID: "google123",
			},
			wantErr: true,
			errMsg:  "UserID cannot be empty",
		},
		{
			name: "expired token",
			pa: &ProviderAccount{
				UserID:      "user123",
				Provider:    "google",
				ProviderID:  "google123",
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(-1 * time.Hour),
			},
			wantErr: true,
			errMsg:  "access token has expired",
		},
		{
			name: "valid token",
			pa: &ProviderAccount{
				UserID:      "user123",
				Provider:    "google",
				ProviderID:  "google123",
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(1 * time.Hour),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pa.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProviderAccount_IsTokenExpired(t *testing.T) {
	tests := []struct {
		name     string
		pa       *ProviderAccount
		expected bool
	}{
		{
			name: "no token",
			pa: &ProviderAccount{
				AccessToken: "",
			},
			expected: false,
		},
		{
			name: "no expiry",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Time{},
			},
			expected: false,
		},
		{
			name: "expired token",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(-1 * time.Hour),
			},
			expected: true,
		},
		{
			name: "valid token",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(1 * time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pa.IsTokenExpired()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProviderAccount_NeedsRefresh(t *testing.T) {
	tests := []struct {
		name     string
		pa       *ProviderAccount
		expected bool
	}{
		{
			name: "no token",
			pa: &ProviderAccount{
				AccessToken: "",
			},
			expected: false,
		},
		{
			name: "expires in 10 minutes",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(10 * time.Minute),
			},
			expected: false,
		},
		{
			name: "expires in 3 minutes",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(3 * time.Minute),
			},
			expected: true,
		},
		{
			name: "already expired",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(-1 * time.Minute),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pa.NeedsRefresh()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProviderAccount_MarkUsed(t *testing.T) {
	pa := &ProviderAccount{}
	assert.Nil(t, pa.LastUsedAt)

	pa.MarkUsed()
	assert.NotNil(t, pa.LastUsedAt)
	assert.True(t, time.Since(*pa.LastUsedAt) < time.Second)
}

func TestProviderAccount_SetPrimary(t *testing.T) {
	pa := &ProviderAccount{}
	assert.False(t, pa.IsPrimary)

	pa.SetPrimary()
	assert.True(t, pa.IsPrimary)
}

func TestProviderAccount_ClearPrimary(t *testing.T) {
	pa := &ProviderAccount{IsPrimary: true}
	assert.True(t, pa.IsPrimary)

	pa.ClearPrimary()
	assert.False(t, pa.IsPrimary)
}

func TestProviderAccount_GetDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		pa       *ProviderAccount
		expected string
	}{
		{
			name: "has display name",
			pa: &ProviderAccount{
				DisplayName:  "John Doe",
				ProviderName: "Johnny",
				Username:     "john",
				Email:        "john@example.com",
			},
			expected: "John Doe",
		},
		{
			name: "has provider name",
			pa: &ProviderAccount{
				ProviderName: "Johnny",
				Username:     "john",
				Email:        "john@example.com",
			},
			expected: "Johnny",
		},
		{
			name: "has username",
			pa: &ProviderAccount{
				Username: "john",
				Email:    "john@example.com",
			},
			expected: "john",
		},
		{
			name: "has email only",
			pa: &ProviderAccount{
				Email: "john@example.com",
			},
			expected: "john@example.com",
		},
		{
			name:     "no display data",
			pa:       &ProviderAccount{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pa.GetDisplayName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProviderAccount_HasValidToken(t *testing.T) {
	tests := []struct {
		name     string
		pa       *ProviderAccount
		expected bool
	}{
		{
			name: "no token",
			pa: &ProviderAccount{
				AccessToken: "",
			},
			expected: false,
		},
		{
			name: "expired token",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(-1 * time.Hour),
			},
			expected: false,
		},
		{
			name: "valid token",
			pa: &ProviderAccount{
				AccessToken: "token123",
				TokenExpiry: time.Now().Add(1 * time.Hour),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pa.HasValidToken()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProviderAccount_setupGSIKeys(t *testing.T) {
	pa := &ProviderAccount{
		UserID:     "user123",
		Provider:   "google",
		ProviderID: "google123",
		CreatedAt:  time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	pa.setupGSIKeys()

	assert.Equal(t, "PROVIDER#google", pa.GSI1PK)
	assert.Equal(t, "google123#user123", pa.GSI1SK)
	assert.Equal(t, "USER_PROVIDERS#user123", pa.GSI2PK)
	assert.Equal(t, "google#2023-01-01T12:00:00Z", pa.GSI2SK)
}

func TestIsValidProvider(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"google", true},
		{"github", true},
		{"twitter", true},
		{"facebook", true},
		{"discord", true},
		{"apple", true},
		{"mastodon", true},
		{"GOOGLE", true}, // case insensitive
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			result := isValidProvider(tt.provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}
