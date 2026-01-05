package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidationConstructors_ExhaustiveCoverage(t *testing.T) {
	constructors := []struct {
		name string
		fn   func() *AppError
	}{
		{name: "NewValidationError", fn: func() *AppError { return NewValidationError("field", "msg") }},
		{name: "NewValidationErrorWithCode", fn: func() *AppError { return NewValidationErrorWithCode(CodeInvalidInput, "field", "msg") }},
		{name: "RequiredFieldMissing", fn: func() *AppError { return RequiredFieldMissing("field") }},
		{name: "FieldTooLong", fn: func() *AppError { return FieldTooLong("field", 10, 11) }},
		{name: "FieldTooShort", fn: func() *AppError { return FieldTooShort("field", 3, 2) }},
		{name: "InvalidFormat", fn: func() *AppError { return InvalidFormat("field", "expected") }},
		{name: "InvalidCharacters", fn: func() *AppError { return InvalidCharacters("field", "abc") }},
		{name: "ValueOutOfRange", fn: func() *AppError { return ValueOutOfRange("field", 1, 2, 3) }},
		{name: "InvalidValue", fn: func() *AppError { return InvalidValue("field", []string{"a", "b"}, "c") }},
		{name: "ContentTooLong", fn: func() *AppError { return ContentTooLong("content", 10) }},
		{name: "ContentEmpty", fn: func() *AppError { return ContentEmpty("content") }},
		{name: "ContentContainsForbiddenWord", fn: func() *AppError { return ContentContainsForbiddenWord("word") }},
		{name: "ContentMustHaveContentOrMedia", fn: func() *AppError { return ContentMustHaveContentOrMedia() }},
		{name: "UsernameEmpty", fn: func() *AppError { return UsernameEmpty() }},
		{name: "UsernameInvalidLength", fn: func() *AppError { return UsernameInvalidLength(1, 2) }},
		{name: "UsernameInvalidCharacters", fn: func() *AppError { return UsernameInvalidCharacters() }},
		{name: "UsernameInvalidFormat", fn: func() *AppError { return UsernameInvalidFormat() }},
		{name: "UsernameConsecutiveUnderscores", fn: func() *AppError { return UsernameConsecutiveUnderscores() }},
		{name: "UsernameStartsOrEndsWithUnderscore", fn: func() *AppError { return UsernameStartsOrEndsWithUnderscore() }},
		{name: "EmailEmpty", fn: func() *AppError { return EmailEmpty() }},
		{name: "EmailInvalidFormat", fn: func() *AppError { return EmailInvalidFormat() }},
		{name: "EmailTooLong", fn: func() *AppError { return EmailTooLong() }},
		{name: "EmailDomainInvalid", fn: func() *AppError { return EmailDomainInvalid() }},
		{name: "DisplayNameTooLong", fn: func() *AppError { return DisplayNameTooLong(30) }},
		{name: "BioTooLong", fn: func() *AppError { return BioTooLong(500) }},
		{name: "IDEmpty", fn: func() *AppError { return IDEmpty("id") }},
		{name: "IDInvalidFormat", fn: func() *AppError { return IDInvalidFormat("id") }},
		{name: "IDTooLong", fn: func() *AppError { return IDTooLong("id", 500) }},
		{name: "StatusTooManyMedia", fn: func() *AppError { return StatusTooManyMedia(4, 5) }},
		{name: "StatusInvalidVisibility", fn: func() *AppError { return StatusInvalidVisibility("nope") }},
		{name: "StatusSpoilerTextTooLong", fn: func() *AppError { return StatusSpoilerTextTooLong(500) }},
		{name: "StatusLanguageInvalid", fn: func() *AppError { return StatusLanguageInvalid("xx") }},
		{name: "PollTooFewOptions", fn: func() *AppError { return PollTooFewOptions(2) }},
		{name: "PollTooManyOptions", fn: func() *AppError { return PollTooManyOptions(4, 5) }},
		{name: "PollOptionEmpty", fn: func() *AppError { return PollOptionEmpty() }},
		{name: "PollOptionTooLong", fn: func() *AppError { return PollOptionTooLong(50) }},
		{name: "PollExpiryInvalid", fn: func() *AppError { return PollExpiryInvalid() }},
		{name: "PollExpiryTooShort", fn: func() *AppError { return PollExpiryTooShort(10) }},
		{name: "PollExpiryTooLong", fn: func() *AppError { return PollExpiryTooLong(1000) }},
		{name: "PollMultipleChoiceInvalid", fn: func() *AppError { return PollMultipleChoiceInvalid() }},
		{name: "MediaFileTooLarge", fn: func() *AppError { return MediaFileTooLarge(10, 5) }},
		{name: "MediaInvalidMimeType", fn: func() *AppError { return MediaInvalidMimeType("image/unknown", []string{"image/png"}) }},
		{name: "ImageInvalidFormat", fn: func() *AppError { return ImageInvalidFormat("gif") }},
		{name: "VideoInvalidFormat", fn: func() *AppError { return VideoInvalidFormat("mov") }},
		{name: "AudioInvalidFormat", fn: func() *AppError { return AudioInvalidFormat("wav") }},
		{name: "VideoFileTooLarge", fn: func() *AppError { return VideoFileTooLarge(10, 5) }},
		{name: "MediaDescriptionTooLong", fn: func() *AppError { return MediaDescriptionTooLong(500) }},
		{name: "FilterKeywordEmpty", fn: func() *AppError { return FilterKeywordEmpty() }},
		{name: "FilterKeywordTooLong", fn: func() *AppError { return FilterKeywordTooLong(50) }},
		{name: "FilterContextInvalid", fn: func() *AppError { return FilterContextInvalid("nope") }},
		{name: "FilterActionInvalid", fn: func() *AppError { return FilterActionInvalid("nope") }},
		{name: "ListTitleEmpty", fn: func() *AppError { return ListTitleEmpty() }},
		{name: "ListTitleTooLong", fn: func() *AppError { return ListTitleTooLong(50) }},
		{name: "ListRepliesPolicyInvalid", fn: func() *AppError { return ListRepliesPolicyInvalid("nope") }},
		{name: "OAuthScopeInvalid", fn: func() *AppError { return OAuthScopeInvalid("nope") }},
		{name: "OAuthRedirectURIInvalid", fn: func() *AppError { return OAuthRedirectURIInvalid("nope") }},
		{name: "OAuthClientNameEmpty", fn: func() *AppError { return OAuthClientNameEmpty() }},
		{name: "OAuthResponseTypeInvalid", fn: func() *AppError { return OAuthResponseTypeInvalid("nope") }},
		{name: "OAuthGrantTypeInvalid", fn: func() *AppError { return OAuthGrantTypeInvalid("nope") }},
		{name: "NotificationTypeInvalid", fn: func() *AppError { return NotificationTypeInvalid("nope") }},
		{name: "TimestampInvalidFormat", fn: func() *AppError { return TimestampInvalidFormat("nope") }},
		{name: "TimestampInFuture", fn: func() *AppError { return TimestampInFuture() }},
		{name: "TimestampTooOld", fn: func() *AppError { return TimestampTooOld("1h") }},
		{name: "URLInvalid", fn: func() *AppError { return URLInvalid("nope") }},
		{name: "URLSchemeNotAllowed", fn: func() *AppError { return URLSchemeNotAllowed("x", "ftp") }},
		{name: "URLHostNotAllowed", fn: func() *AppError { return URLHostNotAllowed("x", "host") }},
		{name: "JSONInvalid", fn: func() *AppError { return JSONInvalid("nope") }},
		{name: "JSONTooDeep", fn: func() *AppError { return JSONTooDeep(10) }},
		{name: "JSONTooManyKeys", fn: func() *AppError { return JSONTooManyKeys(10) }},
		{name: "JSONKeyTooLong", fn: func() *AppError { return JSONKeyTooLong(10) }},
		{name: "JSONStringTooLong", fn: func() *AppError { return JSONStringTooLong(10) }},
		{name: "JSONArrayTooLarge", fn: func() *AppError { return JSONArrayTooLarge(10) }},
		{name: "JSONSizeTooLarge", fn: func() *AppError { return JSONSizeTooLarge(10) }},
		{name: "JSONBombDetected", fn: func() *AppError { return JSONBombDetected("nope") }},
		{name: "ActivityPubActorURIEmpty", fn: func() *AppError { return ActivityPubActorURIEmpty() }},
		{name: "ActivityPubActorURIMustUseHTTPS", fn: func() *AppError { return ActivityPubActorURIMustUseHTTPS() }},
		{name: "ActivityPubActivityTypeEmpty", fn: func() *AppError { return ActivityPubActivityTypeEmpty() }},
		{name: "ActivityPubSignatureHeaderEmpty", fn: func() *AppError { return ActivityPubSignatureHeaderEmpty() }},
		{name: "ActivityPubInvalidSignature", fn: func() *AppError { return ActivityPubInvalidSignature() }},
		{name: "ActivityPubUnsupportedActivityType", fn: func() *AppError { return ActivityPubUnsupportedActivityType("Create") }},
		{name: "FormBoundaryMissing", fn: func() *AppError { return FormBoundaryMissing() }},
		{name: "FormFieldMissing", fn: func() *AppError { return FormFieldMissing("field") }},
		{name: "FormFieldInvalid", fn: func() *AppError { return FormFieldInvalid("field", "reason") }},
		{name: "MultipleValidationErrors", fn: func() *AppError { return MultipleValidationErrors([]string{"a", "b"}) }},
	}

	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			assertAppErrorBasics(t, err)
			assert.Equal(t, CategoryValidation, err.Category)
		})
	}
}
