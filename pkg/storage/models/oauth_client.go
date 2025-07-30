package models

import (
	"time"
)

// OAuthClient represents an OAuth 2.0 client application
type OAuthClient struct {
	// DynamoDB keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"-"` // CLIENT#clientID
	SK string `dynamorm:"sk" json:"-"` // METADATA
	
	// Core fields from legacy storage.OAuthClient
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	Name         string    `json:"name"`
	Website      string    `json:"website,omitempty"`
	RedirectURIs []string  `json:"redirect_uris"`
	Scopes       []string  `json:"scopes,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// TableName returns the DynamoDB table name
func (OAuthClient) TableName() string {
	return "lesser-main"
}

// BeforeCreate sets up the keys before creating
func (o *OAuthClient) BeforeCreate() error {
	o.PK = "CLIENT#" + o.ClientID
	o.SK = "METADATA"
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now()
	}
	if o.UpdatedAt.IsZero() {
		o.UpdatedAt = o.CreatedAt
	}
	return nil
}

// BeforeUpdate sets the updated timestamp
func (o *OAuthClient) BeforeUpdate() error {
	o.UpdatedAt = time.Now()
	return nil
}