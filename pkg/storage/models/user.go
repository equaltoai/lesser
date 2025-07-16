package models

import (
	"fmt"
	"strings"
	"time"
)

// User represents a user account stored in DynamoDB using DynamORM
type User struct {
	// Primary key - using username as the primary identifier
	PK string `dynamorm:"pk" json:"pk"` // Format: "user#{username}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "user#{username}"

	// GSI1 - Email lookup for authentication
	GSI1PK string `dynamorm:"index:email-index,pk" json:"gsi1_pk,omitempty"` // Format: "EMAIL#{email}"
	GSI1SK string `dynamorm:"index:email-index,sk" json:"gsi1_sk,omitempty"` // Format: "user#{username}"

	// GSI2 - User listing and pagination
	GSI2PK string `dynamorm:"index:user-list-index,pk" json:"gsi2_pk"` // Format: "USERS"
	GSI2SK string `dynamorm:"index:user-list-index,sk" json:"gsi2_sk"` // Format: "{created_at}#{username}"

	// GSI3 - Role-based queries
	GSI3PK string `dynamorm:"index:role-index,pk" json:"gsi3_pk"` // Format: "ROLE#{role}"
	GSI3SK string `dynamorm:"index:role-index,sk" json:"gsi3_sk"` // Format: "{username}"

	// GSI4 - Status-based queries (approved, suspended, etc.)
	GSI4PK string `dynamorm:"index:status-index,pk" json:"gsi4_pk"` // Format: "STATUS#{status}"
	GSI4SK string `dynamorm:"index:status-index,sk" json:"gsi4_sk"` // Format: "{username}"

	// Core user data
	Username     string    `json:"username"`
	Email        string    `json:"email,omitempty"`         // Optional - not required for email-free auth
	PasswordHash string    `json:"password_hash,omitempty"` // Optional - not required for passkey/wallet auth
	DisplayName  string    `json:"display_name,omitempty"`  // Display name for the user
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Approved     bool      `json:"approved"`
	Suspended    bool      `json:"suspended"`
	Silenced     bool      `json:"silenced"`
	Role         string    `json:"role"` // user, moderator, admin
	Locale       string    `json:"locale,omitempty"`

	// Recovery options (email-free)
	RecoveryMethods []string `json:"recovery_methods,omitempty"` // ["passkey", "wallet", "social", "recovery_code"]

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// TableName returns the DynamoDB table name for the User model
func (User) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (u *User) BeforeCreate() error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	// Set default role if not specified
	if u.Role == "" {
		u.Role = "user"
	}

	// Set up primary key
	u.PK = "user#" + u.Username
	u.SK = "user#" + u.Username

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

	// GSI1 - Email lookup (only if email is provided)
	if u.Email != "" {
		u.GSI1PK = "EMAIL#" + strings.ToLower(u.Email)
		u.GSI1SK = "user#" + username
	} else {
		u.GSI1PK = ""
		u.GSI1SK = ""
	}

	// GSI2 - User listing and pagination
	u.GSI2PK = "USERS"
	u.GSI2SK = fmt.Sprintf("%s#%s", u.CreatedAt.Format(time.RFC3339), username)

	// GSI3 - Role-based queries
	u.GSI3PK = "ROLE#" + u.Role
	u.GSI3SK = username

	// GSI4 - Status-based queries
	status := u.getStatusString()
	u.GSI4PK = "STATUS#" + status
	u.GSI4SK = username
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
