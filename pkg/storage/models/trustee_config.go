package models

import (
	"fmt"
	"time"
)

// TrusteeConfig represents a trusted contact for social recovery
type TrusteeConfig struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // TRUSTEE#CONFIG
	SK string `dynamorm:"sk" json:"-"` // {category} (e.g., recovery#{username}, moderation#{username})

	// Attributes from interface
	Username  string    `json:"username"` // Who owns this trustee relationship
	ActorID   string    `json:"actor_id"` // @friend@mastodon.social
	AddedAt   time.Time `json:"added_at"`
	Confirmed bool      `json:"confirmed"`

	// Additional attributes
	Category         string     `json:"category"`    // recovery, moderation, emergency
	TrustLevel       string     `json:"trust_level"` // full, limited, emergency_only
	ConfirmedAt      *time.Time `json:"confirmed_at,omitempty"`
	LastUsed         *time.Time `json:"last_used,omitempty"`
	UsageCount       int        `json:"usage_count"`
	RecoveryPriority int        `json:"recovery_priority"`     // Order in recovery process (1 = first)
	Permissions      []string   `json:"permissions,omitempty"` // Specific permissions granted
	Notes            string     `json:"notes,omitempty"`       // User notes about this trustee
	UpdatedAt        time.Time  `json:"updated_at"`
}

// UpdateKeys updates the partition and sort keys
func (t *TrusteeConfig) UpdateKeys() {
	t.PK = TrusteeConfigPK
	t.SK = fmt.Sprintf("%s#%s", t.Category, t.Username)
}

// NewTrusteeConfig creates a new trustee configuration
func NewTrusteeConfig(username, actorID, category string) *TrusteeConfig {
	config := &TrusteeConfig{
		Username:         username,
		ActorID:          actorID,
		Category:         category,
		AddedAt:          time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		Confirmed:        false,
		TrustLevel:       "limited",
		RecoveryPriority: 999, // Default to low priority
		Permissions:      []string{},
		UsageCount:       0,
	}
	config.UpdateKeys()
	return config
}

// GetTrusteeConfigKey returns the key for retrieving a specific trustee config
func GetTrusteeConfigKey(category, username string) (pk, sk string) {
	return TrusteeConfigPK, fmt.Sprintf("%s#%s", category, username)
}

// GetTrusteeConfigsByUserKeys returns keys for querying all trustees for a user
func GetTrusteeConfigsByUserKeys(_ string) (pk, skStart, skEnd string) {
	// This would require a GSI to efficiently query by username
	// For now, return pattern that would need scanning
	return TrusteeConfigPK, "", ""
}

// GetTrusteeConfigsByCategoryKeys returns keys for querying all trustees in a category
func GetTrusteeConfigsByCategoryKeys(category string) (pk, skPrefix string) {
	return TrusteeConfigPK, fmt.Sprintf("%s#", category)
}

// Confirm marks the trustee relationship as confirmed
func (t *TrusteeConfig) Confirm() {
	t.Confirmed = true
	now := time.Now().UTC()
	t.ConfirmedAt = &now
	t.UpdatedAt = now
}

// RecordUsage updates usage tracking
func (t *TrusteeConfig) RecordUsage() {
	now := time.Now().UTC()
	t.LastUsed = &now
	t.UsageCount++
	t.UpdatedAt = now
}

// IsActive checks if the trustee is active and confirmed
func (t *TrusteeConfig) IsActive() bool {
	return t.Confirmed
}

// CanPerformAction checks if trustee has permission for an action
func (t *TrusteeConfig) CanPerformAction(action string) bool {
	// Check trust level first
	switch t.TrustLevel {
	case "full":
		return true // Full trust can do anything
	case "emergency_only":
		// Only emergency actions allowed
		if action != "emergency_recovery" {
			return false
		}
	}

	// Check specific permissions
	for _, perm := range t.Permissions {
		if perm == action || perm == "*" {
			return true
		}
	}

	return false
}

// SetPermissions sets the permissions list
func (t *TrusteeConfig) SetPermissions(permissions []string) {
	t.Permissions = permissions
	t.UpdatedAt = time.Now().UTC()
}

// AddPermission adds a permission if not already present
func (t *TrusteeConfig) AddPermission(permission string) bool {
	for _, p := range t.Permissions {
		if p == permission {
			return false // Already has permission
		}
	}
	t.Permissions = append(t.Permissions, permission)
	t.UpdatedAt = time.Now().UTC()
	return true
}

// RemovePermission removes a permission
func (t *TrusteeConfig) RemovePermission(permission string) bool {
	for i, p := range t.Permissions {
		if p == permission {
			t.Permissions = append(t.Permissions[:i], t.Permissions[i+1:]...)
			t.UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

// GetTrustLevelPriority returns numeric priority for trust levels
func (t *TrusteeConfig) GetTrustLevelPriority() int {
	switch t.TrustLevel {
	case "full":
		return 1
	case "limited":
		return 2
	case "emergency_only":
		return 3
	default:
		return 999
	}
}

// IsRecoveryTrustee checks if this is a recovery trustee
func (t *TrusteeConfig) IsRecoveryTrustee() bool {
	return t.Category == "recovery"
}

// IsModerationTrustee checks if this is a moderation trustee
func (t *TrusteeConfig) IsModerationTrustee() bool {
	return t.Category == "moderation"
}

// FormatDisplayName returns a display-friendly name
func (t *TrusteeConfig) FormatDisplayName() string {
	return fmt.Sprintf("%s (%s trustee)", t.ActorID, t.Category)
}
