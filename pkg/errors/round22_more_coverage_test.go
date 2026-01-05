package errors

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertAppErrorBasics(t *testing.T, err *AppError) {
	t.Helper()
	require.NotNil(t, err)
	assert.NotEmpty(t, err.Message)
	assert.NotEmpty(t, err.Code)
	assert.NotEmpty(t, err.Category)

	assert.True(t, err.Code.IsValid())
	assert.True(t, err.Category.IsValid())
	assert.Equal(t, err.Code.GetHTTPStatusCode(), err.HTTPStatusCode)
}

func TestErrorCategories_StringAndValidation(t *testing.T) {
	cats := AllCategories()
	require.NotEmpty(t, cats)

	for _, cat := range cats {
		assert.Equal(t, string(cat), cat.String())
		assert.True(t, cat.IsValid())
	}

	assert.False(t, ErrorCategory("").IsValid())
	assert.False(t, ErrorCategory("NOPE").IsValid())
}

func TestErrorCodes_StringValidationAndHTTPStatusMappings(t *testing.T) {
	assert.Equal(t, "NOT_FOUND", CodeNotFound.String())
	assert.True(t, CodeNotFound.IsValid())
	assert.False(t, ErrorCode("").IsValid())

	assert.Equal(t, 405, CodeMethodNotAllowed.GetHTTPStatusCode())
	assert.Equal(t, 415, CodeUnsupportedMediaType.GetHTTPStatusCode())
	assert.Equal(t, 429, CodeRateLimited.GetHTTPStatusCode())
	assert.Equal(t, 429, CodeQuotaExceeded.GetHTTPStatusCode())
	assert.Equal(t, 408, CodeTimeout.GetHTTPStatusCode())
	assert.Equal(t, 503, CodeExternalServiceUnavailable.GetHTTPStatusCode())
	assert.Equal(t, 500, ErrorCode("UNKNOWN").GetHTTPStatusCode())
}

func TestAuthConstructors_MoreCoverage(t *testing.T) {
	boom := stdErrors.New("boom")

	constructors := []struct {
		name string
		fn   func() *AppError
	}{
		{name: "NewAuthError", fn: func() *AppError { return NewAuthError(CodeAuthFailed, "msg") }},
		{name: "NewAuthInternalError", fn: func() *AppError { return NewAuthInternalError(CodeAuthFailed, "msg", boom) }},
		{name: "AuthFailed", fn: func() *AppError { return AuthFailed("reason") }},
		{name: "InvalidCredentials", fn: func() *AppError { return InvalidCredentials() }},
		{name: "UserNotFound", fn: func() *AppError { return UserNotFound("alice") }},
		{name: "UserSuspended", fn: func() *AppError { return UserSuspended("alice") }},
		{name: "UserNotApproved", fn: func() *AppError { return UserNotApproved("alice") }},
		{name: "TokenExpired", fn: func() *AppError { return TokenExpired() }},
		{name: "TokenInvalid", fn: func() *AppError { return TokenInvalid("bad") }},
		{name: "TokenRevoked", fn: func() *AppError { return TokenRevoked() }},
		{name: "TokenReuse", fn: func() *AppError { return TokenReuse() }},
		{name: "RefreshTokenInvalid", fn: func() *AppError { return RefreshTokenInvalid() }},
		{name: "RefreshTokenExpired", fn: func() *AppError { return RefreshTokenExpired() }},
		{name: "SessionNotFound", fn: func() *AppError { return SessionNotFound("sess") }},
		{name: "SessionExpired", fn: func() *AppError { return SessionExpired() }},
		{name: "SessionSecurityValidationFailed", fn: func() *AppError { return SessionSecurityValidationFailed("reason") }},
		{name: "SessionCannotBeExtended", fn: func() *AppError { return SessionCannotBeExtended("reason") }},
		{name: "ConcurrentSessionLimitExceeded", fn: func() *AppError { return ConcurrentSessionLimitExceeded() }},
		{name: "InsufficientScope", fn: func() *AppError { return InsufficientScope("read") }},
		{name: "AccessDenied", fn: func() *AppError { return AccessDenied("resource") }},
		{name: "OperationNotAllowed", fn: func() *AppError { return OperationNotAllowed("op") }},
		{name: "PasswordTooShort", fn: func() *AppError { return PasswordTooShort(8) }},
		{name: "PasswordTooLong", fn: func() *AppError { return PasswordTooLong(128) }},
		{name: "PasswordMissingRequirement", fn: func() *AppError { return PasswordMissingRequirement("upper") }},
		{name: "PasswordTooCommon", fn: func() *AppError { return PasswordTooCommon() }},
		{name: "PasswordContainsUsername", fn: func() *AppError { return PasswordContainsUsername() }},
		{name: "PasswordHashingFailed", fn: func() *AppError { return PasswordHashingFailed(boom) }},
		{name: "WebAuthnNotConfigured", fn: func() *AppError { return WebAuthnNotConfigured() }},
		{name: "WebAuthnRegistrationFailed", fn: func() *AppError { return WebAuthnRegistrationFailed("reason") }},
		{name: "WebAuthnLoginFailed", fn: func() *AppError { return WebAuthnLoginFailed("reason") }},
		{name: "CredentialNotFound", fn: func() *AppError { return CredentialNotFound() }},
		{name: "MaxCredentialsReached", fn: func() *AppError { return MaxCredentialsReached() }},
		{name: "LastAuthMethodDelete", fn: func() *AppError { return LastAuthMethodDelete() }},
		{name: "WalletChallengeExpired", fn: func() *AppError { return WalletChallengeExpired() }},
		{name: "WalletSignatureInvalid", fn: func() *AppError { return WalletSignatureInvalid("reason") }},
		{name: "WalletAlreadyLinked", fn: func() *AppError { return WalletAlreadyLinked() }},
		{name: "WalletAddressMismatch", fn: func() *AppError { return WalletAddressMismatch() }},
		{name: "RateLimitExceeded", fn: func() *AppError { return RateLimitExceeded("login", 123) }},
		{name: "IPRateLimitExceeded", fn: func() *AppError { return IPRateLimitExceeded(123) }},
		{name: "AccountRateLimitExceeded", fn: func() *AppError { return AccountRateLimitExceeded(123) }},
		{name: "DeviceNotFound", fn: func() *AppError { return DeviceNotFound("device") }},
		{name: "MaxDevicesExceeded", fn: func() *AppError { return MaxDevicesExceeded() }},
		{name: "DeviceOwnershipMismatch", fn: func() *AppError { return DeviceOwnershipMismatch() }},
		{name: "OAuthInvalidClient", fn: func() *AppError { return OAuthInvalidClient() }},
		{name: "OAuthInvalidGrant", fn: func() *AppError { return OAuthInvalidGrant() }},
		{name: "OAuthInvalidScope", fn: func() *AppError { return OAuthInvalidScope("scope") }},
		{name: "OAuthUnsupportedGrantType", fn: func() *AppError { return OAuthUnsupportedGrantType("gt") }},
		{name: "RecoveryRequestNotFound", fn: func() *AppError { return RecoveryRequestNotFound() }},
		{name: "RecoveryRequestExpired", fn: func() *AppError { return RecoveryRequestExpired() }},
		{name: "InsufficientTrustees", fn: func() *AppError { return InsufficientTrustees() }},
		{name: "TrusteeAlreadyVoted", fn: func() *AppError { return TrusteeAlreadyVoted() }},
		{name: "RecoveryCodeInvalid", fn: func() *AppError { return RecoveryCodeInvalid() }},
		{name: "RecoveryCodeUsed", fn: func() *AppError { return RecoveryCodeUsed() }},
		{name: "CSRFTokenMissing", fn: func() *AppError { return CSRFTokenMissing() }},
		{name: "CSRFTokenInvalid", fn: func() *AppError { return CSRFTokenInvalid() }},
		{name: "AuthServiceUnavailable", fn: func() *AppError { return AuthServiceUnavailable(boom) }},
		{name: "SessionStorageFailed", fn: func() *AppError { return SessionStorageFailed(boom) }},
		{name: "TokenGenerationFailed", fn: func() *AppError { return TokenGenerationFailed(boom) }},
		{name: "CredentialStorageFailed", fn: func() *AppError { return CredentialStorageFailed(boom) }},
		{name: "SecretsManagerError", fn: func() *AppError { return SecretsManagerError(boom) }},
		{name: "AuditLoggingFailed", fn: func() *AppError { return AuditLoggingFailed(boom) }},
	}

	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			assertAppErrorBasics(t, err)
			assert.Equal(t, CategoryAuth, err.Category)
		})
	}
}
