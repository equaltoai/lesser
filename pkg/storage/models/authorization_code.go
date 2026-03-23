package models

import (
	"time"
)

// AuthorizationCode represents an OAuth 2.0 authorization code
type AuthorizationCode struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"-"` // AUTHCODE#code
	SK string `theorydb:"sk,attr:SK" json:"-"` // CODE

	// Core fields from legacy storage.AuthorizationCode
	Code              string    `theorydb:"attr:code" json:"Code"`
	ClientID          string    `theorydb:"attr:clientID" json:"ClientID"`
	RedirectURI       string    `theorydb:"attr:redirectURI" json:"RedirectURI"`
	Resource          string    `theorydb:"attr:resource,omitempty" json:"Resource,omitempty"`
	Username          string    `theorydb:"attr:username" json:"Username"`
	PrincipalUsername string    `theorydb:"attr:principalUsername,omitempty" json:"PrincipalUsername,omitempty"`
	AgentUsername     string    `theorydb:"attr:agentUsername,omitempty" json:"AgentUsername,omitempty"`
	CodeChallenge     string    `theorydb:"attr:codeChallenge" json:"CodeChallenge"`
	ExpiresAt         time.Time `theorydb:"attr:expiresAt" json:"ExpiresAt"`
	Scopes            []string  `theorydb:"attr:scopes" json:"Scopes"`

	// Tracking fields
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`

	// TTL field for automatic expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`
}

// TableName returns the DynamoDB table name
func (AuthorizationCode) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys and TTL before creating
func (a *AuthorizationCode) BeforeCreate() error {
	a.PK = "AUTHCODE#" + a.Code
	a.SK = "CODE"

	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	// Set TTL if not already set
	if a.TTL == 0 && !a.ExpiresAt.IsZero() {
		a.TTL = a.ExpiresAt.Unix()
	}

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (a *AuthorizationCode) GetPK() string {
	return a.PK
}

// GetSK returns the sort key for BaseModel interface
func (a *AuthorizationCode) GetSK() string {
	return a.SK
}

// UpdateKeys implements BaseModel interface and updates DynamoDB keys
func (a *AuthorizationCode) UpdateKeys() error {
	a.PK = "AUTHCODE#" + a.Code
	a.SK = "CODE"
	return nil
}
