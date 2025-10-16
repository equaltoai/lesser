package auth

import (
	"errors"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
)

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// Basic password length errors
	ErrPasswordTooShort = apperrors.PasswordTooShort(8)
	ErrPasswordTooLong  = apperrors.PasswordTooLong(72)

	// Length requirement error for policy validation
	ErrPasswordInsufficientLength = apperrors.PasswordInsufficientLength()

	// Password processing errors
	ErrPasswordHashFailed = apperrors.PasswordHashingFailed(errors.New("password hashing failed"))

	// Character requirement errors
	ErrPasswordMissingUppercase   = apperrors.PasswordMissingRequirement("uppercase letter")
	ErrPasswordMissingLowercase   = apperrors.PasswordMissingRequirement("lowercase letter")
	ErrPasswordMissingNumber      = apperrors.PasswordMissingRequirement("number")
	ErrPasswordMissingSpecialChar = apperrors.PasswordMissingRequirement("special character")

	// Content validation errors
	ErrPasswordContainsUsername  = apperrors.PasswordContainsUsername()
	ErrPasswordTooCommon         = apperrors.PasswordTooCommon()
	ErrPasswordSequentialPattern = apperrors.PasswordSequentialPattern()
	ErrPasswordRepeatedPattern   = apperrors.PasswordRepeatedPattern()

	// Session security errors
	ErrCSRFTokenGeneration     = apperrors.CSRFTokenGenerationFailed(errors.New("CSRF token generation failed"))
	ErrSessionIDGeneration     = apperrors.SessionIDGenerationFailed(errors.New("session ID generation failed"))
	ErrCookieEntropyGeneration = apperrors.CookieEntropyGenerationFailed(errors.New("cookie entropy generation failed"))
	ErrCSRFTokenRotation       = apperrors.CSRFTokenRotationFailed(errors.New("CSRF token rotation failed"))
	ErrCSRFValidationFailed    = apperrors.CSRFValidationFailed()

	// OAuth token validation errors
	ErrUnexpectedSigningMethod = apperrors.UnexpectedSigningMethod()
	ErrSessionIDMismatch       = apperrors.SessionIDMismatch()
	ErrIPAddressMismatch       = apperrors.IPAddressMismatch()
	ErrTokenVersionMismatch    = apperrors.TokenVersionMismatch()
	ErrTokenTooOld             = apperrors.TokenTooOld()

	// Wallet authentication errors
	ErrNonceGeneration          = apperrors.NonceGenerationFailed(errors.New("nonce generation failed"))
	ErrChallengeStorage         = apperrors.ChallengeStorageFailed(errors.New("challenge storage failed"))
	ErrChallengeRetrieval       = apperrors.ChallengeRetrievalFailed(errors.New("challenge retrieval failed"))
	ErrChallengeExpired         = apperrors.WalletChallengeExpired()
	ErrMessageMismatch          = apperrors.MessageMismatch()
	ErrAddressMismatch          = apperrors.WalletAddressMismatch()
	ErrSignatureVerification    = apperrors.SignatureVerificationFailed()
	ErrWalletCheck              = apperrors.WalletCheckFailed(errors.New("wallet check failed"))
	ErrWalletStorage            = apperrors.WalletStorageFailed(errors.New("wallet storage failed"))
	ErrWalletRetrieval          = apperrors.WalletRetrievalFailed(errors.New("wallet retrieval failed"))
	ErrWalletDeletion           = apperrors.WalletDeletionFailed(errors.New("wallet deletion failed"))
	ErrWalletAlreadyLinked      = apperrors.WalletAlreadyLinked()
	ErrInvalidSignatureFormat   = apperrors.InvalidSignatureFormat()
	ErrInvalidSignatureLength   = apperrors.InvalidSignatureLength()
	ErrPublicKeyRecovery        = apperrors.PublicKeyRecoveryFailed(errors.New("public key recovery failed"))
	ErrSignatureAddressMismatch = apperrors.SignatureAddressMismatch()

	// Social recovery errors
	ErrTrusteeActorIDRequired    = apperrors.TrusteeActorIDRequired()
	ErrInsufficientTrustees      = apperrors.InsufficientTrustees()
	ErrRecoveryTokenGeneration   = apperrors.RecoveryTokenGenerationFailed(errors.New("recovery token generation failed"))
	ErrRecoveryRequestNotFound   = apperrors.RecoveryRequestNotFound()
	ErrRecoveryRequestNotPending = apperrors.RecoveryRequestNotPending()
	ErrRecoveryRequestExpired    = apperrors.RecoveryRequestExpired()
	ErrTrusteeAlreadyVoted       = apperrors.TrusteeAlreadyVoted()

	// Social recovery repository operation errors
	ErrTrusteeStorage           = apperrors.TrusteeStorageFailed(errors.New("trustee storage failed"))
	ErrTrusteeDeletion          = apperrors.TrusteeDeletionFailed(errors.New("trustee deletion failed"))
	ErrTrusteeRetrieval         = apperrors.TrusteeRetrievalFailed(errors.New("trustee retrieval failed"))
	ErrRecoveryRequestStorage   = apperrors.RecoveryRequestStorageFailed(errors.New("recovery request storage failed"))
	ErrRecoveryRequestRetrieval = apperrors.RecoveryRequestRetrievalFailed(errors.New("recovery request retrieval failed"))
	ErrRecoveryRequestUpdate    = apperrors.RecoveryRequestUpdateFailed(errors.New("recovery request update failed"))
	ErrRecoveryTokenStorage     = apperrors.RecoveryTokenStorageFailed(errors.New("recovery token storage failed"))

	// WebAuthn service errors
	ErrWebAuthnServiceInit        = apperrors.WebAuthnServiceInitFailed(errors.New("WebAuthn service init failed"))
	ErrUserRetrieval              = apperrors.UserRetrievalFailed(errors.New("user retrieval failed"))
	ErrCredentialRetrieval        = apperrors.CredentialRetrievalFailed(errors.New("credential retrieval failed"))
	ErrRegistrationBegin          = apperrors.RegistrationBeginFailed(errors.New("registration begin failed"))
	ErrLoginBegin                 = apperrors.LoginBeginFailed(errors.New("login begin failed"))
	ErrSessionDataSerialization   = apperrors.SessionDataSerializationFailed(errors.New("session data serialization failed"))
	ErrSessionDataDeserialization = apperrors.SessionDataDeserializationFailed(errors.New("session data deserialization failed"))
	ErrWebAuthnChallengeStorage   = apperrors.WebAuthnChallengeStorageFailed(errors.New("WebAuthn challenge storage failed"))
	ErrCredentialResponse         = apperrors.CredentialResponseParseFailed(errors.New("credential response parse failed"))
	ErrCredentialCreation         = apperrors.CredentialCreationFailed(errors.New("credential creation failed"))
	ErrCredentialValidation       = apperrors.CredentialValidationFailed(errors.New("credential validation failed"))
	ErrCredentialStorage          = apperrors.CredentialStorageFailed(errors.New("credential storage failed"))
	ErrMaxCredentialsReached      = apperrors.MaxCredentialsReached()
	ErrInvalidSessionDataType     = apperrors.InvalidSessionDataType()
	ErrLastAuthMethodDelete       = apperrors.LastAuthMethodDelete()

	// Recovery code errors
	ErrRecoveryCodeGeneration = apperrors.RecoveryCodeGenerationFailed(errors.New("recovery code generation failed"))
	ErrRecoveryCodeHashing    = apperrors.RecoveryCodeHashingFailed(errors.New("recovery code hashing failed"))
	ErrRecoveryCodeStorage    = apperrors.RecoveryCodeStorageFailed(errors.New("recovery code storage failed"))
	ErrRecoveryCodeRetrieval  = apperrors.RecoveryCodeRetrievalFailed(errors.New("recovery code retrieval failed"))
	ErrRecoveryCodeMarkUsed   = apperrors.RecoveryCodeMarkUsedFailed(errors.New("recovery code mark used failed"))
	ErrRecoveryCodeClear      = apperrors.RecoveryCodeClearFailed(errors.New("recovery code clear failed"))

	// Secrets Manager errors
	ErrAWSConfigLoad              = apperrors.AWSConfigLoadFailed(errors.New("AWS config load failed"))
	ErrSecretsManagerConnection   = apperrors.SecretsManagerConnectionFailed(errors.New("secrets manager connection failed"))
	ErrInvalidPrivateKeyFormat    = apperrors.InvalidPrivateKeyFormat()
	ErrSecretValueMarshal         = apperrors.SecretValueMarshalFailed(errors.New("secret value marshal failed"))
	ErrSecretCreation             = apperrors.SecretCreationFailed(errors.New("secret creation failed"))
	ErrPrivateKeyRetrieval        = apperrors.PrivateKeyRetrievalFailed(errors.New("private key retrieval failed"))
	ErrSecretValueUnmarshal       = apperrors.SecretValueUnmarshalFailed(errors.New("secret value unmarshal failed"))
	ErrRetrievedPrivateKeyInvalid = apperrors.RetrievedPrivateKeyInvalid()
	ErrSecretDeletion             = apperrors.SecretDeletionFailed(errors.New("secret deletion failed"))
	ErrRSAKeyPairGeneration       = apperrors.RSAKeyPairGenerationFailed(errors.New("RSA key pair generation failed"))
	ErrPrivateKeyMarshal          = apperrors.PrivateKeyMarshalFailed(errors.New("private key marshal failed"))
	ErrPublicKeyMarshal           = apperrors.PublicKeyMarshalFailed(errors.New("public key marshal failed"))
	ErrGeneratedPrivateKeyStorage = apperrors.GeneratedPrivateKeyStorageFailed(errors.New("generated private key storage failed"))
	ErrKeyPairGenerationRotation  = apperrors.KeyPairGenerationRotationFailed(errors.New("key pair generation rotation failed"))
	ErrPEMBlockDecode             = apperrors.PEMBlockDecodeFailed()
	ErrPrivateKeyParse            = apperrors.PrivateKeyParseFailed(errors.New("private key parse failed"))
	ErrSecretValueNil             = apperrors.SecretValueNil()
	ErrSecretRetrievalRetries     = apperrors.SecretRetrievalRetriesFailed(errors.New("secret retrieval retries failed"))

	// Audit logging errors
	ErrAuditEventMarshal          = apperrors.AuditEventMarshalFailed(errors.New("audit event marshal failed"))
	ErrSIEMRequestCreation        = apperrors.SIEMRequestCreationFailed(errors.New("SIEM request creation failed"))
	ErrSIEMTransmission           = apperrors.SIEMTransmissionFailed(errors.New("SIEM transmission failed"))
	ErrSIEMResponseError          = apperrors.SIEMResponseError()
	ErrAuditRepositoryUnavailable = apperrors.AuditRepositoryUnavailable()

	// Rate limiting operation errors
	ErrIPRateLimitCheck       = apperrors.IPRateLimitCheckFailed(errors.New("IP rate limit check failed"))
	ErrAccountRateLimitCheck  = apperrors.AccountRateLimitCheckFailed(errors.New("account rate limit check failed"))
	ErrRecordIPAttempt        = apperrors.RecordIPAttemptFailed(errors.New("record IP attempt failed"))
	ErrRecordAccountAttempt   = apperrors.RecordAccountAttemptFailed(errors.New("record account attempt failed"))
	ErrGetIPAttemptCount      = apperrors.GetIPAttemptCountFailed(errors.New("get IP attempt count failed"))
	ErrGetAccountAttemptCount = apperrors.GetAccountAttemptCountFailed(errors.New("get account attempt count failed"))
	ErrImposeIPLockout        = apperrors.ImposeIPLockoutFailed(errors.New("impose IP lockout failed"))
	ErrImposeAccountLockout   = apperrors.ImposeAccountLockoutFailed(errors.New("impose account lockout failed"))

	// Session management errors
	ErrRefreshTokenGeneration    = apperrors.RefreshTokenGenerationFailed(errors.New("refresh token generation failed"))
	ErrDeviceIDRetrieval         = apperrors.DeviceIDRetrievalFailed(errors.New("device ID retrieval failed"))
	ErrSessionStorage            = apperrors.SessionStorageFailed(errors.New("session storage failed"))
	ErrNewRefreshTokenGeneration = apperrors.NewRefreshTokenGenerationFailed(errors.New("new refresh token generation failed"))
	ErrSessionUpdate             = apperrors.SessionUpdateFailed(errors.New("session update failed"))
	ErrUserSessionsRetrieval     = apperrors.UserSessionsRetrievalFailed(errors.New("user sessions retrieval failed"))
	ErrOldestSessionRemoval      = apperrors.OldestSessionRemovalFailed(errors.New("oldest session removal failed"))
	ErrInvalidRefreshToken       = apperrors.RefreshTokenInvalid()
	ErrSessionNotFound           = apperrors.SessionNotFound("")
	ErrSessionExpired            = apperrors.SessionExpired()
	ErrDeviceNotFound            = apperrors.DeviceNotFound("")

	// Device fingerprinting errors
	ErrUserDevicesRetrieval = apperrors.UserDevicesRetrievalFailed(errors.New("user devices retrieval failed"))
	ErrDeviceCreation       = apperrors.DeviceCreationFailed(errors.New("device creation failed"))
	ErrMaxDevicesExceeded   = apperrors.MaxDevicesExceeded()

	// Device ownership errors
	ErrDeviceOwnershipMismatch = apperrors.DeviceOwnershipMismatch()

	// JWT validation errors
	ErrJWTUnexpectedSigningMethod = apperrors.JWTUnexpectedSigningMethod()

	// Recovery federation errors
	ErrInvalidActivityObject           = apperrors.InvalidActivityObject()
	ErrNotRecoveryConfirmationActivity = apperrors.NotRecoveryConfirmationActivity()
	ErrMissingRequestID                = apperrors.MissingRequestID()
	ErrFailedToDecodePEM               = apperrors.FailedToDecodePEM()
	ErrUnsupportedPrivateKeyType       = apperrors.UnsupportedPrivateKeyType()
	ErrSecretsManagerNotAvailable      = apperrors.SecretsManagerNotAvailable()
	ErrSigningActorRetrievalFailed     = apperrors.SigningActorRetrievalFailed(errors.New("signing actor retrieval failed"))
	ErrRecoveryConfirmationFailed      = apperrors.RecoveryConfirmationFailed(errors.New("recovery confirmation failed"))
	ErrActorRetrievalFailed            = apperrors.ActorRetrievalFailed(errors.New("actor retrieval failed"))
	ErrPrivateKeyParseFailed           = apperrors.PrivateKeyParseFailed(errors.New("private key parse failed"))
	ErrPublicKeyMarshalFailed          = apperrors.PublicKeyMarshalFailed(errors.New("public key marshal failed"))
	ErrSystemActorKeyRetrievalFailed   = apperrors.SystemActorKeyRetrievalFailed(errors.New("system actor key retrieval failed"))
	ErrSystemActorKeyRotationFailed    = apperrors.SystemActorKeyRotationFailed(errors.New("system actor key rotation failed"))

	// Session lifecycle errors
	ErrSessionSecurityValidationFailed = apperrors.SessionSecurityValidationFailed("")
	ErrSessionCannotBeExtended         = apperrors.SessionCannotBeExtended("")
	ErrSessionMaxLifetimeReached       = apperrors.SessionMaxLifetimeReached()
	ErrSessionExtensionDisabled        = apperrors.SessionExtensionDisabled()
	ErrConcurrentSessionLimitExceeded  = apperrors.ConcurrentSessionLimitExceeded()
	ErrRefreshTokenRotationFailed      = apperrors.RefreshTokenRotationFailed(errors.New("refresh token rotation failed"))
	ErrInvalidRefreshTokenProvided     = apperrors.InvalidRefreshTokenProvided()
	ErrSessionSecurityCheckFailed      = apperrors.SessionSecurityCheckFailed(errors.New("session security check failed"))

	// Auth service operation errors
	ErrSessionCreationFailed         = apperrors.SessionCreationFailed(errors.New("session creation failed"))
	ErrAccessTokenGenerationFailed   = apperrors.AccessTokenGenerationFailed(errors.New("access token generation failed"))
	ErrPasswordHashingFailed         = apperrors.PasswordHashingFailed(errors.New("password hashing failed"))
	ErrPasswordUpdateFailed          = apperrors.PasswordUpdateFailed(errors.New("password update failed"))
	ErrSignatureVerificationFailed   = apperrors.SignatureVerificationFailed()
	ErrUserRetrievalFailed           = apperrors.UserRetrievalFailed(errors.New("user retrieval failed"))
	ErrRecoveryTokenGenerationFailed = apperrors.RecoveryTokenGenerationFailed(errors.New("recovery token generation failed"))
	ErrRecoveryTokenStorageFailed    = apperrors.RecoveryTokenStorageFailed(errors.New("recovery token storage failed"))

	// Common authentication errors (moved from service.go)
	ErrInvalidCredentials    = apperrors.InvalidCredentials()
	ErrUserNotFound          = apperrors.UserNotFound("")
	ErrUserSuspended         = apperrors.UserSuspended("")
	ErrUserNotApproved       = apperrors.UserNotApproved("")
	ErrInvalidToken          = apperrors.TokenInvalid("")
	ErrWebAuthnNotConfigured = apperrors.WebAuthnNotConfigured()
)
