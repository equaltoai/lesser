package models

import (
	"fmt"
	"math"
	"time"
)

// OAuthClient represents an OAuth 2.0 client application
type OAuthClient struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys
	PK             string  `dynamorm:"pk,attr:PK" json:"-"`                                       // OAUTH_CLIENT#clientID
	SK             string  `dynamorm:"sk,attr:SK" json:"-"`                                       // CLIENT
	GSI1PK         *string `dynamorm:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`              // OWNER#ownerID (for owner index)
	GSI1SK         *string `dynamorm:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`              // CLIENT#clientID
	OAuthClientsPK string  `dynamorm:"index:oauth-clients-index,pk,attr:oauthClientsPK" json:"-"` // OAUTH_CLIENTS
	OAuthClientsSK string  `dynamorm:"index:oauth-clients-index,sk,attr:oauthClientsSK" json:"-"` // CREATED_AT#{ts_desc}#CLIENT#{clientID}

	// Core fields
	ID           string    `dynamorm:"attr:id" json:"id,omitempty"`
	ClientID     string    `dynamorm:"attr:clientID" json:"client_id"`
	ClientSecret string    `dynamorm:"attr:clientSecret" json:"client_secret"`
	Name         string    `dynamorm:"attr:name" json:"name"`
	Description  string    `dynamorm:"attr:description" json:"description,omitempty"`
	Website      string    `dynamorm:"attr:website" json:"website,omitempty"`
	RedirectURIs []string  `dynamorm:"attr:redirectURIs" json:"redirect_uris"`
	GrantTypes   []string  `dynamorm:"attr:grantTypes" json:"grant_types,omitempty"`
	Scopes       []string  `dynamorm:"attr:scopes" json:"scopes,omitempty"`
	OwnerID      string    `dynamorm:"attr:ownerID,omitempty" json:"owner_id,omitempty"`
	Confidential bool      `dynamorm:"attr:confidential" json:"confidential"`
	CreatedAt    time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt    time.Time `dynamorm:"attr:updatedAt" json:"updated_at,omitempty"`
}

// TableName returns the DynamoDB table name
func (OAuthClient) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (o *OAuthClient) BeforeCreate() error {
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	if o.UpdatedAt.IsZero() {
		o.UpdatedAt = o.CreatedAt
	}
	if err := o.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return nil
}

// BeforeUpdate sets the updated timestamp
func (o *OAuthClient) BeforeUpdate() error {
	o.UpdatedAt = time.Now().UTC()
	if err := o.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (o *OAuthClient) GetPK() string {
	return o.PK
}

// GetSK returns the sort key for BaseModel interface
func (o *OAuthClient) GetSK() string {
	return o.SK
}

// UpdateKeys implements BaseModel interface and updates DynamoDB keys
func (o *OAuthClient) UpdateKeys() error {
	o.PK = "OAUTH_CLIENT#" + o.ClientID
	o.SK = "CLIENT"

	// GSI1 is for owner-based queries - only set if OwnerID exists
	// DynamoDB requires GSI keys to be non-empty if they're part of the index
	// So we either set them or leave them empty (and DynamORM should skip them)
	if o.OwnerID != "" {
		pk := "OWNER#" + o.OwnerID
		sk := "CLIENT#" + o.ClientID
		o.GSI1PK = &pk
		o.GSI1SK = &sk
	} else {
		// Clear pointers when OwnerID is empty so DynamoDB omits the index attributes
		o.GSI1PK = nil
		o.GSI1SK = nil
	}

	desc := encodeDescendingTimestamp(o.CreatedAt)
	o.OAuthClientsPK = "OAUTH_CLIENTS"
	o.OAuthClientsSK = fmt.Sprintf("CREATED_AT#%019d#CLIENT#%s", desc, o.ClientID)
	return nil
}

func encodeDescendingTimestamp(timestamp time.Time) int64 {
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return math.MaxInt64 - timestamp.UTC().UnixNano()
}
