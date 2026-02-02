package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ProviderAccount represents an OAuth provider account linked to a user
type ProviderAccount struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - using composite key for user + provider relationship
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "user#{userID}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "provider#{provider}#{providerID}"

	// GSI1 - Provider lookup (find user by provider account)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "PROVIDER#{provider}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{providerID}#{userID}"

	// GSI2 - User's provider accounts
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "USER_PROVIDERS#{userID}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{provider}#{created_at}"

	// Core provider data
	UserID       string `theorydb:"attr:userID" json:"user_id"`
	Provider     string `theorydb:"attr:provider" json:"provider"`                    // "google", "github", "twitter", etc.
	ProviderID   string `theorydb:"attr:providerID" json:"provider_id"`               // Provider's unique ID for the user
	ProviderName string `theorydb:"attr:providerName" json:"provider_name,omitempty"` // Display name from provider

	// OAuth tokens (stored encrypted)
	AccessToken  string    `theorydb:"attr:accessToken" json:"access_token,omitempty"`
	RefreshToken string    `theorydb:"attr:refreshToken" json:"refresh_token,omitempty"`
	TokenExpiry  time.Time `theorydb:"attr:tokenExpiry" json:"token_expiry,omitempty"`

	// Profile data from provider
	Email       string `theorydb:"attr:email" json:"email,omitempty"`
	Username    string `theorydb:"attr:username" json:"username,omitempty"`
	DisplayName string `theorydb:"attr:displayName" json:"display_name,omitempty"`
	AvatarURL   string `theorydb:"attr:avatarURL" json:"avatar_url,omitempty"`

	// Status
	IsActive  bool `theorydb:"attr:isActive" json:"is_active"`
	IsPrimary bool `theorydb:"attr:isPrimary" json:"is_primary"` // Is this the primary auth method?

	// Metadata
	CreatedAt  time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt  time.Time  `theorydb:"attr:updatedAt" json:"updated_at"`
	LastUsedAt *time.Time `theorydb:"attr:lastUsedAt" json:"last_used_at,omitempty"`

	// Version for optimistic locking
	Version int `theorydb:"version,attr:version" json:"version"`
}

// TableName returns the DynamoDB table name for the ProviderAccount model
func (ProviderAccount) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (pa *ProviderAccount) BeforeCreate() error {
	now := time.Now()
	pa.CreatedAt = now
	pa.UpdatedAt = now

	// Validate required fields
	if err := common.ValidateRequiredParam("UserID", pa.UserID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("provider", pa.Provider); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("ProviderID", pa.ProviderID); err != nil {
		return err
	}

	// Set defaults
	if !pa.IsActive && !pa.IsPrimary {
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
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(pa.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("provider", strings.TrimSpace(pa.Provider)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("ProviderID", strings.TrimSpace(pa.ProviderID)); err != nil {
		return err
	}

	// Validate provider type
	if !isValidProvider(pa.Provider) {
		return fmt.Errorf("%w: %s", ErrInvalidProvider, pa.Provider)
	}

	// Check token expiry if access token is present
	if err := common.ValidateRequiredParam("AccessToken", ""); err == nil && pa.AccessToken != "" && !pa.TokenExpiry.IsZero() && pa.TokenExpiry.Before(time.Now()) {
		return ErrAccessTokenExpired
	}

	return nil
}

// IsTokenExpired returns true if the access token has expired
func (pa *ProviderAccount) IsTokenExpired() bool {
	if err := common.ValidateRequiredParam("AccessToken", pa.AccessToken); err != nil || pa.TokenExpiry.IsZero() {
		return false // No token or no expiry means not expired
	}
	return pa.TokenExpiry.Before(time.Now())
}

// NeedsRefresh returns true if the token expires soon and should be refreshed
func (pa *ProviderAccount) NeedsRefresh() bool {
	if err := common.ValidateRequiredParam("AccessToken", pa.AccessToken); err != nil || pa.TokenExpiry.IsZero() {
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
	if err := common.ValidateRequiredParam("DisplayName", pa.DisplayName); err == nil {
		return pa.DisplayName
	}
	if err := common.ValidateRequiredParam("ProviderName", pa.ProviderName); err == nil {
		return pa.ProviderName
	}
	if err := common.ValidateRequiredParam("Username", pa.Username); err == nil {
		return pa.Username
	}
	return pa.Email
}

// HasValidToken returns true if the account has a valid, non-expired access token
func (pa *ProviderAccount) HasValidToken() bool {
	return common.ValidateRequiredParam("AccessToken", pa.AccessToken) == nil && !pa.IsTokenExpired()
}
