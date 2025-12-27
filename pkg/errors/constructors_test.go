package errors

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthConstructors tests authentication domain error constructors
func TestAuthConstructors(t *testing.T) {
	tests := []struct {
		name             string
		createError      func() *AppError
		expectedCode     ErrorCode
		expectedCategory ErrorCategory
		expectedMetaKeys []string
	}{
		{
			name:             "AuthFailed",
			createError:      func() *AppError { return AuthFailed("bad password") },
			expectedCode:     CodeAuthFailed,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"reason"},
		},
		{
			name:             "InvalidCredentials",
			createError:      func() *AppError { return InvalidCredentials() },
			expectedCode:     CodeAuthFailed,
			expectedCategory: CategoryAuth,
		},
		{
			name:             "UserNotFound",
			createError:      func() *AppError { return UserNotFound("alice") },
			expectedCode:     CodeNotFound,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"username"},
		},
		{
			name:             "UserSuspended",
			createError:      func() *AppError { return UserSuspended("bob") },
			expectedCode:     CodeAccountSuspended,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"username"},
		},
		{
			name:             "TokenExpired",
			createError:      func() *AppError { return TokenExpired() },
			expectedCode:     CodeTokenExpired,
			expectedCategory: CategoryAuth,
		},
		{
			name:             "TokenInvalid",
			createError:      func() *AppError { return TokenInvalid("malformed") },
			expectedCode:     CodeTokenInvalid,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"reason"},
		},
		{
			name:             "AccessDenied",
			createError:      func() *AppError { return AccessDenied("admin-panel") },
			expectedCode:     CodeForbidden,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"resource"},
		},
		{
			name:             "InsufficientScope",
			createError:      func() *AppError { return InsufficientScope("write:statuses") },
			expectedCode:     CodeInsufficientScope,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"required_scope"},
		},
		{
			name:             "SessionExpired",
			createError:      func() *AppError { return SessionExpired() },
			expectedCode:     CodeSessionExpired,
			expectedCategory: CategoryAuth,
		},
		{
			name:             "SessionNotFound",
			createError:      func() *AppError { return SessionNotFound("sess-123") },
			expectedCode:     CodeNotFound,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"session_id"},
		},
		{
			name:             "RateLimitExceeded",
			createError:      func() *AppError { return RateLimitExceeded("login", 1234567890) },
			expectedCode:     CodeRateLimited,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"limit_type", "reset_time"},
		},
		{
			name:             "PasswordTooShort",
			createError:      func() *AppError { return PasswordTooShort(8) },
			expectedCode:     CodeFieldTooShort,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"min_length"},
		},
		{
			name:             "OAuthInvalidClient",
			createError:      func() *AppError { return OAuthInvalidClient() },
			expectedCode:     CodeUnauthorized,
			expectedCategory: CategoryAuth,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.expectedCategory, err.Category)
			assert.NotEmpty(t, err.Message)
			for _, key := range tc.expectedMetaKeys {
				assert.Contains(t, err.Metadata, key, "expected metadata key %s", key)
			}
		})
	}
}

// TestStorageConstructors tests storage domain error constructors
func TestStorageConstructors(t *testing.T) {
	tests := []struct {
		name             string
		createError      func() *AppError
		expectedCode     ErrorCode
		expectedCategory ErrorCategory
		expectedMetaKeys []string
		expectRetryable  bool
	}{
		{
			name:             "ItemNotFound",
			createError:      func() *AppError { return ItemNotFound("user") },
			expectedCode:     CodeNotFound,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"item_type"},
		},
		{
			name:             "ItemNotFoundWithID",
			createError:      func() *AppError { return ItemNotFoundWithID("status", "123") },
			expectedCode:     CodeNotFound,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"item_type", "id"},
		},
		{
			name:             "ItemAlreadyExists",
			createError:      func() *AppError { return ItemAlreadyExists("actor") },
			expectedCode:     CodeAlreadyExists,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"item_type"},
		},
		{
			name:             "DatabaseTimeout",
			createError:      func() *AppError { return DatabaseTimeout("query") },
			expectedCode:     CodeTimeout,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"operation"},
			expectRetryable:  true,
		},
		{
			name:             "DynamoDBProvisionedThroughputExceeded",
			createError:      func() *AppError { return DynamoDBProvisionedThroughputExceeded() },
			expectedCode:     CodeRateLimited,
			expectedCategory: CategoryStorage,
			expectRetryable:  true,
		},
		{
			name:             "DynamoDBItemTooLarge",
			createError:      func() *AppError { return DynamoDBItemTooLarge() },
			expectedCode:     CodeContentTooLarge,
			expectedCategory: CategoryStorage,
		},
		{
			name:             "DynamoDBConditionalCheckFailed",
			createError:      func() *AppError { return DynamoDBConditionalCheckFailed("version=1") },
			expectedCode:     CodeConflict,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"condition"},
		},
		{
			name:             "UniqueConstraintViolated",
			createError:      func() *AppError { return UniqueConstraintViolated("email") },
			expectedCode:     CodeAlreadyExists,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"field"},
		},
		{
			name:             "BatchSizeExceeded",
			createError:      func() *AppError { return BatchSizeExceeded(100, 25) },
			expectedCode:     CodeBadRequest,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"size", "max_size"},
		},
		{
			name:             "StatusNotFound",
			createError:      func() *AppError { return StatusNotFound("status-456") },
			expectedCode:     CodeNotFound,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"item_type", "id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.expectedCategory, err.Category)
			assert.NotEmpty(t, err.Message)
			if tc.expectRetryable {
				assert.True(t, err.Retryable, "expected error to be retryable")
			}
			for _, key := range tc.expectedMetaKeys {
				assert.Contains(t, err.Metadata, key)
			}
		})
	}
}

// TestFederationConstructors tests federation domain error constructors
func TestFederationConstructors(t *testing.T) {
	tests := []struct {
		name             string
		createError      func() *AppError
		expectedCode     ErrorCode
		expectedCategory ErrorCategory
		expectedMetaKeys []string
		expectRetryable  bool
	}{
		{
			name:             "ActivityTypeUnsupported",
			createError:      func() *AppError { return ActivityTypeUnsupported("Unknown") },
			expectedCode:     CodeUnsupportedActivityType,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"activity_type"},
		},
		{
			name:             "ActivityMissingField",
			createError:      func() *AppError { return ActivityMissingField("actor") },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"field"},
		},
		{
			name:             "ActorNotFound",
			createError:      func() *AppError { return ActorNotFound("https://example.com/users/bob") },
			expectedCode:     CodeActorNotFound,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"actor_id"},
		},
		{
			name:             "ActorDomainBlocked",
			createError:      func() *AppError { return ActorDomainBlocked("spam.example") },
			expectedCode:     CodeFederationBlocked,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"domain"},
		},
		{
			name:             "SignatureMissing",
			createError:      func() *AppError { return SignatureMissing() },
			expectedCode:     CodeSignatureVerifyFailed,
			expectedCategory: CategoryFederation,
		},
		{
			name:             "SignatureExpired",
			createError:      func() *AppError { return SignatureExpired() },
			expectedCode:     CodeSignatureVerifyFailed,
			expectedCategory: CategoryFederation,
		},
		{
			name:             "SigningKeyNotFound",
			createError:      func() *AppError { return SigningKeyNotFound("key-123") },
			expectedCode:     CodeSignatureVerifyFailed,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"key_id"},
		},
		{
			name:             "InboxMessageDuplicate",
			createError:      func() *AppError { return InboxMessageDuplicate("activity-789") },
			expectedCode:     CodeAlreadyExists,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"activity_id"},
		},
		{
			name:             "DeliveryTimeout",
			createError:      func() *AppError { return DeliveryTimeout("https://remote.example/inbox") },
			expectedCode:     CodeTimeout,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"recipient"},
			expectRetryable:  true,
		},
		{
			name:             "InstanceSuspended",
			createError:      func() *AppError { return InstanceSuspended("banned.example") },
			expectedCode:     CodeForbidden,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"domain"},
		},
		{
			name:             "FollowAlreadyExists",
			createError:      func() *AppError { return FollowAlreadyExists("alice", "bob") },
			expectedCode:     CodeAlreadyExists,
			expectedCategory: CategoryFederation,
			expectedMetaKeys: []string{"follower", "followee"},
		},
		{
			name:             "CreateObjectMissing",
			createError:      func() *AppError { return CreateObjectMissing() },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryFederation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.expectedCategory, err.Category)
			assert.NotEmpty(t, err.Message)
			if tc.expectRetryable {
				assert.True(t, err.Retryable)
			}
			for _, key := range tc.expectedMetaKeys {
				assert.Contains(t, err.Metadata, key)
			}
		})
	}
}

// TestLambdaConstructors tests Lambda domain error constructors
func TestLambdaConstructors(t *testing.T) {
	tests := []struct {
		name             string
		createError      func() *AppError
		expectedCode     ErrorCode
		expectedCategory ErrorCategory
		expectedMetaKeys []string
		expectRetryable  bool
	}{
		{
			name:             "LambdaTimeout",
			createError:      func() *AppError { return LambdaTimeout("my-function") },
			expectedCode:     CodeLambdaTimeout,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"function_name"},
		},
		{
			name:             "LambdaColdStart",
			createError:      func() *AppError { return LambdaColdStart("my-function", 2500) },
			expectedCode:     CodeLambdaColdStart,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"function_name", "duration_ms"},
		},
		{
			name:             "LambdaMemoryExceeded",
			createError:      func() *AppError { return LambdaMemoryExceeded("my-function", 512) },
			expectedCode:     CodeLambdaMemoryExceeded,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"function_name", "memory_used_mb"},
		},
		{
			name:             "SQSMessageInvalid",
			createError:      func() *AppError { return SQSMessageInvalid("msg-123", "missing body") },
			expectedCode:     CodeBadRequest,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"message_id", "reason"},
		},
		{
			name:             "SQSRetryExhausted",
			createError:      func() *AppError { return SQSRetryExhausted("msg-456", 5) },
			expectedCode:     CodeDLQRetryExhausted,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"message_id", "attempts"},
		},
		{
			name:             "DLQRetryExhausted",
			createError:      func() *AppError { return DLQRetryExhausted("dlq-msg-789", 3) },
			expectedCode:     CodeDLQRetryExhausted,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"message_id", "max_attempts"},
		},
		{
			name:             "EventInvalid",
			createError:      func() *AppError { return EventInvalid("SQSEvent", "no records") },
			expectedCode:     CodeBadRequest,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"event_type", "reason"},
		},
		{
			name:             "EventMissingField",
			createError:      func() *AppError { return EventMissingField("DynamoDBStream", "Keys") },
			expectedCode:     CodeBadRequest,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"event_type", "field"},
		},
		{
			name:             "StreamNewImageMissing",
			createError:      func() *AppError { return StreamNewImageMissing("record-001") },
			expectedCode:     CodeBadRequest,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"record_id"},
		},
		{
			name:             "EnvironmentVariableMissing",
			createError:      func() *AppError { return EnvironmentVariableMissing("DYNAMODB_TABLE") },
			expectedCode:     CodeInternal,
			expectedCategory: CategoryLambda,
			expectedMetaKeys: []string{"variable_name"},
		},
		{
			name:             "StreamingConnectionNotFound",
			createError:      func() *AppError { return StreamingConnectionNotFound() },
			expectedCode:     CodeNotFound,
			expectedCategory: CategoryLambda,
		},
		{
			name:             "StreamingInvalidMessageFormat",
			createError:      func() *AppError { return StreamingInvalidMessageFormat() },
			expectedCode:     CodeBadRequest,
			expectedCategory: CategoryLambda,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.expectedCategory, err.Category)
			assert.NotEmpty(t, err.Message)
			if tc.expectRetryable {
				assert.True(t, err.Retryable)
			}
			for _, key := range tc.expectedMetaKeys {
				assert.Contains(t, err.Metadata, key)
			}
		})
	}
}

// TestValidationConstructors tests validation domain error constructors
func TestValidationConstructors(t *testing.T) {
	tests := []struct {
		name             string
		createError      func() *AppError
		expectedCode     ErrorCode
		expectedCategory ErrorCategory
		expectedMetaKeys []string
	}{
		{
			name:             "RequiredFieldMissing",
			createError:      func() *AppError { return RequiredFieldMissing("username") },
			expectedCode:     CodeRequiredFieldMissing,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field"},
		},
		{
			name:             "FieldTooLong",
			createError:      func() *AppError { return FieldTooLong("bio", 500, 600) },
			expectedCode:     CodeFieldTooLong,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "max_length", "actual_length"},
		},
		{
			name:             "FieldTooShort",
			createError:      func() *AppError { return FieldTooShort("password", 8, 4) },
			expectedCode:     CodeFieldTooShort,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "min_length", "actual_length"},
		},
		{
			name:             "InvalidFormat",
			createError:      func() *AppError { return InvalidFormat("email", "user@domain.com") },
			expectedCode:     CodeInvalidFormat,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "expected_format"},
		},
		{
			name:             "UsernameEmpty",
			createError:      func() *AppError { return UsernameEmpty() },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field"},
		},
		{
			name:             "EmailInvalidFormat",
			createError:      func() *AppError { return EmailInvalidFormat() },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field"},
		},
		{
			name:             "StatusTooManyMedia",
			createError:      func() *AppError { return StatusTooManyMedia(4, 10) },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "max_count", "actual_count"},
		},
		{
			name:             "PollTooFewOptions",
			createError:      func() *AppError { return PollTooFewOptions(2) },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "min_options"},
		},
		{
			name:             "MediaFileTooLarge",
			createError:      func() *AppError { return MediaFileTooLarge(50000000, 40000000) },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "file_size", "max_size"},
		},
		{
			name:             "URLInvalid",
			createError:      func() *AppError { return URLInvalid("not-a-url") },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "url"},
		},
		{
			name:             "JSONInvalid",
			createError:      func() *AppError { return JSONInvalid("unexpected EOF") },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field", "reason"},
		},
		{
			name:             "ContentMustHaveContentOrMedia",
			createError:      func() *AppError { return ContentMustHaveContentOrMedia() },
			expectedCode:     CodeValidationFailed,
			expectedCategory: CategoryValidation,
			expectedMetaKeys: []string{"field"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.expectedCategory, err.Category)
			assert.NotEmpty(t, err.Message)
			for _, key := range tc.expectedMetaKeys {
				assert.Contains(t, err.Metadata, key)
			}
		})
	}
}

// TestCommonConstructors tests common error constructors
func TestCommonConstructors(t *testing.T) {
	innerErr := stdErrors.New("inner error")

	tests := []struct {
		name             string
		createError      func() *AppError
		expectedCode     ErrorCode
		expectedCategory ErrorCategory
		expectedMetaKeys []string
		expectRetryable  bool
	}{
		{
			name:             "FailedToCreate",
			createError:      func() *AppError { return FailedToCreate("user", innerErr) },
			expectedCode:     CodeInternal,
			expectedCategory: CategoryStorage,
		},
		{
			name:             "FailedToGet",
			createError:      func() *AppError { return FailedToGet("status", innerErr) },
			expectedCode:     CodeInternal,
			expectedCategory: CategoryStorage,
		},
		{
			name:             "FailedToList",
			createError:      func() *AppError { return FailedToList("followers", innerErr) },
			expectedCode:     CodeInternal,
			expectedCategory: CategoryStorage,
			expectRetryable:  true,
		},
		{
			name:             "ServiceUnavailable",
			createError:      func() *AppError { return ServiceUnavailable("auth") },
			expectedCode:     CodeExternalServiceUnavailable,
			expectedCategory: CategoryExternal,
			expectedMetaKeys: []string{"service_name"},
			expectRetryable:  true,
		},
		{
			name:             "TimeoutError",
			createError:      func() *AppError { return TimeoutError("database query") },
			expectedCode:     CodeTimeout,
			expectedCategory: CategoryInternal,
			expectedMetaKeys: []string{"operation"},
			expectRetryable:  true,
		},
		{
			name:             "QuotaExceeded",
			createError:      func() *AppError { return QuotaExceeded("api-calls", 1000) },
			expectedCode:     CodeQuotaExceeded,
			expectedCategory: CategoryBusiness,
			expectedMetaKeys: []string{"quota_type", "limit"},
		},
		{
			name:             "ConfigurationMissing",
			createError:      func() *AppError { return ConfigurationMissing("JWT_SECRET") },
			expectedCode:     CodeInternal,
			expectedCategory: CategoryInternal,
			expectedMetaKeys: []string{"config_key"},
		},
		{
			name:             "InvalidStateForOperation",
			createError:      func() *AppError { return InvalidStateForOperation("pending", "delete") },
			expectedCode:     CodeInvalidStateTransition,
			expectedCategory: CategoryBusiness,
			expectedMetaKeys: []string{"current_state", "operation"},
		},
		{
			name:             "ConcurrentModification",
			createError:      func() *AppError { return ConcurrentModification("status") },
			expectedCode:     CodeConcurrencyError,
			expectedCategory: CategoryStorage,
			expectedMetaKeys: []string{"resource_type"},
			expectRetryable:  true,
		},
		{
			name:             "InsufficientPermissions",
			createError:      func() *AppError { return InsufficientPermissions("delete") },
			expectedCode:     CodeForbidden,
			expectedCategory: CategoryAuth,
			expectedMetaKeys: []string{"operation"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.expectedCategory, err.Category)
			assert.NotEmpty(t, err.Message)
			if tc.expectRetryable {
				assert.True(t, err.Retryable)
			}
			for _, key := range tc.expectedMetaKeys {
				assert.Contains(t, err.Metadata, key)
			}
		})
	}
}

// TestWrappingConstructors tests error wrapping constructors preserve underlying error
func TestWrappingConstructors(t *testing.T) {
	innerErr := stdErrors.New("underlying error")

	tests := []struct {
		name        string
		createError func() *AppError
	}{
		{"PasswordHashingFailed", func() *AppError { return PasswordHashingFailed(innerErr) }},
		{"DatabaseConnectionFailed", func() *AppError { return DatabaseConnectionFailed(innerErr) }},
		{"TransactionFailed", func() *AppError { return TransactionFailed(innerErr) }},
		{"QueryFailed", func() *AppError { return QueryFailed("get_users", innerErr) }},
		{"ActivityParsingFailed", func() *AppError { return ActivityParsingFailed("Create", innerErr) }},
		{"DeliveryFailed", func() *AppError { return DeliveryFailed("https://example.com/inbox", innerErr) }},
		{"ActorFetchFailed", func() *AppError { return ActorFetchFailed("actor-123", innerErr) }},
		{"LambdaInitializationFailed", func() *AppError { return LambdaInitializationFailed("fn", innerErr) }},
		{"SQSMessageProcessingFailed", func() *AppError { return SQSMessageProcessingFailed("msg-1", innerErr) }},
		{"EventProcessingFailed", func() *AppError { return EventProcessingFailed("SQS", innerErr) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createError()
			require.NotNil(t, err)

			// Verify errors.Is works with wrapped error
			assert.ErrorIs(t, err, innerErr, "errors.Is should find inner error")

			// Verify InternalMessage contains inner error text
			assert.Contains(t, err.InternalMessage, "underlying error")
		})
	}
}

// TestHelperFunctions tests helper functions for error inspection
func TestHelperFunctions(t *testing.T) {
	t.Run("IsRetryableError", func(t *testing.T) {
		retryable := DatabaseTimeout("query")
		assert.True(t, IsRetryableError(retryable))

		notRetryable := InvalidCredentials()
		assert.False(t, IsRetryableError(notRetryable))

		plainErr := stdErrors.New("plain error")
		assert.False(t, IsRetryableError(plainErr))
	})

	t.Run("IsTemporaryError", func(t *testing.T) {
		timeout := TimeoutError("operation")
		assert.True(t, IsTemporaryError(timeout))

		rateLimited := RateLimitExceeded("api", 0)
		assert.True(t, IsTemporaryError(rateLimited))

		permanent := UserNotFound("missing-user")
		assert.False(t, IsTemporaryError(permanent))
	})

	t.Run("IsClientError", func(t *testing.T) {
		badRequest := RequiredFieldMissing("field")
		assert.True(t, IsClientError(badRequest))

		serverError := DatabaseConnectionFailed(stdErrors.New("db"))
		assert.False(t, IsClientError(serverError))
	})

	t.Run("IsServerError", func(t *testing.T) {
		serverError := DatabaseConnectionFailed(stdErrors.New("db"))
		assert.True(t, IsServerError(serverError))

		clientError := RequiredFieldMissing("field")
		assert.False(t, IsServerError(clientError))
	})

	t.Run("HasCode_NonAppError", func(t *testing.T) {
		plainErr := stdErrors.New("plain error")
		assert.False(t, HasCode(plainErr, CodeNotFound))
	})

	t.Run("HasCategory_NonAppError", func(t *testing.T) {
		plainErr := stdErrors.New("plain error")
		assert.False(t, HasCategory(plainErr, CategoryAuth))
	})

	t.Run("GetErrorCode_NonAppError", func(t *testing.T) {
		plainErr := stdErrors.New("plain error")
		code := GetErrorCode(plainErr)
		// GetErrorCode returns CodeInternal for non-AppErrors
		assert.Equal(t, CodeInternal, code)
	})

	t.Run("GetErrorCategory_NonAppError", func(t *testing.T) {
		plainErr := stdErrors.New("plain error")
		cat := GetErrorCategory(plainErr)
		// GetErrorCategory returns CategoryInternal for non-AppErrors
		assert.Equal(t, CategoryInternal, cat)
	})

	t.Run("GetHTTPStatus_NonAppError", func(t *testing.T) {
		plainErr := stdErrors.New("plain error")
		status := GetHTTPStatus(plainErr)
		assert.Equal(t, 500, status)
	})
}
