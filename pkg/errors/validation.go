package errors

// Validation domain errors
// Consolidates input validation errors from across the application

// NewValidationError creates a new validation error for the specified field and message.
func NewValidationError(field, message string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, message).
		WithMetadata("field", field)
}

// NewValidationErrorWithCode creates a validation error with a specific error code for the specified field.
func NewValidationErrorWithCode(code ErrorCode, field, message string) *AppError {
	return NewAppError(code, CategoryValidation, message).
		WithMetadata("field", field)
}

// RequiredFieldMissing creates an error indicating a required field is missing or empty.
func RequiredFieldMissing(field string) *AppError {
	return NewValidationErrorWithCode(CodeRequiredFieldMissing, field, "Required field is missing or empty")
}

// FieldTooLong creates an error indicating a field exceeds maximum length.
func FieldTooLong(field string, maxLength int, actualLength int) *AppError {
	return NewValidationErrorWithCode(CodeFieldTooLong, field, "Field exceeds maximum length").
		WithMetadata("max_length", maxLength).
		WithMetadata("actual_length", actualLength)
}

// FieldTooShort creates an error indicating a field is below minimum length.
func FieldTooShort(field string, minLength int, actualLength int) *AppError {
	return NewValidationErrorWithCode(CodeFieldTooShort, field, "Field is below minimum length").
		WithMetadata("min_length", minLength).
		WithMetadata("actual_length", actualLength)
}

// InvalidFormat creates an error indicating a field has an invalid format.
func InvalidFormat(field, expectedFormat string) *AppError {
	return NewValidationErrorWithCode(CodeInvalidFormat, field, "Field has invalid format").
		WithMetadata("expected_format", expectedFormat)
}

// InvalidCharacters creates an error indicating a field contains invalid characters.
func InvalidCharacters(field, allowedChars string) *AppError {
	return NewValidationErrorWithCode(CodeInvalidCharacters, field, "Field contains invalid characters").
		WithMetadata("allowed_characters", allowedChars)
}

// ValueOutOfRange creates an error indicating a value is outside the allowed range.
func ValueOutOfRange(field string, minVal, maxVal, actual interface{}) *AppError {
	return NewValidationErrorWithCode(CodeValueOutOfRange, field, "Value is outside allowed range").
		WithMetadata("min", minVal).
		WithMetadata("max", maxVal).
		WithMetadata("actual", actual)
}

// InvalidValue creates an error indicating an invalid value that is not in the allowed values list.
func InvalidValue(field string, allowedValues []string, actual string) *AppError {
	return NewValidationErrorWithCode(CodeInvalidInput, field, "Invalid value").
		WithMetadata("allowed_values", allowedValues).
		WithMetadata("actual", actual)
}

// ContentTooLong creates an error indicating content exceeds maximum length.
func ContentTooLong(contentType string, maxLength int) *AppError {
	return NewValidationErrorWithCode(CodeContentTooLarge, "content", "Content exceeds maximum length").
		WithMetadata("content_type", contentType).
		WithMetadata("max_length", maxLength)
}

// ContentEmpty creates an error indicating content cannot be empty.
func ContentEmpty(contentType string) *AppError {
	return NewValidationError("content", "Content cannot be empty").
		WithMetadata("content_type", contentType)
}

// ContentContainsForbiddenWord creates an error indicating content contains a forbidden word.
func ContentContainsForbiddenWord(word string) *AppError {
	return NewValidationError("content", "Content contains forbidden word").
		WithMetadata("forbidden_word", word)
}

// ContentMustHaveContentOrMedia creates an error indicating content must have either text content or media attachments.
func ContentMustHaveContentOrMedia() *AppError {
	return NewValidationError("content", "Must have content or media attachments")
}

// UsernameEmpty creates an error indicating username cannot be empty.
func UsernameEmpty() *AppError {
	return NewValidationError("username", "Username cannot be empty")
}

// UsernameInvalidLength creates an error indicating username length is invalid.
func UsernameInvalidLength(minVal, maxVal int) *AppError {
	return NewValidationError("username", "Username length is invalid").
		WithMetadata("min_length", minVal).
		WithMetadata("max_length", maxVal)
}

// UsernameInvalidCharacters creates an error indicating username contains invalid characters.
func UsernameInvalidCharacters() *AppError {
	return NewValidationError("username", "Username contains invalid characters")
}

// UsernameInvalidFormat creates an error indicating username format is invalid.
func UsernameInvalidFormat() *AppError {
	return NewValidationError("username", "Username format is invalid")
}

// UsernameConsecutiveUnderscores creates an error indicating username cannot contain consecutive underscores.
func UsernameConsecutiveUnderscores() *AppError {
	return NewValidationError("username", "Username cannot contain consecutive underscores")
}

// UsernameStartsOrEndsWithUnderscore creates an error indicating username cannot start or end with underscore.
func UsernameStartsOrEndsWithUnderscore() *AppError {
	return NewValidationError("username", "Username cannot start or end with underscore")
}

// EmailEmpty creates an error indicating email address is required.
func EmailEmpty() *AppError {
	return NewValidationError("email", "Email address is required")
}

// EmailInvalidFormat creates an error indicating email address format is invalid.
func EmailInvalidFormat() *AppError {
	return NewValidationError("email", "Email address format is invalid")
}

// EmailTooLong creates an error indicating email address is too long.
func EmailTooLong() *AppError {
	return NewValidationError("email", "Email address is too long")
}

// EmailDomainInvalid creates an error indicating email domain is invalid.
func EmailDomainInvalid() *AppError {
	return NewValidationError("email", "Email domain is invalid")
}

// DisplayNameTooLong creates an error indicating display name is too long.
func DisplayNameTooLong(maxLength int) *AppError {
	return NewValidationErrorWithCode(CodeFieldTooLong, "display_name", "Display name is too long").
		WithMetadata("max_length", maxLength)
}

// BioTooLong creates an error indicating bio is too long.
func BioTooLong(maxLength int) *AppError {
	return NewValidationErrorWithCode(CodeFieldTooLong, "bio", "Bio is too long").
		WithMetadata("max_length", maxLength)
}

// IDEmpty creates an error indicating ID cannot be empty.
func IDEmpty(idType string) *AppError {
	return NewValidationError("id", "ID cannot be empty").
		WithMetadata("id_type", idType)
}

// IDInvalidFormat creates an error indicating ID format is invalid.
func IDInvalidFormat(idType string) *AppError {
	return NewValidationError("id", "ID format is invalid").
		WithMetadata("id_type", idType)
}

// IDTooLong creates an error indicating ID is too long.
func IDTooLong(idType string, maxLength int) *AppError {
	return NewValidationError("id", "ID is too long").
		WithMetadata("id_type", idType).
		WithMetadata("max_length", maxLength)
}

// StatusTooManyMedia creates an error indicating too many media attachments.
func StatusTooManyMedia(maxCount int, actualCount int) *AppError {
	return NewValidationError("media", "Too many media attachments").
		WithMetadata("max_count", maxCount).
		WithMetadata("actual_count", actualCount)
}

// StatusInvalidVisibility creates an error indicating invalid visibility setting.
func StatusInvalidVisibility(visibility string) *AppError {
	return NewValidationError("visibility", "Invalid visibility setting").
		WithMetadata("visibility", visibility)
}

// StatusSpoilerTextTooLong creates an error indicating spoiler text is too long.
func StatusSpoilerTextTooLong(maxLength int) *AppError {
	return NewValidationError("spoiler_text", "Spoiler text is too long").
		WithMetadata("max_length", maxLength)
}

// StatusLanguageInvalid creates an error indicating invalid language code.
func StatusLanguageInvalid(language string) *AppError {
	return NewValidationError("language", "Invalid language code").
		WithMetadata("language", language)
}

// PollTooFewOptions creates an error indicating poll has too few options.
func PollTooFewOptions(minOptions int) *AppError {
	return NewValidationError("poll_options", "Poll has too few options").
		WithMetadata("min_options", minOptions)
}

// PollTooManyOptions creates an error indicating poll has too many options.
func PollTooManyOptions(maxOptions int, actualCount int) *AppError {
	return NewValidationError("poll_options", "Poll has too many options").
		WithMetadata("max_options", maxOptions).
		WithMetadata("actual_count", actualCount)
}

// PollOptionEmpty creates an error indicating poll option cannot be empty.
func PollOptionEmpty() *AppError {
	return NewValidationError("poll_option", "Poll option cannot be empty")
}

// PollOptionTooLong creates an error indicating poll option is too long.
func PollOptionTooLong(maxLength int) *AppError {
	return NewValidationError("poll_option", "Poll option is too long").
		WithMetadata("max_length", maxLength)
}

// PollExpiryInvalid creates an error indicating poll expiry time is invalid.
func PollExpiryInvalid() *AppError {
	return NewValidationError("expires_in", "Poll expiry time is invalid")
}

// PollExpiryTooShort creates an error indicating poll expiry time is too short.
func PollExpiryTooShort(minSeconds int) *AppError {
	return NewValidationError("expires_in", "Poll expiry time is too short").
		WithMetadata("min_seconds", minSeconds)
}

// PollExpiryTooLong creates an error indicating poll expiry time is too long.
func PollExpiryTooLong(maxSeconds int) *AppError {
	return NewValidationError("expires_in", "Poll expiry time is too long").
		WithMetadata("max_seconds", maxSeconds)
}

// PollMultipleChoiceInvalid creates an error indicating invalid multiple choice setting.
func PollMultipleChoiceInvalid() *AppError {
	return NewValidationError("multiple", "Invalid multiple choice setting")
}

// MediaFileTooLarge creates an error indicating media file size exceeds limit.
func MediaFileTooLarge(fileSize int64, maxSize int64) *AppError {
	return NewValidationError("media", "Media file size exceeds limit").
		WithMetadata("file_size", fileSize).
		WithMetadata("max_size", maxSize)
}

// MediaInvalidMimeType creates an error indicating invalid media MIME type.
func MediaInvalidMimeType(mimeType string, allowedTypes []string) *AppError {
	return NewValidationError("media", "Invalid media MIME type").
		WithMetadata("mime_type", mimeType).
		WithMetadata("allowed_types", allowedTypes)
}

// ImageInvalidFormat creates an error indicating invalid image format.
func ImageInvalidFormat(format string) *AppError {
	return NewValidationError("image", "Invalid image format").
		WithMetadata("format", format)
}

// VideoInvalidFormat creates an error indicating invalid video format.
func VideoInvalidFormat(format string) *AppError {
	return NewValidationError("video", "Invalid video format").
		WithMetadata("format", format)
}

// AudioInvalidFormat creates an error indicating invalid audio format.
func AudioInvalidFormat(format string) *AppError {
	return NewValidationError("audio", "Invalid audio format").
		WithMetadata("format", format)
}

// VideoFileTooLarge creates an error indicating video file size exceeds limit.
func VideoFileTooLarge(fileSize int64, maxSize int64) *AppError {
	return NewValidationError("video", "Video file size exceeds limit").
		WithMetadata("file_size", fileSize).
		WithMetadata("max_size", maxSize)
}

// MediaDescriptionTooLong creates an error indicating media description is too long.
func MediaDescriptionTooLong(maxLength int) *AppError {
	return NewValidationError("description", "Media description is too long").
		WithMetadata("max_length", maxLength)
}

// FilterKeywordEmpty creates an error indicating filter keyword cannot be empty.
func FilterKeywordEmpty() *AppError {
	return NewValidationError("keyword", "Filter keyword cannot be empty")
}

// FilterKeywordTooLong creates an error indicating filter keyword is too long.
func FilterKeywordTooLong(maxLength int) *AppError {
	return NewValidationError("keyword", "Filter keyword is too long").
		WithMetadata("max_length", maxLength)
}

// FilterContextInvalid creates an error indicating invalid filter context.
func FilterContextInvalid(context string) *AppError {
	return NewValidationError("context", "Invalid filter context").
		WithMetadata("context", context)
}

// FilterActionInvalid creates an error indicating invalid filter action.
func FilterActionInvalid(action string) *AppError {
	return NewValidationError("filter_action", "Invalid filter action").
		WithMetadata("action", action)
}

// ListTitleEmpty creates an error indicating list title cannot be empty.
func ListTitleEmpty() *AppError {
	return NewValidationError("title", "List title cannot be empty")
}

// ListTitleTooLong creates an error indicating list title is too long.
func ListTitleTooLong(maxLength int) *AppError {
	return NewValidationError("title", "List title is too long").
		WithMetadata("max_length", maxLength)
}

// ListRepliesPolicyInvalid creates an error indicating invalid list replies policy.
func ListRepliesPolicyInvalid(policy string) *AppError {
	return NewValidationError("replies_policy", "Invalid list replies policy").
		WithMetadata("policy", policy)
}

// OAuthScopeInvalid creates an error indicating invalid OAuth scope.
func OAuthScopeInvalid(scope string) *AppError {
	return NewValidationError("scope", "Invalid OAuth scope").
		WithMetadata("scope", scope)
}

// OAuthRedirectURIInvalid creates an error indicating invalid OAuth redirect URI.
func OAuthRedirectURIInvalid(uri string) *AppError {
	return NewValidationError("redirect_uri", "Invalid OAuth redirect URI").
		WithMetadata("uri", uri)
}

// OAuthClientNameEmpty creates an error indicating OAuth client name is required.
func OAuthClientNameEmpty() *AppError {
	return NewValidationError("client_name", "OAuth client name is required")
}

// OAuthResponseTypeInvalid creates an error indicating invalid OAuth response type.
func OAuthResponseTypeInvalid(responseType string) *AppError {
	return NewValidationError("response_type", "Invalid OAuth response type").
		WithMetadata("response_type", responseType)
}

// OAuthGrantTypeInvalid creates an error indicating invalid OAuth grant type.
func OAuthGrantTypeInvalid(grantType string) *AppError {
	return NewValidationError("grant_type", "Invalid OAuth grant type").
		WithMetadata("grant_type", grantType)
}

// NotificationTypeInvalid creates an error indicating invalid notification type.
func NotificationTypeInvalid(notificationType string) *AppError {
	return NewValidationError("type", "Invalid notification type").
		WithMetadata("type", notificationType)
}

// TimestampInvalidFormat creates an error indicating invalid timestamp format.
func TimestampInvalidFormat(timestamp string) *AppError {
	return NewValidationError("timestamp", "Invalid timestamp format").
		WithMetadata("timestamp", timestamp)
}

// TimestampInFuture creates an error indicating timestamp cannot be in the future.
func TimestampInFuture() *AppError {
	return NewValidationError("timestamp", "Timestamp cannot be in the future")
}

// TimestampTooOld creates an error indicating timestamp is too old.
func TimestampTooOld(maxAge string) *AppError {
	return NewValidationError("timestamp", "Timestamp is too old").
		WithMetadata("max_age", maxAge)
}

// URLInvalid creates an error indicating invalid URL format.
func URLInvalid(url string) *AppError {
	return NewValidationError("url", "Invalid URL format").
		WithMetadata("url", url)
}

// URLSchemeNotAllowed creates an error indicating URL scheme not allowed.
func URLSchemeNotAllowed(url, scheme string) *AppError {
	return NewValidationError("url", "URL scheme not allowed").
		WithMetadata("url", url).
		WithMetadata("scheme", scheme)
}

// URLHostNotAllowed creates an error indicating URL host not allowed.
func URLHostNotAllowed(url, host string) *AppError {
	return NewValidationError("url", "URL host not allowed").
		WithMetadata("url", url).
		WithMetadata("host", host)
}

// JSONInvalid creates an error indicating invalid JSON structure.
func JSONInvalid(reason string) *AppError {
	return NewValidationError("json", "Invalid JSON structure").
		WithMetadata("reason", reason)
}

// JSONTooDeep creates an error indicating JSON nesting too deep.
func JSONTooDeep(maxDepth int) *AppError {
	return NewValidationError("json", "JSON nesting too deep").
		WithMetadata("max_depth", maxDepth)
}

// JSONTooManyKeys creates an error indicating JSON object has too many keys.
func JSONTooManyKeys(maxKeys int) *AppError {
	return NewValidationError("json", "JSON object has too many keys").
		WithMetadata("max_keys", maxKeys)
}

// JSONKeyTooLong creates an error indicating JSON key is too long.
func JSONKeyTooLong(maxLength int) *AppError {
	return NewValidationError("json", "JSON key is too long").
		WithMetadata("max_length", maxLength)
}

// JSONStringTooLong creates an error indicating JSON string is too long.
func JSONStringTooLong(maxLength int) *AppError {
	return NewValidationError("json", "JSON string is too long").
		WithMetadata("max_length", maxLength)
}

// JSONArrayTooLarge creates an error indicating JSON array has too many elements.
func JSONArrayTooLarge(maxElements int) *AppError {
	return NewValidationError("json", "JSON array has too many elements").
		WithMetadata("max_elements", maxElements)
}

// JSONSizeTooLarge creates an error indicating JSON size exceeds maximum.
func JSONSizeTooLarge(maxSize int64) *AppError {
	return NewValidationError("json", "JSON size exceeds maximum").
		WithMetadata("max_size", maxSize)
}

// JSONBombDetected creates an error indicating possible JSON bomb detected.
func JSONBombDetected(reason string) *AppError {
	return NewValidationError("json", "Possible JSON bomb detected").
		WithMetadata("reason", reason)
}

// ActivityPubActorURIEmpty creates an error indicating actor URI cannot be empty.
func ActivityPubActorURIEmpty() *AppError {
	return NewValidationError("actor", "Actor URI cannot be empty")
}

// ActivityPubActorURIMustUseHTTPS creates an error indicating actor URI must use HTTPS.
func ActivityPubActorURIMustUseHTTPS() *AppError {
	return NewValidationError("actor", "Actor URI must use HTTPS")
}

// ActivityPubActivityTypeEmpty creates an error indicating activity type cannot be empty.
func ActivityPubActivityTypeEmpty() *AppError {
	return NewValidationError("type", "Activity type cannot be empty")
}

// ActivityPubSignatureHeaderEmpty creates an error indicating signature header is empty.
func ActivityPubSignatureHeaderEmpty() *AppError {
	return NewValidationError("signature", "Signature header is empty")
}

// ActivityPubInvalidSignature creates an error indicating invalid signature missing keyId or signature.
func ActivityPubInvalidSignature() *AppError {
	return NewValidationError("signature", "Invalid signature: missing keyId or signature")
}

// ActivityPubUnsupportedActivityType creates an error indicating unsupported ActivityPub activity type.
func ActivityPubUnsupportedActivityType(activityType string) *AppError {
	return NewValidationError("type", "Unsupported ActivityPub activity type").
		WithMetadata("activity_type", activityType)
}

// FormBoundaryMissing creates an error indicating no boundary found in multipart content type.
func FormBoundaryMissing() *AppError {
	return NewValidationError("content_type", "No boundary found in multipart content type")
}

// FormFieldMissing creates an error indicating required form field is missing.
func FormFieldMissing(field string) *AppError {
	return NewValidationError(field, "Required form field is missing")
}

// FormFieldInvalid creates an error indicating form field is invalid.
func FormFieldInvalid(field, reason string) *AppError {
	return NewValidationError(field, "Form field is invalid").
		WithMetadata("reason", reason)
}

// MultipleValidationErrors creates an error indicating multiple validation errors occurred.
func MultipleValidationErrors(errors []string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, "Multiple validation errors").
		WithMetadata("errors", errors)
}
