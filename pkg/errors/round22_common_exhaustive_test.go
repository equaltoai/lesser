package errors

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommonErrors_Constructors_ExhaustiveCoverage(t *testing.T) {
	boom := stdErrors.New("boom")

	constructors := []struct {
		name string
		fn   func() *AppError
	}{
		{name: "FailedToCreate", fn: func() *AppError { return FailedToCreate("item", boom) }},
		{name: "FailedToGet", fn: func() *AppError { return FailedToGet("item", boom) }},
		{name: "FailedToUpdate", fn: func() *AppError { return FailedToUpdate("item", boom) }},
		{name: "FailedToDelete", fn: func() *AppError { return FailedToDelete("item", boom) }},
		{name: "FailedToList", fn: func() *AppError { return FailedToList("item", boom) }},
		{name: "FailedToQuery", fn: func() *AppError { return FailedToQuery("item", boom) }},
		{name: "FailedToStore", fn: func() *AppError { return FailedToStore("item", boom) }},
		{name: "FailedToRetrieve", fn: func() *AppError { return FailedToRetrieve("item", boom) }},
		{name: "FailedToSave", fn: func() *AppError { return FailedToSave("item", boom) }},
		{name: "FailedToRemove", fn: func() *AppError { return FailedToRemove("item", boom) }},
		{name: "OperationNotAllowedOnSelf", fn: func() *AppError { return OperationNotAllowedOnSelf("op") }},
		{name: "InsufficientPermissions", fn: func() *AppError { return InsufficientPermissions("op") }},
		{name: "ResourceUnavailable", fn: func() *AppError { return ResourceUnavailable("resource") }},
		{name: "ServiceUnavailable", fn: func() *AppError { return ServiceUnavailable("service") }},
		{name: "ProcessingFailed_nil", fn: func() *AppError { return ProcessingFailed("process", nil) }},
		{name: "ProcessingFailed_withErr", fn: func() *AppError { return ProcessingFailed("process", boom) }},
		{name: "ParsingFailed", fn: func() *AppError { return ParsingFailed("parse", boom) }},
		{name: "MarshalingFailed", fn: func() *AppError { return MarshalingFailed("type", boom) }},
		{name: "UnmarshalingFailed", fn: func() *AppError { return UnmarshalingFailed("type", boom) }},
		{name: "NetworkError", fn: func() *AppError { return NetworkError("op", boom) }},
		{name: "TimeoutError", fn: func() *AppError { return TimeoutError("op") }},
		{name: "ExternalAPIError", fn: func() *AppError { return ExternalAPIError("api", 500, boom) }},
		{name: "ConfigurationMissing", fn: func() *AppError { return ConfigurationMissing("cfg") }},
		{name: "ConfigurationInvalid", fn: func() *AppError { return ConfigurationInvalid("cfg", "reason") }},
		{name: "EnvironmentVariableRequired", fn: func() *AppError { return EnvironmentVariableRequired("VAR") }},
		{name: "DependencyInitializationFailed", fn: func() *AppError { return DependencyInitializationFailed("dep", boom) }},
		{name: "ServiceInitializationFailedGeneric", fn: func() *AppError { return ServiceInitializationFailedGeneric("svc", boom) }},
		{name: "ConnectionFailed", fn: func() *AppError { return ConnectionFailed("conn", boom) }},
		{name: "QuotaExceeded", fn: func() *AppError { return QuotaExceeded("quota", 10) }},
		{name: "RateLimitExceededGeneric", fn: func() *AppError { return RateLimitExceededGeneric("limit") }},
		{name: "TooManyRequests", fn: func() *AppError { return TooManyRequests("resource") }},
		{name: "InvalidStateForOperation", fn: func() *AppError { return InvalidStateForOperation("state", "op") }},
		{name: "ConcurrentModification", fn: func() *AppError { return ConcurrentModification("resource") }},
		{name: "ResourceLocked", fn: func() *AppError { return ResourceLocked("resource", "id") }},
		{name: "DataCorrupted", fn: func() *AppError { return DataCorrupted("type") }},
		{name: "DataInconsistent", fn: func() *AppError { return DataInconsistent("ctx") }},
		{name: "ContentNotAllowed", fn: func() *AppError { return ContentNotAllowed("type", "reason") }},
		{name: "SecurityViolation", fn: func() *AppError { return SecurityViolation("violation") }},
		{name: "AccessDeniedForResource", fn: func() *AppError { return AccessDeniedForResource("resource", "id") }},
		{name: "TamperingDetected", fn: func() *AppError { return TamperingDetected("ctx") }},
		{name: "BusinessRuleViolated", fn: func() *AppError { return BusinessRuleViolated("rule", map[string]interface{}{"k": "v"}) }},
		{name: "PreConditionFailed", fn: func() *AppError { return PreConditionFailed("cond") }},
		{name: "PostConditionFailed", fn: func() *AppError { return PostConditionFailed("cond") }},
		{name: "MultipleErrors", fn: func() *AppError { return MultipleErrors([]error{boom, stdErrors.New("other")}, "op") }},
		{name: "TimelineRequiresField", fn: func() *AppError { return TimelineRequiresField("home", "field") }},
		{name: "UnsupportedTimelineType", fn: func() *AppError { return UnsupportedTimelineType("type") }},
		{name: "RepositoryNotAvailable", fn: func() *AppError { return RepositoryNotAvailable("repo") }},
		{name: "ServiceNotAvailable", fn: func() *AppError { return ServiceNotAvailable("svc") }},
		{name: "ScheduledTimeValidationFailed", fn: func() *AppError { return ScheduledTimeValidationFailed("reason") }},
		{name: "UsernameTaken", fn: func() *AppError { return UsernameTaken("user") }},
		{name: "EmailRequired", fn: func() *AppError { return EmailRequired() }},
		{name: "MustAgreeToTerms", fn: func() *AppError { return MustAgreeToTerms() }},
		{name: "KeypairGenerationFailed", fn: func() *AppError { return KeypairGenerationFailed(boom) }},
		{name: "PublicKeyEncodingFailed", fn: func() *AppError { return PublicKeyEncodingFailed(boom) }},
		{name: "InvalidPrivateKeyType", fn: func() *AppError { return InvalidPrivateKeyType() }},
		{name: "KeyTypeUnsupported", fn: func() *AppError { return KeyTypeUnsupported("k") }},
		{name: "DomainHealthScoreRetrievalFailed", fn: func() *AppError { return DomainHealthScoreRetrievalFailed("d", boom) }},
		{name: "StorageTypeUnsupported", fn: func() *AppError { return StorageTypeUnsupported("s") }},
		{name: "RegistryOptionApplyFailed", fn: func() *AppError { return RegistryOptionApplyFailed(boom) }},
		{name: "RegistryValidationFailed", fn: func() *AppError { return RegistryValidationFailed("reason") }},
		{name: "DatabaseTypeUnsupported", fn: func() *AppError { return DatabaseTypeUnsupported("db") }},
		{name: "NoDatabaseAvailable", fn: func() *AppError { return NoDatabaseAvailable() }},
		{name: "ActorIDFormatInvalid", fn: func() *AppError { return ActorIDFormatInvalid("actor") }},
		{name: "QueueURLNotConfigured", fn: func() *AppError { return QueueURLNotConfigured("q") }},
		{name: "SQSConnectionFailed", fn: func() *AppError { return SQSConnectionFailed(boom) }},
		{name: "MessageMarshalingFailed", fn: func() *AppError { return MessageMarshalingFailed("t", boom) }},
		{name: "SQSMessageSendFailed", fn: func() *AppError { return SQSMessageSendFailed(boom) }},
		{name: "FileSizeExceedsLimit", fn: func() *AppError { return FileSizeExceedsLimit(10, 5) }},
		{name: "ContentTypeNotAllowed", fn: func() *AppError { return ContentTypeNotAllowed("type") }},
		{name: "FormatNotSupported", fn: func() *AppError { return FormatNotSupported("fmt") }},
		{name: "JSONFormatInvalid", fn: func() *AppError { return JSONFormatInvalid("reason") }},
		{name: "CSVValidationFailed", fn: func() *AppError { return CSVValidationFailed("reason") }},
		{name: "FileValidationFailed", fn: func() *AppError { return FileValidationFailed("reason") }},
		{name: "ContentValidationFailed", fn: func() *AppError { return ContentValidationFailed("field", "reason") }},
		{name: "ExpandMediaSettingInvalid", fn: func() *AppError { return ExpandMediaSettingInvalid("setting") }},
		{name: "TimelineOrderInvalid", fn: func() *AppError { return TimelineOrderInvalid("order") }},
		{name: "AccountNoActivityPubActor", fn: func() *AppError { return AccountNoActivityPubActor("user") }},
		{name: "AccountAlreadyPinned", fn: func() *AppError { return AccountAlreadyPinned("user") }},
		{name: "MediaAttachmentValidationFailed", fn: func() *AppError { return MediaAttachmentValidationFailed("reason") }},
		{name: "MediaAttachmentNotReady", fn: func() *AppError { return MediaAttachmentNotReady("mid") }},
		{name: "MediaAttachmentExpired", fn: func() *AppError { return MediaAttachmentExpired("mid") }},
		{name: "DateRangeInvalid", fn: func() *AppError { return DateRangeInvalid("reason") }},
		{name: "MetricUnsupported", fn: func() *AppError { return MetricUnsupported("m") }},
		{name: "InsufficientHistoricalData", fn: func() *AppError { return InsufficientHistoricalData(10) }},
		{name: "AlreadyExists", fn: func() *AppError { return AlreadyExists("item") }},
		{name: "ValidationFailedWithField", fn: func() *AppError { return ValidationFailedWithField("field") }},
		{name: "StreamingConnectionClosed", fn: func() *AppError { return StreamingConnectionClosed("cid", "reason") }},
		{name: "StreamingConnectionTimeout", fn: func() *AppError { return StreamingConnectionTimeout("cid") }},
		{name: "StreamingRecoveryFailed", fn: func() *AppError { return StreamingRecoveryFailed("cid", 1, boom) }},
		{name: "StreamingCircuitBreakerOpen", fn: func() *AppError { return StreamingCircuitBreakerOpen("cid") }},
		{name: "StreamingSyncFailed", fn: func() *AppError { return StreamingSyncFailed("cid", boom) }},
		{name: "StreamingHealthCheckFailed", fn: func() *AppError { return StreamingHealthCheckFailed("cid", boom) }},
		{name: "TransformFunctionNotSet", fn: func() *AppError { return TransformFunctionNotSet() }},
		{name: "TransformItemFailed", fn: func() *AppError { return TransformItemFailed(boom) }},
		{name: "CircuitBreakerOpen", fn: func() *AppError { return CircuitBreakerOpen() }},
		{name: "CircuitBreakerReopened", fn: func() *AppError { return CircuitBreakerReopened() }},
		{name: "HourlyCostLimitExceeded", fn: func() *AppError { return HourlyCostLimitExceeded() }},
		{name: "LoggerRequired", fn: func() *AppError { return LoggerRequired() }},
		{name: "DatabaseRequired", fn: func() *AppError { return DatabaseRequired() }},
		{name: "SNSPublishFailed", fn: func() *AppError { return SNSPublishFailed(boom) }},
		{name: "InvalidPlanTier", fn: func() *AppError { return InvalidPlanTier() }},
		{name: "InvalidFileSize", fn: func() *AppError { return InvalidFileSize() }},
		{name: "VideoDurationInvalid", fn: func() *AppError { return VideoDurationInvalid() }},
		{name: "UploadLimitsInvalid", fn: func() *AppError { return UploadLimitsInvalid() }},
		{name: "BudgetLimitsInvalid", fn: func() *AppError { return BudgetLimitsInvalid() }},
		{name: "ModerationThresholdInvalid", fn: func() *AppError { return ModerationThresholdInvalid() }},
		{name: "InvalidQualitySetting", fn: func() *AppError { return InvalidQualitySetting() }},
		{name: "PlanUpgradeFailed", fn: func() *AppError { return PlanUpgradeFailed(boom) }},
		{name: "UserIDRequired", fn: func() *AppError { return UserIDRequired() }},
		{name: "PatternSaveFailed", fn: func() *AppError { return PatternSaveFailed(boom) }},
		{name: "PatternCreateFailed", fn: func() *AppError { return PatternCreateFailed(boom) }},
		{name: "PatternUpdateFailed", fn: func() *AppError { return PatternUpdateFailed(boom) }},
		{name: "PatternDeleteFailed", fn: func() *AppError { return PatternDeleteFailed(boom) }},
		{name: "PatternQueryFailed", fn: func() *AppError { return PatternQueryFailed(boom) }},
		{name: "PatternCacheCreateFailed", fn: func() *AppError { return PatternCacheCreateFailed(boom) }},
		{name: "PatternCacheUpdateFailed", fn: func() *AppError { return PatternCacheUpdateFailed(boom) }},
		{name: "PatternMetricsCreateFailed", fn: func() *AppError { return PatternMetricsCreateFailed(boom) }},
		{name: "PatternMetricsUpdateFailed", fn: func() *AppError { return PatternMetricsUpdateFailed(boom) }},
		{name: "PatternTestResultCreateFailed", fn: func() *AppError { return PatternTestResultCreateFailed(boom) }},
		{name: "PatternTestResultQueryFailed", fn: func() *AppError { return PatternTestResultQueryFailed(boom) }},
		{name: "PatternTestResultNotFound", fn: func() *AppError { return PatternTestResultNotFound() }},
		{name: "PatternMetricsQueryFailed", fn: func() *AppError { return PatternMetricsQueryFailed(boom) }},
		{name: "PatternAnalysisFailed", fn: func() *AppError { return PatternAnalysisFailed(boom) }},
		{name: "PatternValidationFailed", fn: func() *AppError { return PatternValidationFailed("reason") }},
		{name: "NilPattern", fn: func() *AppError { return NilPattern() }},
		{name: "NilPatternCache", fn: func() *AppError { return NilPatternCache() }},
		{name: "NilPatternMetric", fn: func() *AppError { return NilPatternMetric() }},
		{name: "NilPatternTestResult", fn: func() *AppError { return NilPatternTestResult() }},
	}

	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			assertAppErrorBasics(t, tc.fn())
		})
	}
}

func TestCommonErrors_ClassificationAndWrapping(t *testing.T) {
	t.Run("classification functions", func(t *testing.T) {
		retryable := NewAppError(CodeInternal, CategoryInternal, "x").AsRetryable()
		assert.True(t, IsRetryableError(retryable))

		timeout := NewAppError(CodeTimeout, CategoryExternal, "timeout")
		assert.True(t, IsTemporaryError(timeout))
		assert.False(t, IsTemporaryError(stdErrors.New("plain")))

		assert.True(t, IsClientError(BadRequest("bad")))
		assert.True(t, IsServerError(Internal("oops")))
	})

	t.Run("WrapWithContext and metadata wrappers", func(t *testing.T) {
		assert.Nil(t, WrapWithContext(nil, "ctx"))

		withInternal := Internal("msg").WithInternalMessage("inner")
		wrapped := WrapWithContext(withInternal, "ctx")
		assertAppErrorBasics(t, wrapped)
		assert.Contains(t, wrapped.InternalMessage, "ctx:")

		withoutInternal := Internal("msg")
		wrapped = WrapWithContext(withoutInternal, "ctx")
		assertAppErrorBasics(t, wrapped)
		assert.Equal(t, "ctx", wrapped.InternalMessage)

		plain := stdErrors.New("plain")
		wrapped = WrapWithContext(plain, "ctx")
		assertAppErrorBasics(t, wrapped)

		opWrapped := WrapWithOperation(withInternal, "op")
		assertAppErrorBasics(t, opWrapped)
		assert.Contains(t, opWrapped.Metadata, "operation")

		resWrapped := WrapWithResource(withInternal, "t", "id")
		assertAppErrorBasics(t, resWrapped)
		assert.Contains(t, resWrapped.Metadata, "resource_type")
		assert.Contains(t, resWrapped.Metadata, "resource_id")
	})
}
