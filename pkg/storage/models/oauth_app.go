package models

import (
	"fmt"
	"time"
)

// OAuthApp represents an OAuth application stored in DynamoDB
type OAuthApp struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // OAUTH_APP#{clientID}
	SK string `theorydb:"sk,attr:SK" json:"-"` // METADATA

	// GSI for querying by name
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"` // OAUTH_APPS
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"` // {name}#{clientID}

	// OAuth app data
	ClientID     string    `theorydb:"attr:clientID" json:"client_id"`
	ClientSecret string    `theorydb:"attr:clientSecret" json:"client_secret"`
	Name         string    `theorydb:"attr:name" json:"name"`
	RedirectURIs []string  `theorydb:"attr:redirectURIs" json:"redirect_uris"`
	Scopes       []string  `theorydb:"attr:scopes" json:"scopes"`
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt    time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	CreatedBy    string    `theorydb:"attr:createdBy" json:"created_by"` // User who created the app
	Website      string    `theorydb:"attr:website" json:"website,omitempty"`
	Description  string    `theorydb:"attr:description" json:"description,omitempty"`
	Active       bool      `theorydb:"attr:active" json:"active"`
}

// UpdateKeys updates the GSI keys based on the OAuth app data
func (o *OAuthApp) UpdateKeys() {
	// Primary key
	o.PK = fmt.Sprintf("OAUTH_APP#%s", o.ClientID)
	o.SK = SKMetadata

	// GSI for listing all apps and querying by name
	o.GSI1PK = "OAUTH_APPS"
	o.GSI1SK = fmt.Sprintf("%s#%s", o.Name, o.ClientID)
}

// HasScope checks if the app has a specific scope
func (o *OAuthApp) HasScope(scope string) bool {
	for _, s := range o.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// IsValidRedirectURI checks if a redirect URI is valid for this app
func (o *OAuthApp) IsValidRedirectURI(uri string) bool {
	for _, r := range o.RedirectURIs {
		if r == uri {
			return true
		}
	}
	return false
}

// TableName returns the DynamoDB table backing OAuthApp.
func (OAuthApp) TableName() string {
	return MainTableName
}
