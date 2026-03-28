package models

import (
	"fmt"
	"strings"
	"time"
)

// UserAppConsent represents user consent for an OAuth app stored in DynamoDB
type UserAppConsent struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // USER#userID
	SK string `theorydb:"sk,attr:SK" json:"-"` // CONSENT#appID

	// GSI for querying by app
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"` // APP#appID
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"` // USER#userID

	// Consent data
	UserID    string     `theorydb:"attr:userID" json:"user_id"`
	AppID     string     `theorydb:"attr:appID" json:"app_id"` // OAuth app client ID
	Resource  string     `theorydb:"attr:resource,omitempty" json:"resource,omitempty"`
	Scopes    []string   `theorydb:"attr:scopes" json:"scopes"`
	CreatedAt time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time  `theorydb:"attr:updatedAt" json:"updated_at"`
	RevokedAt *time.Time `theorydb:"attr:revokedAt" json:"revoked_at,omitempty"`
	Active    bool       `theorydb:"attr:active" json:"active"`
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
	c.SK = userAppConsentSortKey(c.AppID, c.Resource)

	// GSI - for app's authorized users list
	c.GSI1PK = fmt.Sprintf("APP#%s", c.AppID)
	c.GSI1SK = userAppConsentUserIndexSortKey(c.UserID, c.Resource)
	return nil
}

func userAppConsentSortKey(appID, resource string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return fmt.Sprintf("CONSENT#%s", appID)
	}
	return fmt.Sprintf("CONSENT#%s#RESOURCE#%s", appID, resource)
}

func userAppConsentUserIndexSortKey(userID, resource string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return fmt.Sprintf("USER#%s", userID)
	}
	return fmt.Sprintf("USER#%s#RESOURCE#%s", userID, resource)
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
