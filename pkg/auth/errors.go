package auth

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// Basic password length errors
	ErrPasswordTooShort = errors.PasswordTooShort(8)
	ErrPasswordTooLong  = errors.PasswordTooLong(72)

	// Length requirement error for policy validation
	ErrPasswordInsufficientLength = errors.PasswordInsufficientLength()

	// Password processing errors
	ErrPasswordHashFailed = errors.PasswordHashingFailed(nil)

	// Character requirement errors
	ErrPasswordMissingUppercase   = errors.PasswordMissingRequirement("uppercase letter")
	ErrPasswordMissingLowercase   = errors.PasswordMissingRequirement("lowercase letter")
	ErrPasswordMissingNumber      = errors.PasswordMissingRequirement("number")
	ErrPasswordMissingSpecialChar = errors.PasswordMissingRequirement("special character")

	// Content validation errors
	ErrPasswordContainsUsername  = errors.PasswordContainsUsername()
	ErrPasswordTooCommon         = errors.PasswordTooCommon()
	ErrPasswordSequentialPattern = errors.PasswordSequentialPattern()
	ErrPasswordRepeatedPattern   = errors.PasswordRepeatedPattern()

	// Session security errors
	ErrCSRFTokenGeneration     = errors.CSRFTokenGenerationFailed(nil)
	ErrSessionIDGeneration     = errors.SessionIDGenerationFailed(nil)
	ErrCookieEntropyGeneration = errors.CookieEntropyGenerationFailed(nil)
	ErrCSRFTokenRotation       = errors.CSRFTokenRotationFailed(nil)
	ErrCSRFValidationFailed    = errors.CSRFValidationFailed()

	// OAuth token validation errors
	ErrUnexpectedSigningMethod = errors.UnexpectedSigningMethod()
	ErrSessionIDMismatch       = errors.SessionIDMismatch()
	ErrIPAddressMismatch       = errors.IPAddressMismatch()
	ErrTokenVersionMismatch    = errors.TokenVersionMismatch()
	ErrTokenTooOld             = errors.TokenTooOld()

	// Wallet authentication errors
	ErrNonceGeneration          = errors.NonceGenerationFailed(nil)
	ErrChallengeStorage         = errors.ChallengeStorageFailed(nil)
	ErrChallengeRetrieval       = errors.ChallengeRetrievalFailed(nil)
	ErrChallengeExpired         = errors.WalletChallengeExpired()
	ErrMessageMismatch          = errors.MessageMismatch()
	ErrAddressMismatch          = errors.WalletAddressMismatch()
	ErrSignatureVerification    = errors.SignatureVerificationFailed()
	ErrWalletCheck              = errors.WalletCheckFailed(nil)
	ErrWalletStorage            = errors.WalletStorageFailed(nil)
	ErrWalletRetrieval          = errors.WalletRetrievalFailed(nil)
	ErrWalletDeletion           = errors.WalletDeletionFailed(nil)
	ErrWalletAlreadyLinked      = errors.WalletAlreadyLinked()
	ErrInvalidSignatureFormat   = errors.InvalidSignatureFormat()
	ErrInvalidSignatureLength   = errors.InvalidSignatureLength()
	ErrPublicKeyRecovery        = errors.PublicKeyRecoveryFailed(nil)
	ErrSignatureAddressMismatch = errors.SignatureAddressMismatch()

	// Social recovery errors
	ErrTrusteeActorIDRequired    = errors.TrusteeActorIDRequired()
	ErrInsufficientTrustees      = errors.InsufficientTrustees()
	ErrRecoveryTokenGeneration   = errors.RecoveryTokenGenerationFailed(nil)
	ErrRecoveryRequestNotFound   = errors.RecoveryRequestNotFound()
	ErrRecoveryRequestNotPending = errors.RecoveryRequestNotPending()
	ErrRecoveryRequestExpired    = errors.RecoveryRequestExpired()
	ErrTrusteeAlreadyVoted       = errors.TrusteeAlreadyVoted()

	// Social recovery repository operation errors
	ErrTrusteeStorage           = errors.TrusteeStorageFailed(nil)
	ErrTrusteeDeletion          = errors.TrusteeDeletionFailed(nil)
	ErrTrusteeRetrieval         = errors.TrusteeRetrievalFailed(nil)
	ErrRecoveryRequestStorage   = errors.RecoveryRequestStorageFailed(nil)
	ErrRecoveryRequestRetrieval = errors.RecoveryRequestRetrievalFailed(nil)
	ErrRecoveryRequestUpdate    = errors.RecoveryRequestUpdateFailed(nil)
	ErrRecoveryTokenStorage     = errors.RecoveryTokenStorageFailed(nil)

	// WebAuthn service errors
	ErrWebAuthnServiceInit        = errors.WebAuthnServiceInitFailed(nil)
	ErrUserRetrieval              = errors.UserRetrievalFailed(nil)
	ErrCredentialRetrieval        = errors.CredentialRetrievalFailed(nil)
	ErrRegistrationBegin          = errors.RegistrationBeginFailed(nil)
	ErrLoginBegin                 = errors.LoginBeginFailed(nil)
	ErrSessionDataSerialization   = errors.SessionDataSerializationFailed(nil)
	ErrSessionDataDeserialization = errors.SessionDataDeserializationFailed(nil)
	ErrWebAuthnChallengeStorage   = errors.WebAuthnChallengeStorageFailed(nil)
	ErrCredentialResponse         = errors.CredentialResponseParseFailed(nil)
	ErrCredentialCreation         = errors.CredentialCreationFailed(nil)
	ErrCredentialValidation       = errors.CredentialValidationFailed(nil)
	ErrCredentialStorage          = errors.CredentialStorageFailed(nil)
	ErrMaxCredentialsReached      = errors.MaxCredentialsReached()
	ErrInvalidSessionDataType     = errors.InvalidSessionDataType()
	ErrLastAuthMethodDelete       = errors.LastAuthMethodDelete()

	// Recovery code errors
	ErrRecoveryCodeGeneration = errors.RecoveryCodeGenerationFailed(nil)
	ErrRecoveryCodeHashing    = errors.RecoveryCodeHashingFailed(nil)
	ErrRecoveryCodeStorage    = errors.RecoveryCodeStorageFailed(nil)
	ErrRecoveryCodeRetrieval  = errors.RecoveryCodeRetrievalFailed(nil)
	ErrRecoveryCodeMarkUsed   = errors.RecoveryCodeMarkUsedFailed(nil)
	ErrRecoveryCodeClear      = errors.RecoveryCodeClearFailed(nil)

	// Secrets Manager errors
	ErrAWSConfigLoad              = errors.AWSConfigLoadFailed(nil)
	ErrSecretsManagerConnection   = errors.SecretsManagerConnectionFailed(nil)
	ErrInvalidPrivateKeyFormat    = errors.InvalidPrivateKeyFormat()
	ErrSecretValueMarshal         = errors.SecretValueMarshalFailed(nil)
	ErrSecretCreation             = errors.SecretCreationFailed(nil)
	ErrPrivateKeyRetrieval        = errors.PrivateKeyRetrievalFailed(nil)
	ErrSecretValueUnmarshal       = errors.SecretValueUnmarshalFailed(nil)
	ErrRetrievedPrivateKeyInvalid = errors.RetrievedPrivateKeyInvalid()
	ErrSecretDeletion             = errors.SecretDeletionFailed(nil)
	ErrRSAKeyPairGeneration       = errors.RSAKeyPairGenerationFailed(nil)
	ErrPrivateKeyMarshal          = errors.PrivateKeyMarshalFailed(nil)
	ErrPublicKeyMarshal           = errors.PublicKeyMarshalFailed(nil)
	ErrGeneratedPrivateKeyStorage = errors.GeneratedPrivateKeyStorageFailed(nil)
	ErrKeyPairGenerationRotation  = errors.KeyPairGenerationRotationFailed(nil)
	ErrPEMBlockDecode             = errors.PEMBlockDecodeFailed()
	ErrPrivateKeyParse            = errors.PrivateKeyParseFailed(nil)
	ErrSecretValueNil             = errors.SecretValueNil()
	ErrSecretRetrievalRetries     = errors.SecretRetrievalRetriesFailed(nil)

	// Audit logging errors
	ErrAuditEventMarshal          = errors.AuditEventMarshalFailed(nil)
	ErrSIEMRequestCreation        = errors.SIEMRequestCreationFailed(nil)
	ErrSIEMTransmission           = errors.SIEMTransmissionFailed(nil)
	ErrSIEMResponseError          = errors.SIEMResponseError()
	ErrAuditRepositoryUnavailable = errors.AuditRepositoryUnavailable()

	// Rate limiting operation errors
	ErrIPRateLimitCheck       = errors.IPRateLimitCheckFailed(nil)
	ErrAccountRateLimitCheck  = errors.AccountRateLimitCheckFailed(nil)
	ErrRecordIPAttempt        = errors.RecordIPAttemptFailed(nil)
	ErrRecordAccountAttempt   = errors.RecordAccountAttemptFailed(nil)
	ErrGetIPAttemptCount      = errors.GetIPAttemptCountFailed(nil)
	ErrGetAccountAttemptCount = errors.GetAccountAttemptCountFailed(nil)
	ErrImposeIPLockout        = errors.ImposeIPLockoutFailed(nil)
	ErrImposeAccountLockout   = errors.ImposeAccountLockoutFailed(nil)

	// Session management errors
	ErrRefreshTokenGeneration    = errors.RefreshTokenGenerationFailed(nil)
	ErrDeviceIDRetrieval         = errors.DeviceIDRetrievalFailed(nil)
	ErrSessionStorage            = errors.SessionStorageFailed(nil)
	ErrNewRefreshTokenGeneration = errors.NewRefreshTokenGenerationFailed(nil)
	ErrSessionUpdate             = errors.SessionUpdateFailed(nil)
	ErrUserSessionsRetrieval     = errors.UserSessionsRetrievalFailed(nil)
	ErrOldestSessionRemoval      = errors.OldestSessionRemovalFailed(nil)
	ErrInvalidRefreshToken       = errors.RefreshTokenInvalid()
	ErrSessionNotFound           = errors.SessionNotFound("")
	ErrSessionExpired            = errors.SessionExpired()
	ErrDeviceNotFound            = errors.DeviceNotFound("")

	// Device fingerprinting errors
	ErrUserDevicesRetrieval = errors.UserDevicesRetrievalFailed(nil)
	ErrDeviceCreation       = errors.DeviceCreationFailed(nil)
	ErrMaxDevicesExceeded   = errors.MaxDevicesExceeded()

	// Device ownership errors
	ErrDeviceOwnershipMismatch = errors.DeviceOwnershipMismatch()

	// JWT validation errors
	ErrJWTUnexpectedSigningMethod = errors.JWTUnexpectedSigningMethod()

	// Recovery federation errors
	ErrInvalidActivityObject           = errors.InvalidActivityObject()
	ErrNotRecoveryConfirmationActivity = errors.NotRecoveryConfirmationActivity()
	ErrMissingRequestID                = errors.MissingRequestID()
	ErrFailedToDecodePEM               = errors.FailedToDecodePEM()
	ErrUnsupportedPrivateKeyType       = errors.UnsupportedPrivateKeyType()
	ErrSecretsManagerNotAvailable      = errors.SecretsManagerNotAvailable()
	ErrSigningActorRetrievalFailed     = errors.SigningActorRetrievalFailed(nil)
	ErrRecoveryConfirmationFailed      = errors.RecoveryConfirmationFailed(nil)
	ErrActorRetrievalFailed            = errors.ActorRetrievalFailed(nil)
	ErrPrivateKeyParseFailed           = errors.PrivateKeyParseFailed(nil)
	ErrPublicKeyMarshalFailed          = errors.PublicKeyMarshalFailed(nil)
	ErrSystemActorKeyRetrievalFailed   = errors.SystemActorKeyRetrievalFailed(nil)
	ErrSystemActorKeyRotationFailed    = errors.SystemActorKeyRotationFailed(nil)

	// Session lifecycle errors
	ErrSessionSecurityValidationFailed = errors.SessionSecurityValidationFailed("")
	ErrSessionCannotBeExtended         = errors.SessionCannotBeExtended("")
	ErrSessionMaxLifetimeReached       = errors.SessionMaxLifetimeReached()
	ErrSessionExtensionDisabled        = errors.SessionExtensionDisabled()
	ErrConcurrentSessionLimitExceeded  = errors.ConcurrentSessionLimitExceeded()
	ErrRefreshTokenRotationFailed      = errors.RefreshTokenRotationFailed(nil)
	ErrInvalidRefreshTokenProvided     = errors.InvalidRefreshTokenProvided()
	ErrSessionSecurityCheckFailed      = errors.SessionSecurityCheckFailed(nil)

	// Auth service operation errors
	ErrSessionCreationFailed         = errors.SessionCreationFailed(nil)
	ErrAccessTokenGenerationFailed   = errors.AccessTokenGenerationFailed(nil)
	ErrPasswordHashingFailed         = errors.PasswordHashingFailed(nil)
	ErrPasswordUpdateFailed          = errors.PasswordUpdateFailed(nil)
	ErrSignatureVerificationFailed   = errors.SignatureVerificationFailed()
	ErrUserRetrievalFailed           = errors.UserRetrievalFailed(nil)
	ErrRecoveryTokenGenerationFailed = errors.RecoveryTokenGenerationFailed(nil)
	ErrRecoveryTokenStorageFailed    = errors.RecoveryTokenStorageFailed(nil)

	// Common authentication errors (moved from service.go)
	ErrInvalidCredentials    = errors.InvalidCredentials()
	ErrUserNotFound          = errors.UserNotFound("")
	ErrUserSuspended         = errors.UserSuspended("")
	ErrUserNotApproved       = errors.UserNotApproved("")
	ErrInvalidToken          = errors.TokenInvalid("")
	ErrWebAuthnNotConfigured = errors.WebAuthnNotConfigured()
)
