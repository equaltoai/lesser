package errors

// Authentication and authorization domain errors
// Consolidates errors from pkg/auth/errors.go, pkg/common/errors.go (auth-related), and related files

// NewAuthError creates a new authentication error with the specified error code and message.
func NewAuthError(code ErrorCode, message string) *AppError {
	return NewAppError(code, CategoryAuth, message)
}

// NewAuthInternalError creates an authentication error with internal details wrapped from an underlying error.
func NewAuthInternalError(code ErrorCode, message string, internal error) *AppError {
	return WrapError(internal, code, CategoryAuth, message)
}

// AuthFailed creates an authentication failed error with the specified reason.
func AuthFailed(reason string) *AppError {
	return NewAuthError(CodeAuthFailed, "Authentication failed").
		WithMetadata("reason", reason)
}

// InvalidCredentials creates an error indicating invalid username or password.
func InvalidCredentials() *AppError {
	return NewAuthError(CodeAuthFailed, "Invalid username or password")
}

// UserNotFound creates an error indicating the specified user was not found.
func UserNotFound(username string) *AppError {
	return NewAuthError(CodeNotFound, "User not found").
		WithMetadata("username", username)
}

// UserSuspended creates an error indicating the user account is suspended.
func UserSuspended(username string) *AppError {
	return NewAuthError(CodeAccountSuspended, "User account is suspended").
		WithMetadata("username", username)
}

// UserNotApproved creates an error indicating the user account is not approved.
func UserNotApproved(username string) *AppError {
	return NewAuthError(CodeForbidden, "User account is not approved").
		WithMetadata("username", username)
}

// TokenExpired creates an error indicating an authentication token has expired.
func TokenExpired() *AppError {
	return NewAuthError(CodeTokenExpired, "Token has expired")
}

// TokenInvalid creates an error indicating an authentication token is invalid.
func TokenInvalid(reason string) *AppError {
	return NewAuthError(CodeTokenInvalid, "Invalid token").
		WithMetadata("reason", reason)
}

// TokenRevoked creates an error indicating an authentication token has been revoked.
func TokenRevoked() *AppError {
	return NewAuthError(CodeTokenRevoked, "Token has been revoked")
}

// TokenReuse creates an error indicating token reuse was detected, which is a potential security breach.
func TokenReuse() *AppError {
	return NewAuthError(CodeTokenReuse, "Token reuse detected - potential security breach")
}

// RefreshTokenInvalid creates an error indicating a refresh token is invalid.
func RefreshTokenInvalid() *AppError {
	return NewAuthError(CodeTokenInvalid, "Invalid refresh token")
}

// RefreshTokenExpired creates an error indicating a refresh token has expired.
func RefreshTokenExpired() *AppError {
	return NewAuthError(CodeTokenExpired, "Refresh token has expired")
}

// SessionNotFound creates an error indicating the specified session was not found.
func SessionNotFound(sessionID string) *AppError {
	return NewAuthError(CodeNotFound, "Session not found").
		WithMetadata("session_id", sessionID)
}

// SessionExpired creates an error indicating a user session has expired.
func SessionExpired() *AppError {
	return NewAuthError(CodeSessionExpired, "Session has expired")
}

// SessionSecurityValidationFailed creates an error indicating session security validation failed.
func SessionSecurityValidationFailed(reason string) *AppError {
	return NewAuthError(CodeAuthFailed, "Session security validation failed").
		WithMetadata("reason", reason)
}

// SessionCannotBeExtended creates an error indicating a session cannot be extended.
func SessionCannotBeExtended(reason string) *AppError {
	return NewAuthError(CodeForbidden, "Session cannot be extended").
		WithMetadata("reason", reason)
}

// ConcurrentSessionLimitExceeded creates an error indicating the maximum concurrent sessions limit has been exceeded.
func ConcurrentSessionLimitExceeded() *AppError {
	return NewAuthError(CodeQuotaExceeded, "Maximum concurrent sessions exceeded")
}

// InsufficientScope creates an error indicating insufficient permissions for the required scope.
func InsufficientScope(requiredScope string) *AppError {
	return NewAuthError(CodeInsufficientScope, "Insufficient permissions").
		WithMetadata("required_scope", requiredScope)
}

// AccessDenied creates an error indicating access to a resource is denied.
func AccessDenied(resource string) *AppError {
	return NewAuthError(CodeForbidden, "Access denied").
		WithMetadata("resource", resource)
}

// OperationNotAllowed creates an error indicating the specified operation is not allowed.
func OperationNotAllowed(operation string) *AppError {
	return NewAuthError(CodeForbidden, "Operation not allowed").
		WithMetadata("operation", operation)
}

// PasswordTooShort creates an error indicating a password is too short.
func PasswordTooShort(minLength int) *AppError {
	return NewAuthError(CodeFieldTooShort, "Password is too short").
		WithMetadata("min_length", minLength)
}

// PasswordTooLong creates an error indicating a password is too long.
func PasswordTooLong(maxLength int) *AppError {
	return NewAuthError(CodeFieldTooLong, "Password is too long").
		WithMetadata("max_length", maxLength)
}

// PasswordMissingRequirement creates an error indicating a password does not meet requirements.
func PasswordMissingRequirement(requirement string) *AppError {
	return NewAuthError(CodeValidationFailed, "Password does not meet requirements").
		WithMetadata("requirement", requirement)
}

// PasswordTooCommon creates an error indicating a password is too common.
func PasswordTooCommon() *AppError {
	return NewAuthError(CodeValidationFailed, "Password is too common")
}

// PasswordContainsUsername creates an error indicating a password contains the username.
func PasswordContainsUsername() *AppError {
	return NewAuthError(CodeValidationFailed, "Password cannot contain username")
}

// PasswordHashingFailed creates an error indicating password processing failed.
func PasswordHashingFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Password processing failed", err)
}

// WebAuthnNotConfigured creates an error indicating WebAuthn is not configured.
func WebAuthnNotConfigured() *AppError {
	return NewAuthError(CodeInternal, "WebAuthn is not configured")
}

// WebAuthnRegistrationFailed creates an error indicating WebAuthn registration failed.
func WebAuthnRegistrationFailed(reason string) *AppError {
	return NewAuthError(CodeValidationFailed, "WebAuthn registration failed").
		WithMetadata("reason", reason)
}

// WebAuthnLoginFailed creates an error indicating WebAuthn authentication failed.
func WebAuthnLoginFailed(reason string) *AppError {
	return NewAuthError(CodeAuthFailed, "WebAuthn authentication failed").
		WithMetadata("reason", reason)
}

// CredentialNotFound creates an error indicating an authentication credential was not found.
func CredentialNotFound() *AppError {
	return NewAuthError(CodeNotFound, "Authentication credential not found")
}

// MaxCredentialsReached creates an error indicating the maximum number of credentials has been reached.
func MaxCredentialsReached() *AppError {
	return NewAuthError(CodeQuotaExceeded, "Maximum number of credentials reached")
}

// LastAuthMethodDelete creates an error indicating the last authentication method cannot be deleted.
func LastAuthMethodDelete() *AppError {
	return NewAuthError(CodeOperationNotAllowed, "Cannot delete last authentication method")
}

// WalletChallengeExpired creates an error indicating a wallet authentication challenge has expired.
func WalletChallengeExpired() *AppError {
	return NewAuthError(CodeTokenExpired, "Wallet authentication challenge expired")
}

// WalletSignatureInvalid creates an error indicating a wallet signature is invalid.
func WalletSignatureInvalid(reason string) *AppError {
	return NewAuthError(CodeAuthFailed, "Invalid wallet signature").
		WithMetadata("reason", reason)
}

// WalletAlreadyLinked creates an error indicating a wallet is already linked to another account.
func WalletAlreadyLinked() *AppError {
	return NewAuthError(CodeConflict, "Wallet is already linked to another account")
}

// WalletAddressMismatch creates an error indicating a wallet address mismatch.
func WalletAddressMismatch() *AppError {
	return NewAuthError(CodeValidationFailed, "Wallet address mismatch")
}

// RateLimitExceeded creates an error indicating a rate limit has been exceeded.
func RateLimitExceeded(limitType string, resetTime int64) *AppError {
	return NewAuthError(CodeRateLimited, "Rate limit exceeded").
		WithMetadata("limit_type", limitType).
		WithMetadata("reset_time", resetTime)
}

// IPRateLimitExceeded creates an error indicating too many requests from an IP address.
func IPRateLimitExceeded(resetTime int64) *AppError {
	return NewAuthError(CodeRateLimited, "Too many requests from this IP address").
		WithMetadata("reset_time", resetTime)
}

// AccountRateLimitExceeded creates an error indicating too many failed attempts for an account.
func AccountRateLimitExceeded(resetTime int64) *AppError {
	return NewAuthError(CodeRateLimited, "Too many failed attempts for this account").
		WithMetadata("reset_time", resetTime)
}

// DeviceNotFound creates an error indicating a device was not found.
func DeviceNotFound(deviceID string) *AppError {
	return NewAuthError(CodeNotFound, "Device not found").
		WithMetadata("device_id", deviceID)
}

// MaxDevicesExceeded creates an error indicating the maximum number of devices has been exceeded.
func MaxDevicesExceeded() *AppError {
	return NewAuthError(CodeQuotaExceeded, "Maximum number of devices exceeded")
}

// DeviceOwnershipMismatch creates an error indicating a device does not belong to the user.
func DeviceOwnershipMismatch() *AppError {
	return NewAuthError(CodeForbidden, "Device does not belong to user")
}

// OAuthInvalidClient creates an error indicating an invalid OAuth client.
func OAuthInvalidClient() *AppError {
	return NewAuthError(CodeUnauthorized, "Invalid OAuth client")
}

// OAuthInvalidGrant creates an error indicating an invalid OAuth grant.
func OAuthInvalidGrant() *AppError {
	return NewAuthError(CodeAuthFailed, "Invalid OAuth grant")
}

// OAuthInvalidScope creates an error indicating an invalid OAuth scope.
func OAuthInvalidScope(scope string) *AppError {
	return NewAuthError(CodeInvalidInput, "Invalid OAuth scope").
		WithMetadata("scope", scope)
}

// OAuthUnsupportedGrantType creates an error indicating an unsupported OAuth grant type.
func OAuthUnsupportedGrantType(grantType string) *AppError {
	return NewAuthError(CodeBadRequest, "Unsupported grant type").
		WithMetadata("grant_type", grantType)
}

// RecoveryRequestNotFound creates an error indicating a recovery request was not found.
func RecoveryRequestNotFound() *AppError {
	return NewAuthError(CodeNotFound, "Recovery request not found")
}

// RecoveryRequestExpired creates an error indicating a recovery request has expired.
func RecoveryRequestExpired() *AppError {
	return NewAuthError(CodeTokenExpired, "Recovery request has expired")
}

// InsufficientTrustees creates an error indicating insufficient trustees are configured.
func InsufficientTrustees() *AppError {
	return NewAuthError(CodeValidationFailed, "Insufficient trustees configured")
}

// TrusteeAlreadyVoted creates an error indicating a trustee has already voted.
func TrusteeAlreadyVoted() *AppError {
	return NewAuthError(CodeConflict, "Trustee has already voted")
}

// RecoveryCodeInvalid creates an error indicating a recovery code is invalid.
func RecoveryCodeInvalid() *AppError {
	return NewAuthError(CodeAuthFailed, "Invalid recovery code")
}

// RecoveryCodeUsed creates an error indicating a recovery code has already been used.
func RecoveryCodeUsed() *AppError {
	return NewAuthError(CodeTokenReuse, "Recovery code has already been used")
}

// CSRFTokenMissing creates an error indicating a CSRF token is missing.
func CSRFTokenMissing() *AppError {
	return NewAuthError(CodeBadRequest, "CSRF token is missing")
}

// CSRFTokenInvalid creates an error indicating a CSRF token is invalid.
func CSRFTokenInvalid() *AppError {
	return NewAuthError(CodeForbidden, "Invalid CSRF token")
}

// AuthServiceUnavailable creates an error indicating the authentication service is unavailable.
func AuthServiceUnavailable(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Authentication service unavailable", err)
}

// SessionStorageFailed creates an error indicating session storage failed.
func SessionStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Session storage failed", err).AsRetryable()
}

// TokenGenerationFailed creates an error indicating token generation failed.
func TokenGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Token generation failed", err)
}

// CredentialStorageFailed creates an error indicating credential storage failed.
func CredentialStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Credential storage failed", err).AsRetryable()
}

// SecretsManagerError creates an error indicating a secrets manager error occurred.
func SecretsManagerError(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Secrets manager error", err).AsRetryable()
}

// AuditLoggingFailed creates an error indicating audit logging failed.
func AuditLoggingFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Audit logging failed", err).AsRetryable()
}

// Additional auth error functions for missing errors from pkg/auth/errors.go

// PasswordInsufficientLength creates an error indicating password does not meet minimum length requirement.
func PasswordInsufficientLength() *AppError {
	return NewAuthError(CodeFieldTooShort, "Password does not meet minimum length requirement")
}

// PasswordSequentialPattern creates an error indicating password contains sequential characters.
func PasswordSequentialPattern() *AppError {
	return NewAuthError(CodeValidationFailed, "Password contains sequential characters")
}

// PasswordRepeatedPattern creates an error indicating password contains too many repeated characters.
func PasswordRepeatedPattern() *AppError {
	return NewAuthError(CodeValidationFailed, "Password contains too many repeated characters")
}

// CSRFTokenGenerationFailed creates an error indicating CSRF token generation failed.
func CSRFTokenGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate CSRF token", err)
}

// SessionIDGenerationFailed creates an error indicating session ID generation failed.
func SessionIDGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate session ID", err)
}

// CookieEntropyGenerationFailed creates an error indicating cookie entropy generation failed.
func CookieEntropyGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate cookie entropy", err)
}

// CSRFTokenRotationFailed creates an error indicating CSRF token rotation failed.
func CSRFTokenRotationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to rotate CSRF token", err)
}

// CSRFValidationFailed creates an error indicating CSRF validation failed.
func CSRFValidationFailed() *AppError {
	return NewAuthError(CodeForbidden, "CSRF validation failed")
}

// UnexpectedSigningMethod creates an error indicating unexpected JWT signing method.
func UnexpectedSigningMethod() *AppError {
	return NewAuthError(CodeAuthFailed, "Unexpected signing method")
}

// SessionIDMismatch creates an error indicating session ID mismatch.
func SessionIDMismatch() *AppError {
	return NewAuthError(CodeAuthFailed, "Session ID mismatch")
}

// IPAddressMismatch creates an error indicating IP address mismatch.
func IPAddressMismatch() *AppError {
	return NewAuthError(CodeAuthFailed, "IP address mismatch")
}

// TokenVersionMismatch creates an error indicating token version mismatch.
func TokenVersionMismatch() *AppError {
	return NewAuthError(CodeAuthFailed, "Token version mismatch")
}

// TokenTooOld creates an error indicating token is too old.
func TokenTooOld() *AppError {
	return NewAuthError(CodeTokenExpired, "Token too old")
}

// NonceGenerationFailed creates an error indicating nonce generation failed.
func NonceGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate nonce", err)
}

// ChallengeStorageFailed creates an error indicating challenge storage failed.
func ChallengeStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to store challenge", err)
}

// ChallengeRetrievalFailed creates an error indicating challenge retrieval failed.
func ChallengeRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get challenge", err)
}

// MessageMismatch creates an error indicating message mismatch.
func MessageMismatch() *AppError {
	return NewAuthError(CodeValidationFailed, "Message mismatch")
}

// SignatureVerificationFailed creates an error indicating signature verification failed.
func SignatureVerificationFailed() *AppError {
	return NewAuthError(CodeAuthFailed, "Signature verification failed")
}

// WalletCheckFailed creates an error indicating wallet check failed.
func WalletCheckFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to check existing wallet", err)
}

// WalletStorageFailed creates an error indicating wallet storage failed.
func WalletStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to store wallet", err)
}

// WalletRetrievalFailed creates an error indicating wallet retrieval failed.
func WalletRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get user wallets", err)
}

// WalletDeletionFailed creates an error indicating wallet deletion failed.
func WalletDeletionFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to delete wallet", err)
}

// InvalidSignatureFormat creates an error indicating invalid signature format.
func InvalidSignatureFormat() *AppError {
	return NewAuthError(CodeValidationFailed, "Invalid signature format")
}

// InvalidSignatureLength creates an error indicating invalid signature length.
func InvalidSignatureLength() *AppError {
	return NewAuthError(CodeValidationFailed, "Invalid signature length")
}

// PublicKeyRecoveryFailed creates an error indicating public key recovery failed.
func PublicKeyRecoveryFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to recover public key", err)
}

// SignatureAddressMismatch creates an error indicating signature address mismatch.
func SignatureAddressMismatch() *AppError {
	return NewAuthError(CodeValidationFailed, "Signature address mismatch")
}

// TrusteeActorIDRequired creates an error indicating trustee actor ID is required.
func TrusteeActorIDRequired() *AppError {
	return NewAuthError(CodeRequiredFieldMissing, "Trustee actor ID required")
}

// RecoveryTokenGenerationFailed creates an error indicating recovery token generation failed.
func RecoveryTokenGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate recovery token", err)
}

// RecoveryRequestNotPending creates an error indicating recovery request is not pending.
func RecoveryRequestNotPending() *AppError {
	return NewAuthError(CodeInvalidStateTransition, "Recovery request is not pending")
}

// TrusteeStorageFailed creates an error indicating trustee storage failed.
func TrusteeStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to store trustee", err)
}

// TrusteeDeletionFailed creates an error indicating trustee deletion failed.
func TrusteeDeletionFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to delete trustee", err)
}

// TrusteeRetrievalFailed creates an error indicating trustee retrieval failed.
func TrusteeRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get trustees", err)
}

// RecoveryRequestStorageFailed creates an error indicating recovery request storage failed.
func RecoveryRequestStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to store recovery request", err)
}

// RecoveryRequestRetrievalFailed creates an error indicating recovery request retrieval failed.
func RecoveryRequestRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get recovery request", err)
}

// RecoveryRequestUpdateFailed creates an error indicating recovery request update failed.
func RecoveryRequestUpdateFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to update recovery request", err)
}

// RecoveryTokenStorageFailed creates an error indicating recovery token storage failed.
func RecoveryTokenStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to enable recovery token", err)
}

// WebAuthnServiceInitFailed creates an error indicating WebAuthn service initialization failed.
func WebAuthnServiceInitFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to create WebAuthn instance", err)
}

// UserRetrievalFailed creates an error indicating user retrieval failed.
func UserRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get user", err)
}

// CredentialRetrievalFailed creates an error indicating credential retrieval failed.
func CredentialRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get credentials", err)
}

// RegistrationBeginFailed creates an error indicating registration begin failed.
func RegistrationBeginFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to begin registration", err)
}

// LoginBeginFailed creates an error indicating login begin failed.
func LoginBeginFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to begin login", err)
}

// SessionDataSerializationFailed creates an error indicating session data serialization failed.
func SessionDataSerializationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to serialize session data", err)
}

// SessionDataDeserializationFailed creates an error indicating session data deserialization failed.
func SessionDataDeserializationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to deserialize session data", err)
}

// WebAuthnChallengeStorageFailed creates an error indicating WebAuthn challenge storage failed.
func WebAuthnChallengeStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to store challenge", err)
}

// CredentialResponseParseFailed creates an error indicating credential response parsing failed.
func CredentialResponseParseFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to parse credential response", err)
}

// CredentialCreationFailed creates an error indicating credential creation failed.
func CredentialCreationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to create credential", err)
}

// CredentialValidationFailed creates an error indicating credential validation failed.
func CredentialValidationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to validate login", err)
}

// InvalidSessionDataType creates an error indicating invalid session data type.
func InvalidSessionDataType() *AppError {
	return NewAuthError(CodeValidationFailed, "Invalid session data type")
}

// RecoveryCodeGenerationFailed creates an error indicating recovery code generation failed.
func RecoveryCodeGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate recovery code", err)
}

// RecoveryCodeHashingFailed creates an error indicating recovery code hashing failed.
func RecoveryCodeHashingFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to hash recovery code", err)
}

// RecoveryCodeStorageFailed creates an error indicating recovery code storage failed.
func RecoveryCodeStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to store recovery code", err)
}

// RecoveryCodeRetrievalFailed creates an error indicating recovery code retrieval failed.
func RecoveryCodeRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get recovery codes", err)
}

// RecoveryCodeMarkUsedFailed creates an error indicating marking recovery code as used failed.
func RecoveryCodeMarkUsedFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to mark recovery code as used", err)
}

// RecoveryCodeClearFailed creates an error indicating recovery code clearing failed.
func RecoveryCodeClearFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to clear existing recovery codes", err)
}

// AWSConfigLoadFailed creates an error indicating AWS config loading failed.
func AWSConfigLoadFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to load AWS config", err)
}

// SecretsManagerConnectionFailed creates an error indicating Secrets Manager connection failed.
func SecretsManagerConnectionFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to connect to AWS Secrets Manager", err)
}

// InvalidPrivateKeyFormat creates an error indicating invalid private key format.
func InvalidPrivateKeyFormat() *AppError {
	return NewAuthError(CodeValidationFailed, "Invalid private key format")
}

// SecretValueMarshalFailed creates an error indicating secret value marshalling failed.
func SecretValueMarshalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to marshal secret value", err)
}

// SecretCreationFailed creates an error indicating secret creation failed.
func SecretCreationFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to create secret", err)
}

// PrivateKeyRetrievalFailed creates an error indicating private key retrieval failed.
func PrivateKeyRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to retrieve private key", err)
}

// SecretValueUnmarshalFailed creates an error indicating secret value unmarshalling failed.
func SecretValueUnmarshalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to unmarshal secret value", err)
}

// RetrievedPrivateKeyInvalid creates an error indicating retrieved private key is invalid.
func RetrievedPrivateKeyInvalid() *AppError {
	return NewAuthError(CodeInternal, "Retrieved private key is invalid")
}

// SecretDeletionFailed creates an error indicating secret deletion failed.
func SecretDeletionFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to delete secret", err)
}

// RSAKeyPairGenerationFailed creates an error indicating RSA key pair generation failed.
func RSAKeyPairGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate RSA key pair", err)
}

// PrivateKeyMarshalFailed creates an error indicating private key marshalling failed.
func PrivateKeyMarshalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to marshal private key", err)
}

// PublicKeyMarshalFailed creates an error indicating public key marshalling failed.
func PublicKeyMarshalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to marshal public key", err)
}

// GeneratedPrivateKeyStorageFailed creates an error indicating generated private key storage failed.
func GeneratedPrivateKeyStorageFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to store generated private key", err)
}

// KeyPairGenerationRotationFailed creates an error indicating key pair generation during rotation failed.
func KeyPairGenerationRotationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate new key pair during rotation", err)
}

// PEMBlockDecodeFailed creates an error indicating PEM block decoding failed.
func PEMBlockDecodeFailed() *AppError {
	return NewAuthError(CodeValidationFailed, "Failed to decode PEM block")
}

// PrivateKeyParseFailed creates an error indicating private key parsing failed.
func PrivateKeyParseFailed(err error) *AppError {
	return NewAuthInternalError(CodeValidationFailed, "Failed to parse private key", err)
}

// SecretValueNil creates an error indicating secret value is nil.
func SecretValueNil() *AppError {
	return NewAuthError(CodeInternal, "Secret value is nil")
}

// SecretRetrievalRetriesFailed creates an error indicating secret retrieval failed after retries.
func SecretRetrievalRetriesFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to get secret after retries", err)
}

// AuditEventMarshalFailed creates an error indicating audit event marshalling failed.
func AuditEventMarshalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to marshal audit event", err)
}

// SIEMRequestCreationFailed creates an error indicating SIEM request creation failed.
func SIEMRequestCreationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to create SIEM request", err)
}

// SIEMTransmissionFailed creates an error indicating SIEM transmission failed.
func SIEMTransmissionFailed(err error) *AppError {
	return NewAuthInternalError(CodeExternalServiceUnavailable, "Failed to send to SIEM", err)
}

// SIEMResponseError creates an error indicating SIEM returned error status.
func SIEMResponseError() *AppError {
	return NewAuthError(CodeExternalServiceUnavailable, "SIEM returned error status")
}

// AuditRepositoryUnavailable creates an error indicating audit repository is not available.
func AuditRepositoryUnavailable() *AppError {
	return NewAuthError(CodeInternal, "Audit repository not available")
}

// IPRateLimitCheckFailed creates an error indicating IP rate limit check failed.
func IPRateLimitCheckFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to check IP rate limit", err)
}

// AccountRateLimitCheckFailed creates an error indicating account rate limit check failed.
func AccountRateLimitCheckFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to check account rate limit", err)
}

// RecordIPAttemptFailed creates an error indicating recording IP attempt failed.
func RecordIPAttemptFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to record IP attempt", err)
}

// RecordAccountAttemptFailed creates an error indicating recording account attempt failed.
func RecordAccountAttemptFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to record account attempt", err)
}

// GetIPAttemptCountFailed creates an error indicating getting IP attempt count failed.
func GetIPAttemptCountFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get IP attempt count", err)
}

// GetAccountAttemptCountFailed creates an error indicating getting account attempt count failed.
func GetAccountAttemptCountFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get account attempt count", err)
}

// ImposeIPLockoutFailed creates an error indicating imposing IP lockout failed.
func ImposeIPLockoutFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to impose IP lockout", err)
}

// ImposeAccountLockoutFailed creates an error indicating imposing account lockout failed.
func ImposeAccountLockoutFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to impose account lockout", err)
}

// RefreshTokenGenerationFailed creates an error indicating refresh token generation failed.
func RefreshTokenGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate refresh token", err)
}

// DeviceIDRetrievalFailed creates an error indicating device ID retrieval failed.
func DeviceIDRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get device ID", err)
}

// NewRefreshTokenGenerationFailed creates an error indicating new refresh token generation failed.
func NewRefreshTokenGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate new refresh token", err)
}

// SessionUpdateFailed creates an error indicating session update failed.
func SessionUpdateFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to update session", err)
}

// UserSessionsRetrievalFailed creates an error indicating user sessions retrieval failed.
func UserSessionsRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get user sessions", err)
}

// OldestSessionRemovalFailed creates an error indicating oldest session removal failed.
func OldestSessionRemovalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to remove oldest session", err)
}

// UserDevicesRetrievalFailed creates an error indicating user devices retrieval failed.
func UserDevicesRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get user devices", err)
}

// DeviceCreationFailed creates an error indicating device creation failed.
func DeviceCreationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to create device", err)
}

// JWTUnexpectedSigningMethod creates an error indicating JWT unexpected signing method.
func JWTUnexpectedSigningMethod() *AppError {
	return NewAuthError(CodeAuthFailed, "Unexpected signing method")
}

// InvalidActivityObject creates an error indicating invalid activity object.
func InvalidActivityObject() *AppError {
	return NewAuthError(CodeValidationFailed, "Invalid activity object")
}

// NotRecoveryConfirmationActivity creates an error indicating not a recovery confirmation activity.
func NotRecoveryConfirmationActivity() *AppError {
	return NewAuthError(CodeValidationFailed, "Not a recovery confirmation activity")
}

// MissingRequestID creates an error indicating missing request ID.
func MissingRequestID() *AppError {
	return NewAuthError(CodeRequiredFieldMissing, "Missing request ID")
}

// FailedToDecodePEM creates an error indicating failed to decode private key PEM.
func FailedToDecodePEM() *AppError {
	return NewAuthError(CodeValidationFailed, "Failed to decode private key PEM")
}

// UnsupportedPrivateKeyType creates an error indicating unsupported private key type.
func UnsupportedPrivateKeyType() *AppError {
	return NewAuthError(CodeValidationFailed, "Unsupported private key type")
}

// SecretsManagerNotAvailable creates an error indicating secrets manager is not available.
func SecretsManagerNotAvailable() *AppError {
	return NewAuthError(CodeInternal, "Secrets manager not available")
}

// SigningActorRetrievalFailed creates an error indicating signing actor retrieval failed.
func SigningActorRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get signing actor", err)
}

// RecoveryConfirmationFailed creates an error indicating recovery confirmation processing failed.
func RecoveryConfirmationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to process recovery confirmation", err)
}

// ActorRetrievalFailed creates an error indicating actor retrieval failed.
func ActorRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to get actor", err)
}

// SystemActorKeyRetrievalFailed creates an error indicating system actor private key retrieval failed.
func SystemActorKeyRetrievalFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to retrieve system actor private key", err)
}

// SystemActorKeyRotationFailed creates an error indicating system actor key rotation failed.
func SystemActorKeyRotationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to rotate system actor key", err)
}

// SessionMaxLifetimeReached creates an error indicating session max lifetime reached.
func SessionMaxLifetimeReached() *AppError {
	return NewAuthError(CodeForbidden, "Session cannot be extended (max lifetime reached)")
}

// SessionExtensionDisabled creates an error indicating session extension is disabled.
func SessionExtensionDisabled() *AppError {
	return NewAuthError(CodeForbidden, "Session extension is disabled")
}

// RefreshTokenRotationFailed creates an error indicating refresh token rotation failed.
func RefreshTokenRotationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to rotate refresh token", err)
}

// InvalidRefreshTokenProvided creates an error indicating invalid refresh token provided.
func InvalidRefreshTokenProvided() *AppError {
	return NewAuthError(CodeTokenInvalid, "Invalid refresh token provided")
}

// SessionSecurityCheckFailed creates an error indicating session security check failed.
func SessionSecurityCheckFailed(err error) *AppError {
	return NewAuthInternalError(CodeAuthFailed, "Session security check failed", err)
}

// SessionCreationFailed creates an error indicating session creation failed.
func SessionCreationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to create session", err)
}

// AccessTokenGenerationFailed creates an error indicating access token generation failed.
func AccessTokenGenerationFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to generate access token", err)
}

// PasswordUpdateFailed creates an error indicating password update failed.
func PasswordUpdateFailed(err error) *AppError {
	return NewAuthInternalError(CodeInternal, "Failed to update password", err)
}