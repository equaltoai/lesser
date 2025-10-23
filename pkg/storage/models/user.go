package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// User represents a user account stored in DynamoDB using DynamORM
type User struct {
	// Primary key - using username as the primary identifier
	PK string `dynamorm:"pk" json:"pk"` // Format: "USER#{username}" - MUST match legacy exactly
	SK string `dynamorm:"sk" json:"sk"` // Format: "METADATA" - MUST match legacy exactly

	// GSI1 - User listing and pagination (legacy uses GSI1 for user lists)
	GSI1PK string `dynamorm:"index:user-list-index,pk" json:"gsi1_pk"` // Format: "USERS"
	GSI1SK string `dynamorm:"index:user-list-index,sk" json:"gsi1_sk"` // Format: "{created_at}#{username}"

	// GSI2 - Email lookup for authentication (legacy uses GSI2 for email)
	GSI2PK string `dynamorm:"index:email-index,pk" json:"gsi2_pk,omitempty"` // Format: "EMAIL#{email}"
	GSI2SK string `dynamorm:"index:email-index,sk" json:"gsi2_sk,omitempty"` // Format: "USERNAME#{username}"

	// GSI3 - Role-based queries
	GSI3PK string `dynamorm:"index:role-index,pk" json:"gsi3_pk"` // Format: "ROLE#{role}"
	GSI3SK string `dynamorm:"index:role-index,sk" json:"gsi3_sk"` // Format: "{username}"

	// GSI4 - Status-based queries (approved, suspended, etc.)
	GSI4PK string `dynamorm:"index:status-index,pk" json:"gsi4_pk"` // Format: "STATUS#{status}"
	GSI4SK string `dynamorm:"index:status-index,sk" json:"gsi4_sk"` // Format: "{username}"

	// GSI5 - Handle prefix search (optimized begins_with queries)
	GSI5PK string `dynamorm:"index:gsi5,pk" json:"gsi5_pk"`
	GSI5SK string `dynamorm:"index:gsi5,sk" json:"gsi5_sk"`

	// Core user data
	Username     string              `json:"username"`
	Email        string              `json:"email,omitempty"`         // Optional - not required for email-free auth
	PasswordHash string              `json:"password_hash,omitempty"` // Optional - not required for passkey/wallet auth
	DisplayName  string              `json:"display_name,omitempty"`  // Display name for the user
	Note         string              `json:"note,omitempty"`          // Profile bio / summary
	Avatar       string              `json:"avatar,omitempty"`        // Avatar image URL
	Header       string              `json:"header,omitempty"`        // Header image URL
	URL          string              `json:"url,omitempty"`           // Profile URL
	Locked       bool                `json:"locked"`
	Discoverable bool                `json:"discoverable"`
	Fields       []map[string]string `json:"fields,omitempty"` // Custom profile metadata fields
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Approved     bool                `json:"approved"`
	Suspended    bool                `json:"suspended"`
	Silenced     bool                `json:"silenced"`
	Role         string              `json:"role"` // user, moderator, admin
	Locale       string              `json:"locale,omitempty"`

	// Recovery options (email-free)
	RecoveryMethods []string `json:"recovery_methods,omitempty"` // ["passkey", "wallet", "social", "recovery_code"]

	// NSFW Content Preferences
	AllowNSFW          bool                   `json:"allow_nsfw"`           // Whether user allows viewing NSFW content
	RequireNSFWWarning bool                   `json:"require_nsfw_warning"` // Whether user wants warnings before showing NSFW content
	Metadata           map[string]interface{} `json:"metadata,omitempty"`

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// TableName returns the DynamoDB table name for the User model
func (User) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (u *User) BeforeCreate() error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	// Set default role if not specified
	if err := common.ValidateRequiredParam("u.Role", u.Role); err != nil {
		u.Role = "user"
	}

	// Set safe default NSFW preferences (conservative defaults for new users)
	// Users can opt-in to NSFW content after registration
	u.AllowNSFW = false         // Default: block NSFW content
	u.RequireNSFWWarning = true // Default: show warnings even when NSFW is allowed
	// Set up primary key - matches legacy exactly
	u.PK = "USER#" + u.Username
	u.SK = SKMetadata

	// Set up GSI keys
	u.setupGSIKeys()

	return nil
}

// BeforeUpdate sets up the model before update
func (u *User) BeforeUpdate() error {
	u.UpdatedAt = time.Now()

	// Update GSI keys in case email or other indexed fields changed
	u.setupGSIKeys()

	return nil
}

// setupGSIKeys configures all GSI partition and sort keys
func (u *User) setupGSIKeys() {
	username := u.Username

	// GSI1 - User listing and pagination (legacy GSI1 pattern)
	u.GSI1PK = "USERS"
	u.GSI1SK = fmt.Sprintf("%s#%s", u.CreatedAt.Format(time.RFC3339), username)

	// GSI2 - Email lookup (legacy GSI2 pattern - only if email is provided)
	if u.Email != "" {
		u.GSI2PK = "EMAIL#" + strings.ToLower(u.Email)
		u.GSI2SK = "USERNAME#" + username
	} else {
		u.GSI2PK = ""
		u.GSI2SK = ""
	}

	// GSI3 - Role-based queries
	u.GSI3PK = "ROLE#" + u.Role
	u.GSI3SK = username

	// GSI4 - Status-based queries
	status := u.getStatusString()
	u.GSI4PK = "STATUS#" + status
	u.GSI4SK = username

	// GSI5 - Handle prefix search (lowercased username for lexicographic match)
	normalizedUsername := strings.ToLower(username)
	prefix := normalizedUsername
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	u.GSI5PK = fmt.Sprintf("USER_HANDLE_PREFIX#%s", prefix)
	u.GSI5SK = normalizedUsername

}

// getStatusString returns a string representation of the user's status
func (u *User) getStatusString() string {
	if u.Suspended {
		return "suspended"
	}
	if u.Silenced {
		return "silenced"
	}
	if !u.Approved {
		return "pending"
	}
	return "active"
}

// IsActive returns true if the user is active (approved and not suspended/silenced)
func (u *User) IsActive() bool {
	return u.Approved && !u.Suspended && !u.Silenced
}

// HasEmail returns true if the user has an email address
func (u *User) HasEmail() bool {
	return strings.TrimSpace(u.Email) != ""
}

// HasPassword returns true if the user has a password hash
func (u *User) HasPassword() bool {
	return strings.TrimSpace(u.PasswordHash) != ""
}

// IsAdmin returns true if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// IsModerator returns true if the user has moderator or admin role
func (u *User) IsModerator() bool {
	return u.Role == "moderator" || u.Role == "admin"
}

// UpdateKeys updates the GSI keys for this user (required by DynamORM)
func (u *User) UpdateKeys() error {
	u.setupGSIKeys()
	return nil
}

// GetPK returns the partition key (required by BaseModel interface)
func (u *User) GetPK() string {
	return u.PK
}

// GetSK returns the sort key (required by BaseModel interface)
func (u *User) GetSK() string {
	return u.SK
}
