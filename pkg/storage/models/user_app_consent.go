package models

import (
	"fmt"
	"time"
)

// UserAppConsent represents user consent for an OAuth app stored in DynamoDB
type UserAppConsent struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // USER#userID
	SK string `dynamorm:"sk,attr:SK" json:"-"` // CONSENT#appID

	// GSI for querying by app
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsI1PK" json:"-"` // APP#appID
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsI1SK" json:"-"` // USER#userID

	// Consent data
	UserID    string     `dynamorm:"attr:userID" json:"user_id"`
	AppID     string     `dynamorm:"attr:appID" json:"app_id"` // OAuth app client ID
	Scopes    []string   `dynamorm:"attr:scopes" json:"scopes"`
	CreatedAt time.Time  `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`
	RevokedAt *time.Time `dynamorm:"attr:revokedAt" json:"revoked_at,omitempty"`
	Active    bool       `dynamorm:"attr:active" json:"active"`
}

// GetPK returns the partition key for BaseModel interface
func (c *UserAppConsent) GetPK() string {
	return c.PK
}

// GetSK returns the sort key for BaseModel interface
func (c *UserAppConsent) GetSK() string {
	return c.SK
}

// UpdateKeys implements BaseModel interface and updates the GSI keys based on the consent data
func (c *UserAppConsent) UpdateKeys() error {
	// Primary key - for user's consent list
	c.PK = fmt.Sprintf("USER#%s", c.UserID)
	c.SK = fmt.Sprintf("CONSENT#%s", c.AppID)

	// GSI - for app's authorized users list
	c.GSI1PK = fmt.Sprintf("APP#%s", c.AppID)
	c.GSI1SK = fmt.Sprintf("USER#%s", c.UserID)
	return nil
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

// TableName returns the DynamoDB table backing UserAppConsent.
func (UserAppConsent) TableName() string {
	return MainTableName
}
