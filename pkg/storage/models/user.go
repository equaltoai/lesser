package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// User represents a user account stored in DynamoDB using DynamORM
type User struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - using username as the primary identifier
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "USER#{username}"`
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "METADATA"`

	// GSI1 - User listing and pagination
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "USERS"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{created_at}#{username}"`

	// GSI2 - REMOVED: Email lookup is obsolete - email is forbidden
	// Email-based authentication is not supported - wallet/passkey only

	// GSI3 - Role-based queries
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK" json:"gsi3_pk"` // Format: "ROLE#{role}"
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK" json:"gsi3_sk"` // Format: "{username}"

	// GSI4 - Status-based queries (approved, suspended, etc.)
	GSI4PK string `theorydb:"index:gsi4,pk,attr:gsi4PK" json:"gsi4_pk"` // Format: "STATUS#{status}"
	GSI4SK string `theorydb:"index:gsi4,sk,attr:gsi4SK" json:"gsi4_sk"` // Format: "{username}"

	// GSI5 - Handle prefix search (optimized begins_with queries)
	GSI5PK string `theorydb:"index:gsi5,pk,attr:gsi5PK" json:"gsi5_pk"`
	GSI5SK string `theorydb:"index:gsi5,sk,attr:gsi5SK" json:"gsi5_sk"`

	// Core user data
	Username     string              `theorydb:"attr:username" json:"username"`
	Email        string              `theorydb:"attr:email" json:"email,omitempty"`                // Optional - not required for email-free auth
	PasswordHash string              `theorydb:"attr:passwordHash" json:"password_hash,omitempty"` // Optional - not required for passkey/wallet auth
	DisplayName  string              `theorydb:"attr:displayName" json:"display_name,omitempty"`   // Display name for the user
	Note         string              `theorydb:"attr:note" json:"note,omitempty"`                  // Profile bio / summary
	Avatar       string              `theorydb:"attr:avatar" json:"avatar,omitempty"`              // Avatar image URL
	Header       string              `theorydb:"attr:header" json:"header,omitempty"`              // Header image URL
	URL          string              `theorydb:"attr:url" json:"url,omitempty"`                    // Profile URL
	Locked       bool                `theorydb:"attr:locked" json:"locked"`
	Discoverable bool                `theorydb:"attr:discoverable" json:"discoverable"`
	Fields       []map[string]string `theorydb:"attr:fields" json:"fields,omitempty"` // Custom profile metadata fields
	CreatedAt    time.Time           `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt    time.Time           `theorydb:"attr:updatedAt" json:"updated_at"`
	Approved     bool                `theorydb:"attr:approved" json:"approved"`
	Suspended    bool                `theorydb:"attr:suspended" json:"suspended"`
	Silenced     bool                `theorydb:"attr:silenced" json:"silenced"`
	Role         string              `theorydb:"attr:role" json:"role"` // user, moderator, admin
	Locale       string              `theorydb:"attr:locale" json:"locale,omitempty"`

	// Recovery options (email-free)
	RecoveryMethods []string `theorydb:"attr:recoveryMethods" json:"recovery_methods,omitempty"` // ["passkey", "wallet", "social", "recovery_code"]

	// NSFW Content Preferences
	AllowNSFW          bool                   `theorydb:"attr:allowNSFW" json:"allow_nsfw"`                    // Whether user allows viewing NSFW content
	RequireNSFWWarning bool                   `theorydb:"attr:requireNSFWWarning" json:"require_nsfw_warning"` // Whether user wants warnings before showing NSFW content
	Metadata           map[string]interface{} `theorydb:"attr:metadata" json:"metadata,omitempty"`

	// Version for optimistic locking
	Version int `theorydb:"version,attr:version" json:"version"`
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

	// Email is DISALLOWED - wallet-based auth only
	if strings.TrimSpace(u.Email) != "" {
		return fmt.Errorf("email is not supported - wallet authentication only")
	}

	// Set default role if not specified
	if err := common.ValidateRequiredParam("u.Role", u.Role); err != nil {
		u.Role = "user"
	}

	// Set safe default NSFW preferences (conservative defaults for new users)
	// Users can opt-in to NSFW content after registration
	u.AllowNSFW = false         // Default: block NSFW content
	u.RequireNSFWWarning = true // Default: show warnings even when NSFW is allowed
	// Set up primary key
	u.PK = "USER#" + u.Username
	u.SK = SKMetadata

	// Set up GSI keys
	u.setupGSIKeys()

	return nil
}

// BeforeUpdate sets up the model before update
func (u *User) BeforeUpdate() error {
	u.UpdatedAt = time.Now()

	// Email is DISALLOWED - wallet-based auth only
	if strings.TrimSpace(u.Email) != "" {
		return fmt.Errorf("email is not supported - wallet authentication only")
	}

	// Update GSI keys in case email or other indexed fields changed
	u.setupGSIKeys()

	return nil
}

// setupGSIKeys configures all GSI partition and sort keys
func (u *User) setupGSIKeys() {
	username := u.Username

	// GSI1 - User listing and pagination
	u.GSI1PK = "USERS"
	u.GSI1SK = fmt.Sprintf("%s#%s", u.CreatedAt.Format(time.RFC3339), username)

	// GSI2 - REMOVED: No email index needed - email is forbidden

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
	// Set primary keys
	if err := common.ValidateRequiredParam("u.Username", u.Username); err != nil {
		return err
	}
	u.PK = "USER#" + u.Username
	u.SK = SKMetadata

	// Set GSI keys
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
