package models

import (
	"fmt"
	"time"
)

// UserAppConsent represents user consent for an OAuth app stored in DynamoDB
type UserAppConsent struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // USER#userID
	SK string `dynamorm:"sk" json:"-"` // CONSENT#appID

	// GSI for querying by app
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // APP#appID
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // USER#userID

	// Consent data
	UserID    string     `json:"user_id"`
	AppID     string     `json:"app_id"` // OAuth app client ID
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	Active    bool       `json:"active"`
}

// UpdateKeys updates the GSI keys based on the consent data
func (c *UserAppConsent) UpdateKeys() {
	// Primary key - for user's consent list
	c.PK = fmt.Sprintf(KeyPatternUser, c.UserID)
	c.SK = fmt.Sprintf("CONSENT#%s", c.AppID)

	// GSI - for app's authorized users list
	c.GSI1PK = fmt.Sprintf("APP#%s", c.AppID)
	c.GSI1SK = fmt.Sprintf(KeyPatternUser, c.UserID)
}

// HasScope checks if the consent includes a specific scope
func (c *UserAppConsent) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Revoke marks the consent as revoked
func (c *UserAppConsent) Revoke() {
	now := time.Now()
	c.RevokedAt = &now
	c.Active = false
	c.UpdatedAt = now
}

// IsValid checks if the consent is still valid
func (c *UserAppConsent) IsValid() bool {
	return c.Active && c.RevokedAt == nil
}
