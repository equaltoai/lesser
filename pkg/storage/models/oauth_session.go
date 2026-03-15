package models

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// OAuthAuthSession represents an OAuth authorization session during the auth flow
type OAuthAuthSession struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys for authorization sessions
	PK string `theorydb:"pk,attr:PK" json:"-"` // OAUTH_AUTH#sessionID
	SK string `theorydb:"sk,attr:SK" json:"-"` // SESSION#sessionID

	// GSI1 - User sessions lookup (optional - for authenticated users)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk,omitempty"` // USER_OAUTH#username
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk,omitempty"` // created_at#sessionID

	// GSI2 - State lookup for OAuth flow
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2_pk,omitempty"` // OAUTH_STATE#state
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2_sk,omitempty"` // sessionID

	// Core session data
	SessionID string `theorydb:"attr:sessionID" json:"session_id"`
	State     string `theorydb:"attr:state" json:"state,omitempty"` // OAuth state parameter

	// OAuth flow data
	ClientID            string   `theorydb:"attr:clientID" json:"client_id"`
	RedirectURI         string   `theorydb:"attr:redirectURI" json:"redirect_uri"`
	Scopes              []string `theorydb:"attr:scopes" json:"scopes,omitempty"`
	CodeChallenge       string   `theorydb:"attr:codeChallenge" json:"code_challenge,omitempty"`              // PKCE
	CodeChallengeMethod string   `theorydb:"attr:codeChallengeMethod" json:"code_challenge_method,omitempty"` // PKCE method

	// User data (populated after login)
	Username     string     `theorydb:"attr:username" json:"username,omitempty"`          // Set after user authenticates
	IsAuthorized bool       `theorydb:"attr:isAuthorized" json:"is_authorized"`           // User has authorized the app
	AuthorizedAt *time.Time `theorydb:"attr:authorizedAt" json:"authorized_at,omitempty"` // When user authorized

	// Security and tracking
	IPAddress string `theorydb:"attr:ipAddress" json:"ip_address,omitempty"`
	UserAgent string `theorydb:"attr:userAgent" json:"user_agent,omitempty"`
	DeviceID  string `theorydb:"attr:deviceID" json:"device_id,omitempty"`

	// Session flow tracking
	FlowStep  string                 `theorydb:"attr:flowStep" json:"flow_step"`             // login, consent, authorized, error
	FlowData  map[string]interface{} `theorydb:"attr:flowData" json:"flow_data,omitempty"`   // Additional flow context
	ReturnURL string                 `theorydb:"attr:returnURL" json:"return_url,omitempty"` // URL to return to after login

	// Security measures
	CSRFToken    string `theorydb:"attr:csrfToken" json:"csrf_token"`                 // CSRF protection
	SessionNonce string `theorydb:"attr:sessionNonce" json:"session_nonce,omitempty"` // Additional entropy
	IsSecure     bool   `theorydb:"attr:isSecure" json:"is_secure"`                   // Created over HTTPS

	// Timestamps
	CreatedAt  time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	LastUsedAt time.Time `theorydb:"attr:lastUsedAt" json:"last_used_at"`

	// TTL for automatic cleanup (OAuth sessions are short-lived)
	ExpiresAt int64 `theorydb:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp for DynamoDB TTL

	// Session validation
	Version int `theorydb:"attr:version" json:"version"` // For optimistic locking
}

// TableName returns the DynamoDB table name
func (OAuthAuthSession) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the model before creation
func (o *OAuthAuthSession) BeforeCreate() error {
	now := time.Now()
	o.CreatedAt = now
	o.UpdatedAt = now
	o.LastUsedAt = now

	// Generate session ID if not provided
	if err := common.ValidateRequiredParam("SessionID", o.SessionID); err != nil {
		var genErr error
		o.SessionID, genErr = generateSecureOAuthToken(32)
		if genErr != nil {
			return fmt.Errorf("%w: %w", ErrOAuthSessionIDGenerationFailed, genErr)
		}
	}

	// Generate CSRF token for security
	if err := common.ValidateRequiredParam("CSRFToken", o.CSRFToken); err != nil {
		var genErr error
		o.CSRFToken, genErr = generateSecureOAuthToken(24)
		if genErr != nil {
			return fmt.Errorf("%w: %w", ErrOAuthCSRFTokenGenerationFailed, genErr)
		}
	}

	// Set default expiry to 1 hour for OAuth sessions
	if o.ExpiresAt == 0 {
		o.ExpiresAt = now.Add(time.Hour).Unix()
	}

	// Set default flow step
	if err := common.ValidateRequiredParam("FlowStep", o.FlowStep); err != nil {
		o.FlowStep = "initiated"
	}

	// Set up primary key
	o.PK = "OAUTH_AUTH#" + o.SessionID
	o.SK = "SESSION#" + o.SessionID

	// Set up GSI keys
	o.setupGSIKeys()

	return o.Validate()
}

// UpdateKeys implements BaseModel interface - updates the GSI keys
func (o *OAuthAuthSession) UpdateKeys() error {
	// Set primary keys
	if err := common.ValidateRequiredParam("o.SessionID", o.SessionID); err != nil {
		return err
	}

	// Set up primary key
	o.PK = "OAUTH_AUTH#" + o.SessionID
	o.SK = "SESSION#" + o.SessionID

	// Set GSI keys
	o.setupGSIKeys()
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (o *OAuthAuthSession) GetPK() string {
	return o.PK
}

// GetSK returns the sort key for BaseModel interface
func (o *OAuthAuthSession) GetSK() string {
	return o.SK
}

// BeforeUpdate sets up the model before update
func (o *OAuthAuthSession) BeforeUpdate() error {
	o.UpdatedAt = time.Now()
	o.setupGSIKeys()
	return o.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (o *OAuthAuthSession) setupGSIKeys() {
	// GSI1 - User sessions lookup (only if user is known)
	if err := common.ValidateRequiredParam("Username", o.Username); err == nil {
		o.GSI1PK = "USER_OAUTH#" + o.Username
		o.GSI1SK = fmt.Sprintf("%s#%s", o.CreatedAt.Format(time.RFC3339), o.SessionID)
	} else {
		o.GSI1PK = ""
		o.GSI1SK = ""
	}

	// GSI2 - State lookup (only if state exists)
	if err := common.ValidateRequiredParam("State", o.State); err == nil {
		o.GSI2PK = "OAUTH_STATE#" + o.State
		o.GSI2SK = o.SessionID
	} else {
		o.GSI2PK = ""
		o.GSI2SK = ""
	}
}

// Validate performs validation on the OAuth session
func (o *OAuthAuthSession) Validate() error {
	if err := common.ValidateRequiredParam("SessionID", o.SessionID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("ClientID", o.ClientID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("RedirectURI", o.RedirectURI); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("CSRFToken", o.CSRFToken); err != nil {
		return err
	}
	if o.ExpiresAt <= 0 {
		return ErrExpiresAtRequired
	}

	return nil
}

// Touch updates the last used timestamp
func (o *OAuthAuthSession) Touch() {
	o.LastUsedAt = time.Now()
}

// SetUser sets the username and updates GSI keys
func (o *OAuthAuthSession) SetUser(username string) {
	o.Username = username
	o.setupGSIKeys()
}

// Authorize marks the session as authorized by the user
func (o *OAuthAuthSession) Authorize() {
	o.IsAuthorized = true
	now := time.Now()
	o.AuthorizedAt = &now
	o.FlowStep = "authorized"
}

// SetFlowStep updates the current flow step and optional data
func (o *OAuthAuthSession) SetFlowStep(step string, data map[string]interface{}) {
	o.FlowStep = step
	if data != nil {
		if o.FlowData == nil {
			o.FlowData = make(map[string]interface{})
		}
		for k, v := range data {
			o.FlowData[k] = v
		}
	}
}

// IsValid returns true if the session is valid (not expired)
func (o *OAuthAuthSession) IsValid() bool {
	return time.Unix(o.ExpiresAt, 0).After(time.Now())
}

// IsExpired returns true if the session has expired
func (o *OAuthAuthSession) IsExpired() bool {
	return time.Unix(o.ExpiresAt, 0).Before(time.Now())
}

// CanAuthorize returns true if the session is ready for authorization
func (o *OAuthAuthSession) CanAuthorize() bool {
	return common.ValidateRequiredParam("Username", o.Username) == nil && o.FlowStep == "consent" && o.IsValid()
}

// RemainingTime returns the time until the session expires
func (o *OAuthAuthSession) RemainingTime() time.Duration {
	return time.Until(time.Unix(o.ExpiresAt, 0))
}

// SetContext sets a context value in flow data
func (o *OAuthAuthSession) SetContext(key string, value interface{}) {
	if o.FlowData == nil {
		o.FlowData = make(map[string]interface{})
	}
	o.FlowData[key] = value
}

// GetContext gets a context value from flow data
func (o *OAuthAuthSession) GetContext(key string) (interface{}, bool) {
	if o.FlowData == nil {
		return nil, false
	}
	value, exists := o.FlowData[key]
	return value, exists
}

// generateSecureOAuthToken generates a cryptographically secure random token
func generateSecureOAuthToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
