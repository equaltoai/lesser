package models

import (
	"fmt"
	"time"
)

// OAuthApp represents an OAuth application stored in DynamoDB
type OAuthApp struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // OAUTH_APP#{clientID}
	SK string `dynamorm:"sk" json:"-"` // METADATA

	// GSI for querying by name
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // OAUTH_APPS
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // {name}#{clientID}

	// OAuth app data
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	Name         string    `json:"name"`
	RedirectURIs []string  `json:"redirect_uris"`
	Scopes       []string  `json:"scopes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    string    `json:"created_by"` // User who created the app
	Website      string    `json:"website,omitempty"`
	Description  string    `json:"description,omitempty"`
	Active       bool      `json:"active"`
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
