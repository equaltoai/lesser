package auth

import "errors"

// Password validation errors
var (
	// Basic password length errors
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong  = errors.New("password must be less than 72 characters")

	// Length requirement error for policy validation
	ErrPasswordInsufficientLength = errors.New("password does not meet minimum length requirement")

	// Password processing errors
	ErrPasswordHashFailed = errors.New("password hashing failed")

	// Character requirement errors
	ErrPasswordMissingUppercase   = errors.New("password must contain at least one uppercase letter")
	ErrPasswordMissingLowercase   = errors.New("password must contain at least one lowercase letter")
	ErrPasswordMissingNumber      = errors.New("password must contain at least one number")
	ErrPasswordMissingSpecialChar = errors.New("password must contain at least one special character")

	// Content validation errors
	ErrPasswordContainsUsername  = errors.New("password cannot contain username")
	ErrPasswordTooCommon         = errors.New("password is too common, please choose a more unique password")
	ErrPasswordSequentialPattern = errors.New("password contains sequential characters, please choose a more complex password")
	ErrPasswordRepeatedPattern   = errors.New("password contains too many repeated characters")

	// Session security errors
	ErrCSRFTokenGeneration     = errors.New("failed to generate CSRF token")
	ErrSessionIDGeneration     = errors.New("failed to generate new session ID")
	ErrCookieEntropyGeneration = errors.New("failed to generate cookie entropy")
	ErrCSRFTokenRotation       = errors.New("failed to rotate CSRF token")
	ErrCSRFValidationFailed    = errors.New("CSRF validation failed")

	// OAuth token validation errors
	ErrUnexpectedSigningMethod = errors.New("unexpected signing method")
	ErrSessionIDMismatch       = errors.New("session ID mismatch")
	ErrIPAddressMismatch       = errors.New("IP address mismatch")
	ErrTokenVersionMismatch    = errors.New("token version mismatch")
	ErrTokenTooOld             = errors.New("token too old")

	// Wallet authentication errors
	ErrNonceGeneration           = errors.New("failed to generate nonce")
	ErrChallengeStorage          = errors.New("failed to store challenge")
	ErrChallengeRetrieval        = errors.New("failed to get challenge")
	ErrChallengeExpired          = errors.New("challenge expired")
	ErrMessageMismatch           = errors.New("message mismatch")
	ErrAddressMismatch           = errors.New("address mismatch")
	ErrSignatureVerification     = errors.New("signature verification failed")
	ErrWalletCheck               = errors.New("failed to check existing wallet")
	ErrWalletStorage             = errors.New("failed to store wallet")
	ErrWalletRetrieval           = errors.New("failed to get user wallets")
	ErrWalletDeletion            = errors.New("failed to delete wallet")
	ErrWalletAlreadyLinked       = errors.New("wallet already linked to another account")
	ErrInvalidSignatureFormat    = errors.New("invalid signature format")
	ErrInvalidSignatureLength    = errors.New("invalid signature length")
	ErrPublicKeyRecovery         = errors.New("failed to recover public key")
	ErrSignatureAddressMismatch  = errors.New("signature address mismatch")

	// Social recovery errors
	ErrTrusteeActorIDRequired     = errors.New("trustee actor ID required")
	ErrInsufficientTrustees      = errors.New("insufficient trustees configured (minimum 2 required)")
	ErrRecoveryTokenGeneration   = errors.New("failed to generate recovery token")
	ErrRecoveryRequestNotFound   = errors.New("recovery request not found")
	ErrRecoveryRequestNotPending = errors.New("recovery request is not pending")
	ErrRecoveryRequestExpired    = errors.New("recovery request expired")
	ErrTrusteeAlreadyVoted      = errors.New("trustee already voted")

	// Social recovery repository operation errors
	ErrTrusteeStorage             = errors.New("failed to store trustee")
	ErrTrusteeDeletion            = errors.New("failed to delete trustee")
	ErrTrusteeRetrieval           = errors.New("failed to get trustees")
	ErrRecoveryRequestStorage     = errors.New("failed to store recovery request")
	ErrRecoveryRequestRetrieval   = errors.New("failed to get recovery request")
	ErrRecoveryRequestUpdate      = errors.New("failed to update recovery request")
	ErrRecoveryTokenStorage       = errors.New("failed to enable recovery token")

	// WebAuthn service errors
	ErrWebAuthnServiceInit       = errors.New("failed to create WebAuthn instance")
	ErrUserRetrieval             = errors.New("failed to get user")
	ErrCredentialRetrieval       = errors.New("failed to get credentials")
	ErrRegistrationBegin         = errors.New("failed to begin registration")
	ErrLoginBegin                = errors.New("failed to begin login")
	ErrSessionDataSerialization  = errors.New("failed to serialize session data")
	ErrSessionDataDeserialization = errors.New("failed to deserialize session data")
	ErrWebAuthnChallengeStorage  = errors.New("failed to store challenge")
	ErrCredentialResponse        = errors.New("failed to parse credential response")
	ErrCredentialCreation        = errors.New("failed to create credential")
	ErrCredentialValidation      = errors.New("failed to validate login")
	ErrCredentialStorage         = errors.New("failed to store credential")
	ErrMaxCredentialsReached     = errors.New("maximum number of credentials reached")
	ErrInvalidSessionDataType    = errors.New("invalid session data type")
	ErrLastAuthMethodDelete      = errors.New("cannot delete last authentication method")

	// Recovery code errors
	ErrRecoveryCodeGeneration = errors.New("failed to generate recovery code")
	ErrRecoveryCodeHashing    = errors.New("failed to hash recovery code")
	ErrRecoveryCodeStorage    = errors.New("failed to store recovery code")
	ErrRecoveryCodeRetrieval  = errors.New("failed to get recovery codes")
	ErrRecoveryCodeMarkUsed   = errors.New("failed to mark recovery code as used")
	ErrRecoveryCodeClear      = errors.New("failed to clear existing recovery codes")

	// Secrets Manager errors
	ErrAWSConfigLoad               = errors.New("failed to load AWS config")
	ErrSecretsManagerConnection    = errors.New("failed to connect to AWS Secrets Manager")
	ErrInvalidPrivateKeyFormat     = errors.New("invalid private key format")
	ErrSecretValueMarshal          = errors.New("failed to marshal secret value")
	ErrSecretCreation              = errors.New("failed to create secret")
	ErrPrivateKeyRetrieval         = errors.New("failed to retrieve private key")
	ErrSecretValueUnmarshal        = errors.New("failed to unmarshal secret value")
	ErrRetrievedPrivateKeyInvalid  = errors.New("retrieved private key is invalid")
	ErrSecretDeletion              = errors.New("failed to delete secret")
	ErrRSAKeyPairGeneration        = errors.New("failed to generate RSA key pair")
	ErrPrivateKeyMarshal           = errors.New("failed to marshal private key")
	ErrPublicKeyMarshal            = errors.New("failed to marshal public key")
	ErrGeneratedPrivateKeyStorage  = errors.New("failed to store generated private key")
	ErrKeyPairGenerationRotation   = errors.New("failed to generate new key pair during rotation")
	ErrPEMBlockDecode              = errors.New("failed to decode PEM block")
	ErrPrivateKeyParse             = errors.New("failed to parse private key")
	ErrSecretValueNil              = errors.New("secret value is nil")
	ErrSecretRetrievalRetries      = errors.New("failed to get secret after retries")

	// Audit logging errors
	ErrAuditEventMarshal          = errors.New("failed to marshal audit event")
	ErrSIEMRequestCreation        = errors.New("failed to create SIEM request")
	ErrSIEMTransmission           = errors.New("failed to send to SIEM")
	ErrSIEMResponseError          = errors.New("SIEM returned error status")
	ErrAuditRepositoryUnavailable = errors.New("audit repository not available")

	// Rate limiting operation errors
	ErrIPRateLimitCheck      = errors.New("failed to check IP rate limit")
	ErrAccountRateLimitCheck = errors.New("failed to check account rate limit")
	ErrRecordIPAttempt       = errors.New("failed to record IP attempt")
	ErrRecordAccountAttempt  = errors.New("failed to record account attempt")
	ErrGetIPAttemptCount     = errors.New("failed to get IP attempt count")
	ErrGetAccountAttemptCount = errors.New("failed to get account attempt count")
	ErrImposeIPLockout       = errors.New("failed to impose IP lockout")
	ErrImposeAccountLockout  = errors.New("failed to impose account lockout")

	// Session management errors
	ErrRefreshTokenGeneration     = errors.New("failed to generate refresh token")
	ErrDeviceIDRetrieval          = errors.New("failed to get device ID")
	ErrSessionStorage             = errors.New("failed to store session")
	ErrNewRefreshTokenGeneration  = errors.New("failed to generate new refresh token")
	ErrSessionUpdate              = errors.New("failed to update session")
	ErrUserSessionsRetrieval      = errors.New("failed to get user sessions")
	ErrOldestSessionRemoval       = errors.New("failed to remove oldest session")
	ErrInvalidRefreshToken        = errors.New("invalid refresh token")
	ErrSessionNotFound            = errors.New("session not found")
	ErrSessionExpired             = errors.New("session expired")
	ErrDeviceNotFound             = errors.New("device not found")

	// Device fingerprinting errors
	ErrUserDevicesRetrieval = errors.New("failed to get user devices")
	ErrDeviceCreation       = errors.New("failed to create device")
	ErrMaxDevicesExceeded   = errors.New("maximum number of devices exceeded")

	// Device ownership errors
	ErrDeviceOwnershipMismatch = errors.New("device does not belong to user")

	// JWT validation errors
	ErrJWTUnexpectedSigningMethod = errors.New("unexpected signing method")

	// Recovery federation errors
	ErrInvalidActivityObject          = errors.New("invalid activity object")
	ErrNotRecoveryConfirmationActivity = errors.New("not a recovery confirmation activity")
	ErrMissingRequestID               = errors.New("missing request ID")
	ErrFailedToDecodePEM              = errors.New("failed to decode private key PEM")
	ErrUnsupportedPrivateKeyType      = errors.New("unsupported private key type")
	ErrSecretsManagerNotAvailable     = errors.New("secrets manager not available")
	ErrSigningActorRetrievalFailed    = errors.New("failed to get signing actor")
	ErrRecoveryConfirmationFailed     = errors.New("failed to process recovery confirmation")
	ErrActorRetrievalFailed           = errors.New("failed to get actor")
	ErrPrivateKeyParseFailed          = errors.New("failed to parse private key")
	ErrPublicKeyMarshalFailed         = errors.New("failed to marshal public key")
	ErrSystemActorKeyRetrievalFailed  = errors.New("failed to retrieve system actor private key")
	ErrSystemActorKeyRotationFailed   = errors.New("failed to rotate system actor key")

	// Session lifecycle errors
	ErrSessionSecurityValidationFailed = errors.New("session security validation failed")
	ErrSessionCannotBeExtended         = errors.New("session cannot be extended")
	ErrSessionMaxLifetimeReached       = errors.New("session cannot be extended (max lifetime reached)")
	ErrSessionExtensionDisabled        = errors.New("session extension is disabled")
	ErrConcurrentSessionLimitExceeded  = errors.New("concurrent session limit exceeded")
	ErrRefreshTokenRotationFailed      = errors.New("failed to rotate refresh token")
	ErrInvalidRefreshTokenProvided     = errors.New("invalid refresh token provided")
	ErrSessionSecurityCheckFailed      = errors.New("session security check failed")

	// Auth service operation errors
	ErrSessionCreationFailed       = errors.New("failed to create session")
	ErrAccessTokenGenerationFailed = errors.New("failed to generate access token")
	ErrPasswordHashingFailed       = errors.New("failed to hash password")
	ErrPasswordUpdateFailed        = errors.New("failed to update password")
	ErrSignatureVerificationFailed = errors.New("signature verification failed")
	ErrUserRetrievalFailed         = errors.New("failed to get user")
	ErrRecoveryTokenGenerationFailed = errors.New("failed to generate token")
	ErrRecoveryTokenStorageFailed    = errors.New("failed to store recovery token")

	// Common authentication errors (moved from service.go)
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserSuspended      = errors.New("user account is suspended")
	ErrUserNotApproved    = errors.New("user account is not approved")
	ErrInvalidToken       = errors.New("invalid token")
	ErrWebAuthnNotConfigured = errors.New("WebAuthn is not configured")
)
