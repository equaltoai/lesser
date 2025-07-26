package models

import (
	"fmt"
	"strings"
	"time"
)

// ProviderAccount represents an OAuth provider account linked to a user
type ProviderAccount struct {
	// Primary key - using composite key for user + provider relationship
	PK string `dynamorm:"pk" json:"pk"` // Format: "user#{userID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "provider#{provider}#{providerID}"

	// GSI1 - Provider lookup (find user by provider account)
	GSI1PK string `dynamorm:"index:provider-index,pk" json:"gsi1_pk"` // Format: "PROVIDER#{provider}"
	GSI1SK string `dynamorm:"index:provider-index,sk" json:"gsi1_sk"` // Format: "{providerID}#{userID}"

	// GSI2 - User's provider accounts
	GSI2PK string `dynamorm:"index:user-providers-index,pk" json:"gsi2_pk"` // Format: "USER_PROVIDERS#{userID}"
	GSI2SK string `dynamorm:"index:user-providers-index,sk" json:"gsi2_sk"` // Format: "{provider}#{created_at}"

	// Core provider data
	UserID       string `json:"user_id"`
	Provider     string `json:"provider"`     // "google", "github", "twitter", etc.
	ProviderID   string `json:"provider_id"` // Provider's unique ID for the user
	ProviderName string `json:"provider_name,omitempty"` // Display name from provider

	// OAuth tokens (stored encrypted)
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenExpiry  time.Time `json:"token_expiry,omitempty"`

	// Profile data from provider
	Email       string `json:"email,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`

	// Status
	IsActive  bool `json:"is_active"`
	IsPrimary bool `json:"is_primary"` // Is this the primary auth method?

	// Metadata
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// TableName returns the DynamoDB table name for the ProviderAccount model
func (ProviderAccount) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (pa *ProviderAccount) BeforeCreate() error {
	now := time.Now()
	pa.CreatedAt = now
	pa.UpdatedAt = now

	// Validate required fields
	if pa.UserID == "" {
		return fmt.Errorf("UserID is required")
	}
	if pa.Provider == "" {
		return fmt.Errorf("Provider is required")
	}
	if pa.ProviderID == "" {
		return fmt.Errorf("ProviderID is required")
	}

	// Set defaults
	if pa.IsActive == false && pa.IsPrimary == false {
		pa.IsActive = true // Default to active
	}

	// Set up primary key
	pa.PK = "user#" + pa.UserID
	pa.SK = fmt.Sprintf("provider#%s#%s", pa.Provider, pa.ProviderID)

	// Set up GSI keys
	pa.setupGSIKeys()

	return pa.Validate()
}

// BeforeUpdate sets up the model before update
func (pa *ProviderAccount) BeforeUpdate() error {
	pa.UpdatedAt = time.Now()

	// Update GSI keys in case provider or other indexed fields changed
	pa.setupGSIKeys()

	return pa.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (pa *ProviderAccount) setupGSIKeys() {
	// GSI1 - Provider lookup (find user by provider account)
	pa.GSI1PK = "PROVIDER#" + pa.Provider
	pa.GSI1SK = fmt.Sprintf("%s#%s", pa.ProviderID, pa.UserID)

	// GSI2 - User's provider accounts
	pa.GSI2PK = "USER_PROVIDERS#" + pa.UserID
	pa.GSI2SK = fmt.Sprintf("%s#%s", pa.Provider, pa.CreatedAt.Format(time.RFC3339))
}

// Validate performs validation on the ProviderAccount
func (pa *ProviderAccount) Validate() error {
	// Check required fields
	if strings.TrimSpace(pa.UserID) == "" {
		return fmt.Errorf("UserID cannot be empty")
	}
	if strings.TrimSpace(pa.Provider) == "" {
		return fmt.Errorf("Provider cannot be empty")
	}
	if strings.TrimSpace(pa.ProviderID) == "" {
		return fmt.Errorf("ProviderID cannot be empty")
	}

	// Validate provider type
	if !isValidProvider(pa.Provider) {
		return fmt.Errorf("invalid provider: %s", pa.Provider)
	}

	// Check token expiry if access token is present
	if pa.AccessToken != "" && !pa.TokenExpiry.IsZero() && pa.TokenExpiry.Before(time.Now()) {
		return fmt.Errorf("access token has expired")
	}

	return nil
}

// IsTokenExpired returns true if the access token has expired
func (pa *ProviderAccount) IsTokenExpired() bool {
	if pa.AccessToken == "" || pa.TokenExpiry.IsZero() {
		return false // No token or no expiry means not expired
	}
	return pa.TokenExpiry.Before(time.Now())
}

// NeedsRefresh returns true if the token expires soon and should be refreshed
func (pa *ProviderAccount) NeedsRefresh() bool {
	if pa.AccessToken == "" || pa.TokenExpiry.IsZero() {
		return false
	}
	// Refresh if token expires in next 5 minutes
	return pa.TokenExpiry.Before(time.Now().Add(5 * time.Minute))
}

// MarkUsed updates the last used timestamp
func (pa *ProviderAccount) MarkUsed() {
	now := time.Now()
	pa.LastUsedAt = &now
}

// SetPrimary marks this provider account as the primary authentication method
func (pa *ProviderAccount) SetPrimary() {
	pa.IsPrimary = true
}

// ClearPrimary removes the primary status from this provider account
func (pa *ProviderAccount) ClearPrimary() {
	pa.IsPrimary = false
}

// isValidProvider checks if the provider type is supported
func isValidProvider(provider string) bool {
	validProviders := map[string]bool{
		"google":   true,
		"github":   true,
		"twitter":  true,
		"facebook": true,
		"discord":  true,
		"apple":    true,
		"mastodon": true,
	}
	return validProviders[strings.ToLower(provider)]
}

// GetDisplayName returns the best available display name
func (pa *ProviderAccount) GetDisplayName() string {
	if pa.DisplayName != "" {
		return pa.DisplayName
	}
	if pa.ProviderName != "" {
		return pa.ProviderName
	}
	if pa.Username != "" {
		return pa.Username
	}
	return pa.Email
}

// HasValidToken returns true if the account has a valid, non-expired access token
func (pa *ProviderAccount) HasValidToken() bool {
	return pa.AccessToken != "" && !pa.IsTokenExpired()
}