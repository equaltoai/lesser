package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"go.uber.org/zap"
)

// WebAuthn errors
var (
	ErrChallengeNotFound    = errors.New("challenge not found or expired")
	ErrCredentialNotFound   = errors.New("credential not found")
	ErrInvalidCredential    = errors.New("invalid credential")
	ErrUserHasNoCredentials = errors.New("user has no credentials")
)

// WebAuthn constants
const (
	ChallengeDuration     = 5 * time.Minute // WebAuthn challenges expire after 5 minutes
	MaxCredentialsPerUser = 10              // Maximum passkeys per user
)

// WebAuthnService handles WebAuthn operations
type WebAuthnService struct {
	webAuthn *webauthn.WebAuthn
	repos    StorageProvider
	domain   string
}

// NewWebAuthnService creates a new WebAuthn service
func NewWebAuthnService(repos StorageProvider, domain string, displayName string) (*WebAuthnService, error) {
	// Configure WebAuthn
	wconfig := &webauthn.Config{
		RPDisplayName: displayName,                   // Display name for the site
		RPID:          domain,                        // Relying Party ID (domain)
		RPOrigins:     []string{"https://" + domain}, // Origins allowed
	}

	webAuthn, err := webauthn.New(wconfig)
	if err != nil {
		return nil, errors.Join(ErrWebAuthnServiceInit, err)
	}

	return &WebAuthnService{
		webAuthn: webAuthn,
		repos:    repos,
		domain:   domain,
	}, nil
}

// BeginRegistration starts the WebAuthn registration process
func (s *WebAuthnService) BeginRegistration(ctx context.Context, username string) (any, string, error) {
	// Get user from storage
	user, err := s.repos.Account().GetUser(ctx, username)
	if err != nil {
		return nil, "", errors.Join(ErrUserRetrieval, err)
	}

	// Create WebAuthn user adapter
	webAuthnUser := &webAuthnUser{
		id:          user.Username,
		name:        user.Username,
		displayName: user.Username,
		credentials: []webauthn.Credential{},
	}

	// Get existing credentials for exclusion
	credentials, err := s.repos.Account().GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		common.Logger().Warn("failed to get existing credentials", zap.Error(err))
	} else {
		for _, cred := range credentials {
			webAuthnUser.credentials = append(webAuthnUser.credentials, *ToWebAuthnCredential(cred))
		}
	}

	// Begin registration
	options, sessionData, err := s.webAuthn.BeginRegistration(webAuthnUser)
	if err != nil {
		return nil, "", errors.Join(ErrRegistrationBegin, err)
	}

	// Serialize session data
	sessionDataBytes, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", errors.Join(ErrSessionDataSerialization, err)
	}

	// Store challenge for verification
	challengeData := &storage.WebAuthnChallenge{
		Challenge:   sessionData.Challenge,
		UserID:      username,
		SessionData: sessionDataBytes,
		ExpiresAt:   time.Now().Add(ChallengeDuration),
		Type:        "registration",
	}

	if err := s.repos.Account().StoreWebAuthnChallenge(ctx, challengeData); err != nil {
		return nil, "", errors.Join(ErrWebAuthnChallengeStorage, err)
	}

	return options, challengeData.Challenge, nil
}

// FinishRegistration completes the WebAuthn registration process
func (s *WebAuthnService) FinishRegistration(ctx context.Context, username string, challenge string, response []byte, credentialName string) error {
	// Get challenge data
	challengeData, err := s.repos.Account().GetWebAuthnChallenge(ctx, challenge)
	if err != nil {
		return ErrChallengeNotFound
	}

	// Verify challenge belongs to user and hasn't expired
	if challengeData.UserID != username || challengeData.Type != "registration" {
		return ErrChallengeNotFound
	}
	if time.Now().After(challengeData.ExpiresAt) {
		return ErrChallengeNotFound
	}

	// Deserialize session data
	var sessionData webauthn.SessionData
	sessionBytes, ok := challengeData.SessionData.([]byte)
	if !ok {
		// Try to handle it as a string (base64 encoded)
		if sessionStr, ok := challengeData.SessionData.(string); ok {
			sessionBytes = []byte(sessionStr)
		} else {
			return ErrInvalidSessionDataType
		}
	}
	if err := json.Unmarshal(sessionBytes, &sessionData); err != nil {
		return errors.Join(ErrSessionDataDeserialization, err)
	}

	// Get user
	user, err := s.repos.Account().GetUser(ctx, username)
	if err != nil {
		return errors.Join(ErrUserRetrieval, err)
	}

	// Create WebAuthn user adapter
	webAuthnUser := &webAuthnUser{
		id:          user.Username,
		name:        user.Username,
		displayName: user.Username,
		credentials: []webauthn.Credential{},
	}

	// Parse the credential creation response
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return errors.Join(ErrCredentialResponse, err)
	}

	// Verify the credential
	credential, err := s.webAuthn.CreateCredential(webAuthnUser, sessionData, parsedResponse)
	if err != nil {
		return errors.Join(ErrCredentialCreation, err)
	}

	// Check if user has too many credentials
	existingCreds, _ := s.repos.Account().GetUserWebAuthnCredentials(ctx, username)
	if len(existingCreds) >= MaxCredentialsPerUser {
		return ErrMaxCredentialsReached
	}

	// Generate a name if not provided
	if err := common.ValidateRequiredParam("credentialName", credentialName); err != nil {
		credentialName = fmt.Sprintf("Passkey %d", len(existingCreds)+1)
	}

	// Store credential
	storedCredential := &storage.WebAuthnCredential{
		ID:              base64.StdEncoding.EncodeToString(credential.ID),
		UserID:          username,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		CreatedAt:       time.Now(),
		LastUsedAt:      time.Now(),
		Name:            credentialName,
	}

	if err := s.repos.Account().StoreWebAuthnCredential(ctx, storedCredential); err != nil {
		return errors.Join(ErrCredentialStorage, err)
	}

	// Delete the used challenge
	_ = s.repos.Account().DeleteWebAuthnChallenge(ctx, challenge)

	return nil
}

// BeginLogin starts the WebAuthn login process
func (s *WebAuthnService) BeginLogin(ctx context.Context, username string) (any, string, error) {
	// Get user credentials
	credentials, err := s.repos.Account().GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		return nil, "", errors.Join(ErrCredentialRetrieval, err)
	}

	if err := common.ValidateSliceNotEmpty("credentials", credentials); err != nil {
		return nil, "", ErrUserHasNoCredentials
	}

	// Create WebAuthn user adapter with credentials
	webAuthnUser := &webAuthnUser{
		id:          username,
		name:        username,
		displayName: username,
		credentials: []webauthn.Credential{},
	}

	for _, cred := range credentials {
		webAuthnUser.credentials = append(webAuthnUser.credentials, *ToWebAuthnCredential(cred))
	}

	// Begin login
	options, sessionData, err := s.webAuthn.BeginLogin(webAuthnUser)
	if err != nil {
		return nil, "", errors.Join(ErrLoginBegin, err)
	}

	// Serialize session data
	sessionDataBytes, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", errors.Join(ErrSessionDataSerialization, err)
	}

	// Store challenge for verification
	challengeData := &storage.WebAuthnChallenge{
		Challenge:   sessionData.Challenge,
		UserID:      username,
		SessionData: sessionDataBytes,
		ExpiresAt:   time.Now().Add(ChallengeDuration),
		Type:        "authentication",
	}

	if err := s.repos.Account().StoreWebAuthnChallenge(ctx, challengeData); err != nil {
		return nil, "", errors.Join(ErrWebAuthnChallengeStorage, err)
	}

	return options, challengeData.Challenge, nil
}

// FinishLogin completes the WebAuthn login process
func (s *WebAuthnService) FinishLogin(ctx context.Context, username string, challenge string, response []byte) (*storage.WebAuthnCredential, error) {
	// Get challenge data
	challengeData, err := s.repos.Account().GetWebAuthnChallenge(ctx, challenge)
	if err != nil {
		return nil, ErrChallengeNotFound
	}

	// Verify challenge belongs to user and hasn't expired
	if challengeData.UserID != username || challengeData.Type != "authentication" {
		return nil, ErrChallengeNotFound
	}
	if time.Now().After(challengeData.ExpiresAt) {
		return nil, ErrChallengeNotFound
	}

	// Deserialize session data
	var sessionData webauthn.SessionData
	sessionBytes, ok := challengeData.SessionData.([]byte)
	if !ok {
		// Try to handle it as a string (base64 encoded)
		if sessionStr, ok := challengeData.SessionData.(string); ok {
			sessionBytes = []byte(sessionStr)
		} else {
			return nil, ErrInvalidSessionDataType
		}
	}
	if err := json.Unmarshal(sessionBytes, &sessionData); err != nil {
		return nil, errors.Join(ErrSessionDataDeserialization, err)
	}

	// Get user credentials
	credentials, err := s.repos.Account().GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		return nil, errors.Join(ErrCredentialRetrieval, err)
	}

	// Create WebAuthn user adapter with credentials
	webAuthnUser := &webAuthnUser{
		id:          username,
		name:        username,
		displayName: username,
		credentials: []webauthn.Credential{},
	}

	credentialMap := make(map[string]*storage.WebAuthnCredential)
	for _, cred := range credentials {
		webAuthnCred := ToWebAuthnCredential(cred)
		webAuthnUser.credentials = append(webAuthnUser.credentials, *webAuthnCred)
		credentialMap[base64.StdEncoding.EncodeToString(webAuthnCred.ID)] = cred
	}

	// Parse the credential assertion response
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return nil, errors.Join(ErrCredentialResponse, err)
	}

	// Verify the credential
	credential, err := s.webAuthn.ValidateLogin(webAuthnUser, sessionData, parsedResponse)
	if err != nil {
		return nil, errors.Join(ErrCredentialValidation, err)
	}

	// Find the credential that was used
	credID := base64.StdEncoding.EncodeToString(credential.ID)
	usedCredential, ok := credentialMap[credID]
	if !ok {
		return nil, ErrCredentialNotFound
	}

	// Update credential sign count, backup state, and last used time
	usedCredential.SignCount = credential.Authenticator.SignCount
	usedCredential.CloneWarning = credential.Authenticator.CloneWarning
	usedCredential.BackupState = credential.Flags.BackupState
	usedCredential.LastUsedAt = time.Now()

	if err := s.repos.Account().UpdateWebAuthnLastUsed(ctx, usedCredential.ID, usedCredential.SignCount); err != nil {
		common.Logger().Error("failed to update credential", zap.Error(err))
	}

	// Delete the used challenge
	_ = s.repos.Account().DeleteWebAuthnChallenge(ctx, challenge)

	return usedCredential, nil
}

// GetUserCredentials returns all WebAuthn credentials for a user
func (s *WebAuthnService) GetUserCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	return s.repos.Account().GetUserWebAuthnCredentials(ctx, username)
}

// DeleteCredential removes a WebAuthn credential
func (s *WebAuthnService) DeleteCredential(ctx context.Context, username string, credentialID string) error {
	// Verify the credential belongs to the user
	credential, err := s.repos.Account().GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	if credential.UserID != username {
		return ErrCredentialNotFound
	}

	// Make sure user has at least one other auth method
	credentials, err := s.repos.Account().GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		return err
	}

	if len(credentials) <= 1 {
		// Check if user has a password set
		user, err := s.repos.Account().GetUser(ctx, username)
		if err != nil {
			return err
		}

		if err := common.ValidateRequiredParam("user.PasswordHash", user.PasswordHash); err != nil {
			return ErrLastAuthMethodDelete
		}
	}

	return s.repos.Account().DeleteWebAuthnCredential(ctx, credentialID)
}

// UpdateCredentialName updates the display name of a credential
func (s *WebAuthnService) UpdateCredentialName(ctx context.Context, username string, credentialID string, newName string) error {
	// Verify the credential belongs to the user
	credential, err := s.repos.Account().GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	if credential.UserID != username {
		return ErrCredentialNotFound
	}

	credential.Name = newName
	return s.repos.Account().UpdateWebAuthnLastUsed(ctx, credential.ID, credential.SignCount)
}

// webAuthnUser implements the webauthn.User interface
type webAuthnUser struct {
	id          string
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	return []byte(u.id)
}

func (u *webAuthnUser) WebAuthnName() string {
	return u.name
}

func (u *webAuthnUser) WebAuthnDisplayName() string {
	return u.displayName
}

func (u *webAuthnUser) WebAuthnIcon() string {
	return ""
}

func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

// ToWebAuthnCredential converts to the webauthn library credential type
func ToWebAuthnCredential(c *storage.WebAuthnCredential) *webauthn.Credential {
	credID, _ := base64.StdEncoding.DecodeString(c.ID)
	return &webauthn.Credential{
		ID:              credID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Flags: webauthn.CredentialFlags{
			UserPresent:    true, // Always true for WebAuthn
			UserVerified:   true, // We require user verification
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       c.AAGUID,
			SignCount:    c.SignCount,
			CloneWarning: c.CloneWarning,
		},
	}
}
