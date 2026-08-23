// Package handlers implements API handlers and supporting helpers for the API Lambda function.
package handlers

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Account operations errors - using API/Auth domain functions

// invalidActorIDFormat creates an error when actor ID format is invalid.
func invalidActorIDFormat() *errors.AppError {
	return errors.InvalidFormat("actor_id", "valid actor ID format")
}

// unableToParseRequestBody creates an error when request body parsing fails.
func unableToParseRequestBody() *errors.AppError {
	return errors.ParsingFailed("request body", nil)
}

// missingBoundaryInContentType creates an error when boundary is missing in content type.
func missingBoundaryInContentType() *errors.AppError {
	return errors.FormBoundaryMissing()
}

// invalidContentType creates an error when content type is invalid.
func invalidContentType() *errors.AppError {
	return errors.InvalidFormat("content_type", "valid content type")
}

// unsupportedCollectionType creates an error when collection type is unsupported.
func unsupportedCollectionType() *errors.AppError {
	return errors.InvalidValue("collection_type", []string{"followers", "following", "lists"}, "unknown")
}

// Helper operations errors - using API/Auth domain functions

// invalidAccountURL creates an error when account URL is invalid.
func invalidAccountURL() *errors.AppError {
	return errors.URLInvalid("account URL")
}

// helperUnauthorized creates an error when helper operation is unauthorized.
func helperUnauthorized() *errors.AppError {
	return errors.NewAuthError(errors.CodeUnauthorized, "Unauthorized")
}

// helperInsufficientScope creates an error when helper operation has insufficient scope.
func helperInsufficientScope() *errors.AppError {
	return errors.InsufficientScope("required scope")
}

// invalidAccountID creates an error when account ID is invalid.
func invalidAccountID() *errors.AppError {
	return errors.IDInvalidFormat("account")
}

// failedToInitializeAuthService creates an error when auth service initialization fails.
func failedToInitializeAuthService() *errors.AppError {
	return errors.ServiceInitializationFailedGeneric("auth service", nil)
}

// Import operations errors - using API domain functions

// invalidImportType creates an error when import type is invalid.
func invalidImportType() *errors.AppError {
	return errors.InvalidValue("import_type", []string{"follows", "blocks", "mutes", "bookmarks"}, "unknown")
}

// invalidImportMode creates an error when import mode is invalid.
func invalidImportMode() *errors.AppError {
	return errors.InvalidValue("mode", []string{"merge", "overwrite"}, "unknown")
}

// unsupportedFileFormat creates an error when file format is unsupported.
func unsupportedFileFormat() *errors.AppError {
	return errors.NewAppError(errors.CodeUnsupportedMediaType, errors.CategoryAPI, "Unsupported file format")
}

// jobQueueServiceCreationFailed creates an error when job queue service creation fails.
func jobQueueServiceCreationFailed() *errors.AppError {
	return errors.ServiceInitializationFailedGeneric("job queue service", nil)
}

// App operations errors - using Validation domain functions

// unableToParseRequestBodyAsFormOrJSON creates an error when request body cannot be parsed as form data or JSON.
func unableToParseRequestBodyAsFormOrJSON() *errors.AppError {
	return errors.ParsingFailed("request body as form data or JSON", nil)
}

// invalidRedirectURIFormat creates an error when redirect URI format is invalid.
func invalidRedirectURIFormat() *errors.AppError {
	return errors.OAuthRedirectURIInvalid("redirect URI")
}

// failedToParseMultipartForm creates an error when multipart form parsing fails.
func failedToParseMultipartForm() *errors.AppError {
	return errors.ParsingFailed("multipart form", nil)
}

// failedToParseFormData creates an error when form data parsing fails.
func failedToParseFormData() *errors.AppError {
	return errors.ParsingFailed("form data", nil)
}

// Media operations errors - using Validation domain functions

// emptyRequestBody creates an error when request body is empty.
func emptyRequestBody() *errors.AppError {
	return errors.ContentEmpty("request body")
}

// noFileDataFoundInRequest creates an error when no file data is found in request.
func noFileDataFoundInRequest() *errors.AppError {
	return errors.FormFieldMissing("file")
}

// failedToExtractBoundary creates an error when boundary extraction fails.
func failedToExtractBoundary() *errors.AppError {
	return errors.FormBoundaryMissing()
}

// Search operations errors - using API domain functions

// searchFailed creates an error when search operation fails.
func searchFailed() *errors.AppError {
	return errors.ProcessingFailed("search", stdErrors.New("search operation failed"))
}

// privacyAwareSearchFailed creates an error when privacy-aware search fails.
func privacyAwareSearchFailed() *errors.AppError {
	return errors.ProcessingFailed("privacy-aware search", stdErrors.New("privacy-aware search failed"))
}

// statusSearchFailed creates an error when status search fails.
func statusSearchFailed() *errors.AppError {
	return errors.ProcessingFailed("status search", stdErrors.New("status search failed"))
}

// privacyAwareStatusSearchFailed creates an error when privacy-aware status search fails.
func privacyAwareStatusSearchFailed() *errors.AppError {
	return errors.ProcessingFailed("privacy-aware status search", stdErrors.New("privacy-aware status search failed"))
}

// OAuth operations errors - using Auth domain functions

// failedToGenerateTokens creates an error when token generation fails.
func failedToGenerateTokens() *errors.AppError {
	return errors.TokenGenerationFailed(nil)
}

// VAPID key operations errors - using Auth domain functions

// failedToGenerateVAPIDPrivateKey creates an error when VAPID private key generation fails.
func failedToGenerateVAPIDPrivateKey() *errors.AppError {
	return errors.TokenGenerationFailed(nil)
}

// failedToConvertToECDHKey creates an error when ECDH key conversion fails.
func failedToConvertToECDHKey() *errors.AppError {
	return errors.ProcessingFailed("ECDH key conversion", stdErrors.New("failed to convert to ECDH key"))
}

// failedToStoreVAPIDKeys creates an error when VAPID key storage fails.
func failedToStoreVAPIDKeys() *errors.AppError {
	return errors.CredentialStorageFailed(nil)
}

// Export operations errors - using API domain functions

// failedToCreateJobQueueService creates an error when job queue service creation fails for export.
func failedToCreateJobQueueService() *errors.AppError {
	return errors.ServiceInitializationFailedGeneric("job queue service", nil)
}

// invalidStartDate creates an error when start date is invalid.
func invalidStartDate() *errors.AppError {
	return errors.TimestampInvalidFormat("start date")
}

// invalidEndDate creates an error when end date is invalid.
func invalidEndDate() *errors.AppError {
	return errors.TimestampInvalidFormat("end date")
}

// Follow request operations errors - using API domain functions

// failedToGetFollowerActor creates an error when follower actor retrieval fails.
func failedToGetFollowerActor() *errors.AppError {
	return errors.FailedToGet("follower actor", nil)
}

// failedToGetFollowedActor creates an error when followed actor retrieval fails.
func failedToGetFollowedActor() *errors.AppError {
	return errors.FailedToGet("followed actor", nil)
}

// Translation operations errors - using Validation domain functions

// invalidSourceLanguageCode creates an error when source language code is invalid.
func invalidSourceLanguageCode() *errors.AppError {
	return errors.StatusLanguageInvalid("source language")
}

// invalidTargetLanguageCode creates an error when target language code is invalid.
func invalidTargetLanguageCode() *errors.AppError {
	return errors.StatusLanguageInvalid("target language")
}

// WebSocket cost analytics operations errors - using API domain functions

// failedToGetCostRecords creates an error when cost record retrieval fails.
func failedToGetCostRecords() *errors.AppError {
	return errors.FailedToGet("cost records", nil)
}

// Status conversion operations errors - using API domain functions

// failedToConvertStatus creates an error when status conversion fails.
func failedToConvertStatus() *errors.AppError {
	return errors.ProcessingFailed("status conversion", stdErrors.New("failed to convert status"))
}
