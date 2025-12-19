package models

import (
	"fmt"
	"time"
)

// OAuthApp represents an OAuth application stored in DynamoDB
type OAuthApp struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // OAUTH_APP#{clientID}
	SK string `dynamorm:"sk,attr:SK" json:"-"` // METADATA

	// GSI for querying by name
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"` // OAUTH_APPS
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"` // {name}#{clientID}

	// OAuth app data
	ClientID     string    `dynamorm:"attr:clientID" json:"client_id"`
	ClientSecret string    `dynamorm:"attr:clientSecret" json:"client_secret"`
	Name         string    `dynamorm:"attr:name" json:"name"`
	RedirectURIs []string  `dynamorm:"attr:redirectURIs" json:"redirect_uris"`
	Scopes       []string  `dynamorm:"attr:scopes" json:"scopes"`
	CreatedAt    time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt    time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
	CreatedBy    string    `dynamorm:"attr:createdBy" json:"created_by"` // User who created the app
	Website      string    `dynamorm:"attr:website" json:"website,omitempty"`
	Description  string    `dynamorm:"attr:description" json:"description,omitempty"`
	Active       bool      `dynamorm:"attr:active" json:"active"`
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
