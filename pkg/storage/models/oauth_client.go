package models

import (
	"fmt"
	"math"
	"time"
)

// OAuthClient represents an OAuth 2.0 client application
type OAuthClient struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK             string  `theorydb:"pk,attr:PK" json:"-"`                                       // OAUTH_CLIENT#clientID
	SK             string  `theorydb:"sk,attr:SK" json:"-"`                                       // CLIENT
	GSI1PK         *string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`              // OWNER#ownerID (for owner index)
	GSI1SK         *string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`              // CLIENT#clientID
	OAuthClientsPK string  `theorydb:"index:oauth-clients-index,pk,attr:oauthClientsPK" json:"-"` // OAUTH_CLIENTS
	OAuthClientsSK string  `theorydb:"index:oauth-clients-index,sk,attr:oauthClientsSK" json:"-"` // CREATED_AT#{ts_desc}#CLIENT#{clientID}

	// Core fields
	ID                                 string    `theorydb:"attr:id" json:"id,omitempty"`
	ClientID                           string    `theorydb:"attr:clientID" json:"client_id"`
	ClientSecret                       string    `theorydb:"attr:clientSecret" json:"client_secret"`
	PreviousClientSecret               string    `theorydb:"attr:previousClientSecret,omitempty" json:"-"`
	PreviousClientSecretGraceExpiresAt time.Time `theorydb:"attr:previousClientSecretGraceExpiresAt,omitempty" json:"-"`
	SecretRotatedAt                    time.Time `theorydb:"attr:secretRotatedAt,omitempty" json:"-"`
	SecretRotatedBy                    string    `theorydb:"attr:secretRotatedBy,omitempty" json:"-"`
	Name                               string    `theorydb:"attr:name" json:"name"`
	Description                        string    `theorydb:"attr:description" json:"description,omitempty"`
	Website                            string    `theorydb:"attr:website" json:"website,omitempty"`
	ClientURI                          string    `theorydb:"attr:clientURI" json:"client_uri,omitempty"`
	LogoURI                            string    `theorydb:"attr:logoURI,omitempty" json:"logo_uri,omitempty"`
	Contacts                           []string  `theorydb:"attr:contacts,omitempty" json:"contacts,omitempty"`
	TosURI                             string    `theorydb:"attr:tosURI,omitempty" json:"tos_uri,omitempty"`
	PolicyURI                          string    `theorydb:"attr:policyURI,omitempty" json:"policy_uri,omitempty"`
	SoftwareID                         string    `theorydb:"attr:softwareID" json:"software_id,omitempty"`
	SoftwareVersion                    string    `theorydb:"attr:softwareVersion" json:"software_version,omitempty"`
	RedirectURIs                       []string  `theorydb:"attr:redirectURIs" json:"redirect_uris"`
	GrantTypes                         []string  `theorydb:"attr:grantTypes" json:"grant_types,omitempty"`
	ResponseTypes                      []string  `theorydb:"attr:responseTypes" json:"response_types,omitempty"`
	Scopes                             []string  `theorydb:"attr:scopes" json:"scopes,omitempty"`
	ClientClass                        string    `theorydb:"attr:clientClass,omitempty" json:"client_class,omitempty"`
	AgentUsername                      string    `theorydb:"attr:agentUsername,omitempty" json:"agent_username,omitempty"`
	OwnerID                            string    `theorydb:"attr:ownerID,omitempty" json:"owner_id,omitempty"`
	RegistrationSource                 string    `theorydb:"attr:registrationSource,omitempty" json:"registration_source,omitempty"`
	Confidential                       bool      `theorydb:"attr:confidential" json:"confidential"`
	CreatedAt                          time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt                          time.Time `theorydb:"attr:updatedAt" json:"updated_at,omitempty"`
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
