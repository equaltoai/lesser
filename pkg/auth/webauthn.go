package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	accountservice "github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
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

	webAuthnChallengeTypeRegistration   = "registration"
	webAuthnChallengeTypeAuthentication = "authentication"
	webAuthnChallengeTypeSignup         = "signup"
)

// WebAuthnService handles WebAuthn operations
type WebAuthnService struct {
	webAuthn    webAuthnEngine
	repo        webAuthnRepository
	domain      string
	auditLogger *AuditLogger

	parseCreationResponse  func([]byte) (*protocol.ParsedCredentialCreationData, error)
	parseAssertionResponse func([]byte) (*protocol.ParsedCredentialAssertionData, error)
}

type webAuthnEngine interface {
	BeginRegistration(user webauthn.User, opts ...webauthn.RegistrationOption) (creation *protocol.CredentialCreation, session *webauthn.SessionData, err error)
	CreateCredential(user webauthn.User, session webauthn.SessionData, parsedResponse *protocol.ParsedCredentialCreationData) (credential *webauthn.Credential, err error)
	BeginLogin(user webauthn.User, opts ...webauthn.LoginOption) (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	ValidateLogin(user webauthn.User, session webauthn.SessionData, parsedResponse *protocol.ParsedCredentialAssertionData) (credential *webauthn.Credential, err error)
}

type webAuthnRepository interface {
	GetUser(ctx context.Context, username string) (*storage.User, error)
	GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error)
	GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error)
	StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error
	GetWebAuthnChallenge(ctx context.Context, challenge string) (*storage.WebAuthnChallenge, error)
	DeleteWebAuthnChallenge(ctx context.Context, challenge string) error
	StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error
	GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error)
	DeleteWebAuthnCredential(ctx context.Context, credentialID string) error
	DeleteWebAuthnCredentialConditionedOnSurvivor(ctx context.Context, username string, credentialID string, survivingPasskeyID string, survivingWalletAddress string) error
	UpdateWebAuthnCredentialName(ctx context.Context, credentialID string, name string) error
	UpdateWebAuthnAuthenticationState(ctx context.Context, credentialID string, signCount uint32, cloneWarning bool, backupState bool, lastUsedAt time.Time) error
	StorePasskeyRegistrationProof(ctx context.Context, proof *storagemodels.PasskeyRegistrationProof) error
	GetPasskeyRegistrationProof(ctx context.Context, proofID string) (*storagemodels.PasskeyRegistrationProof, error)
}

// NewWebAuthnService creates a new WebAuthn service
func NewWebAuthnService(repos StorageProvider, domain string, displayName string) (*WebAuthnService, error) {
	if repos == nil || repos.Account() == nil {
		return nil, ErrWebAuthnServiceInit
	}

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
		webAuthn:    webAuthn,
		repo:        repos.Account(),
		domain:      domain,
		auditLogger: NewAuditLogger(repos, common.Logger(), DefaultAuditConfig()),
		parseCreationResponse: func(data []byte) (*protocol.ParsedCredentialCreationData, error) {
			return protocol.ParseCredentialCreationResponseBytes(data)
		},
		parseAssertionResponse: func(data []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return protocol.ParseCredentialRequestResponseBytes(data)
		},
	}, nil
}

// BeginRegistration starts the WebAuthn registration process
func (s *WebAuthnService) BeginRegistration(ctx context.Context, username string) (any, string, error) {
	// Get user from storage
	user, err := s.repo.GetUser(ctx, username)
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
	credentials, err := s.repo.GetUserWebAuthnCredentials(ctx, username)
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
		Type:        webAuthnChallengeTypeRegistration,
	}

	if err := s.repo.StoreWebAuthnChallenge(ctx, challengeData); err != nil {
		return nil, "", errors.Join(ErrWebAuthnChallengeStorage, err)
	}

	return options, challengeData.Challenge, nil
}

// BeginSignup starts the public WebAuthn signup process for a not-yet-created account.
func (s *WebAuthnService) BeginSignup(ctx context.Context, username string) (any, string, error) {
	username = accountservice.NormalizeUsernameForDomain(username, s.domain)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, "", err
	}

	webAuthnUser := &webAuthnUser{
		id:          username,
		name:        username,
		displayName: username,
		credentials: []webauthn.Credential{},
	}

	options, sessionData, err := s.webAuthn.BeginRegistration(webAuthnUser, signupRegistrationOptions()...)
	if err != nil {
		return nil, "", errors.Join(ErrRegistrationBegin, err)
	}

	sessionDataBytes, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", errors.Join(ErrSessionDataSerialization, err)
	}

	challengeData := &storage.WebAuthnChallenge{
		Challenge:   sessionData.Challenge,
		UserID:      username,
		SessionData: sessionDataBytes,
		ExpiresAt:   time.Now().Add(ChallengeDuration),
		Type:        webAuthnChallengeTypeSignup,
	}

	if err := s.repo.StoreWebAuthnChallenge(ctx, challengeData); err != nil {
		return nil, "", errors.Join(ErrWebAuthnChallengeStorage, err)
	}

	return options, challengeData.Challenge, nil
}

// FinishRegistration completes the WebAuthn registration process
func (s *WebAuthnService) FinishRegistration(ctx context.Context, username string, challenge string, response []byte, credentialName string) error {
	// Get challenge data
	challengeData, err := s.repo.GetWebAuthnChallenge(ctx, challenge)
	if err != nil {
		return ErrChallengeNotFound
	}

	// Verify challenge belongs to user and hasn't expired
	if challengeData.UserID != username || challengeData.Type != webAuthnChallengeTypeRegistration {
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
	user, err := s.repo.GetUser(ctx, username)
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
	parsedResponse, err := s.parseCreationResponse(response)
	if err != nil {
		return errors.Join(ErrCredentialResponse, err)
	}

	// Verify the credential
	credential, err := s.webAuthn.CreateCredential(webAuthnUser, sessionData, parsedResponse)
	if err != nil {
		return errors.Join(ErrCredentialCreation, err)
	}

	// Check if user has too many credentials
	existingCreds, _ := s.repo.GetUserWebAuthnCredentials(ctx, username)
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

	if err := s.repo.StoreWebAuthnCredential(ctx, storedCredential); err != nil {
		return errors.Join(ErrCredentialStorage, err)
	}
	if s.auditLogger != nil {
		s.auditLogger.LogAuthEvent(ctx, username, "", "", AuditWebAuthnRegistrationCompleted, map[string]interface{}{
			"authentication_method": "webauthn",
			"credential_event":      "added",
			"registration_mode":     "authenticated",
		}, true, nil)
	}

	// Delete the used challenge
	_ = s.repo.DeleteWebAuthnChallenge(ctx, challenge)

	return nil
}

// FinishSignup completes the public WebAuthn signup process and produces a single-use registration proof.
func (s *WebAuthnService) FinishSignup(ctx context.Context, username string, challenge string, response []byte) (string, error) {
	username = accountservice.NormalizeUsernameForDomain(username, s.domain)
	challenge = strings.TrimSpace(challenge)

	challengeData, err := s.repo.GetWebAuthnChallenge(ctx, challenge)
	if err != nil {
		return "", ErrChallengeNotFound
	}

	if challengeData.UserID != username || challengeData.Type != webAuthnChallengeTypeSignup {
		return "", ErrChallengeNotFound
	}
	if time.Now().After(challengeData.ExpiresAt) {
		return "", ErrChallengeNotFound
	}

	// Deserialize session data
	var sessionData webauthn.SessionData
	sessionBytes, ok := challengeData.SessionData.([]byte)
	if !ok {
		// Try to handle it as a string (base64 encoded)
		if sessionStr, ok := challengeData.SessionData.(string); ok {
			sessionBytes = []byte(sessionStr)
		} else {
			return "", ErrInvalidSessionDataType
		}
	}
	if err := json.Unmarshal(sessionBytes, &sessionData); err != nil {
		return "", errors.Join(ErrSessionDataDeserialization, err)
	}

	webAuthnUser := &webAuthnUser{
		id:          username,
		name:        username,
		displayName: username,
		credentials: []webauthn.Credential{},
	}

	parsedResponse, err := s.parseCreationResponse(response)
	if err != nil {
		return "", errors.Join(ErrCredentialResponse, err)
	}

	credential, err := s.webAuthn.CreateCredential(webAuthnUser, sessionData, parsedResponse)
	if err != nil {
		return "", errors.Join(ErrCredentialCreation, err)
	}

	proof := &storagemodels.PasskeyRegistrationProof{
		ID:              uuid.NewString(),
		Username:        username,
		CeremonyID:      challenge,
		CredentialID:    base64.StdEncoding.EncodeToString(credential.ID),
		PublicKey:       append([]byte(nil), credential.PublicKey...),
		AttestationType: credential.AttestationType,
		AAGUID:          append([]byte(nil), credential.Authenticator.AAGUID...),
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
	}

	if err := s.repo.StorePasskeyRegistrationProof(ctx, proof); err != nil {
		return "", errors.Join(ErrCredentialStorage, err)
	}

	_ = s.repo.DeleteWebAuthnChallenge(ctx, challenge)

	return proof.ID, nil
}

// BeginLogin starts the WebAuthn login process
func (s *WebAuthnService) BeginLogin(ctx context.Context, username string) (any, string, error) {
	// Get user credentials
	credentials, err := s.repo.GetUserWebAuthnCredentials(ctx, username)
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
		Type:        webAuthnChallengeTypeAuthentication,
	}

	if err := s.repo.StoreWebAuthnChallenge(ctx, challengeData); err != nil {
		return nil, "", errors.Join(ErrWebAuthnChallengeStorage, err)
	}

	return options, challengeData.Challenge, nil
}

// FinishLogin completes the WebAuthn login process
func (s *WebAuthnService) FinishLogin(ctx context.Context, username string, challenge string, response []byte) (*storage.WebAuthnCredential, error) {
	// Get challenge data
	challengeData, err := s.repo.GetWebAuthnChallenge(ctx, challenge)
	if err != nil {
		return nil, ErrChallengeNotFound
	}

	// Verify challenge belongs to user and hasn't expired
	if challengeData.UserID != username || challengeData.Type != webAuthnChallengeTypeAuthentication {
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
	credentials, err := s.repo.GetUserWebAuthnCredentials(ctx, username)
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
	parsedResponse, err := s.parseAssertionResponse(response)
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

	if err := s.repo.UpdateWebAuthnAuthenticationState(
		ctx,
		usedCredential.ID,
		usedCredential.SignCount,
		usedCredential.CloneWarning,
		usedCredential.BackupState,
		usedCredential.LastUsedAt,
	); err != nil {
		common.Logger().Error("failed to update credential", zap.Error(err))
	}

	// Delete the used challenge
	_ = s.repo.DeleteWebAuthnChallenge(ctx, challenge)

	return usedCredential, nil
}

// GetUserCredentials returns all WebAuthn credentials for a user
func (s *WebAuthnService) GetUserCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	return s.repo.GetUserWebAuthnCredentials(ctx, username)
}

// DeleteCredential removes a WebAuthn credential
func (s *WebAuthnService) DeleteCredential(ctx context.Context, username string, credentialID string) error {
	// Verify the credential belongs to the user
	credential, err := s.repo.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	if credential.UserID != username {
		return ErrCredentialNotFound
	}

	plan, err := planAuthenticatorRemoval(ctx, s.repo, username, authenticatorRemovalPasskey, credential.ID, "")
	if err != nil {
		return err
	}

	if err := s.repo.DeleteWebAuthnCredentialConditionedOnSurvivor(
		ctx,
		username,
		credentialID,
		plan.survivingPasskeyID,
		plan.survivingWalletAddress,
	); err != nil {
		return classifyGuardedWebAuthnRemovalFailure(ctx, s.repo, username, credentialID, err)
	}
	if s.auditLogger != nil {
		s.auditLogger.LogAuthEvent(ctx, username, "", "", AuditWebAuthnCredentialRemoved, map[string]interface{}{
			"authentication_method": "webauthn",
			"credential_event":      "removed",
		}, true, nil)
	}

	return nil
}

// UpdateCredentialName updates the display name of a credential
func (s *WebAuthnService) UpdateCredentialName(ctx context.Context, username string, credentialID string, newName string) error {
	// Verify the credential belongs to the user
	credential, err := s.repo.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	if credential.UserID != username {
		return ErrCredentialNotFound
	}

	return s.repo.UpdateWebAuthnCredentialName(ctx, credential.ID, newName)
}

// GetPasskeyRegistrationProof retrieves a stored single-use signup proof by ID.
func (s *WebAuthnService) GetPasskeyRegistrationProof(ctx context.Context, proofID string) (*storagemodels.PasskeyRegistrationProof, error) {
	return s.repo.GetPasskeyRegistrationProof(ctx, strings.TrimSpace(proofID))
}

func signupRegistrationOptions() []webauthn.RegistrationOption {
	return []webauthn.RegistrationOption{
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		}),
	}
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
