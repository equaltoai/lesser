package main

import "github.com/equaltoai/lesser/pkg/errors"

// Error functions for moderation processor

// Record format validation errors

// ErrInvalidReviewPKFormat returns an error indicating invalid review PK format.
func ErrInvalidReviewPKFormat(pk string) error {
	return errors.NewAppError(errors.CodeInvalidFormat, errors.CategoryValidation, "invalid review PK format").
		WithMetadata("pk", pk)
}

// ErrInvalidReviewSKFormat returns an error indicating invalid review SK format.
func ErrInvalidReviewSKFormat(sk string) error {
	return errors.NewAppError(errors.CodeInvalidFormat, errors.CategoryValidation, "invalid review SK format").
		WithMetadata("sk", sk)
}

// ErrNotReviewRecord returns an error indicating record is not a review record.
func ErrNotReviewRecord() error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "not a review record")
}

// ErrNotEventRecord returns an error indicating record is not an event record.
func ErrNotEventRecord() error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "not an event record")
}

// ErrNotDecisionRecord returns an error indicating record is not a decision record.
func ErrNotDecisionRecord() error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "not a decision record")
}

// Action and processing errors

// ErrUnknownActionType returns an error indicating unknown action type.
func ErrUnknownActionType(actionType string) error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "unknown action type").
		WithMetadata("action_type", actionType)
}

// Moderator and admin errors

// ErrFailedToRetrieveModerators returns an error indicating failed to retrieve moderators and admins.
func ErrFailedToRetrieveModerators(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to retrieve both moderators and admins from storage", err)
}

// ErrNoAdminsAvailableForFallback returns an error indicating no admins available for fallback.
func ErrNoAdminsAvailableForFallback() error {
	return errors.NewAppError(errors.CodeNotFound, errors.CategoryValidation, "no admins available for fallback")
}

// Enforcement errors

// ErrEnforcementFailed returns an error indicating enforcement failed.
func ErrEnforcementFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "enforcement failed", err)
}

// ErrContentRemovalFailed returns an error indicating content removal enforcement failed.
func ErrContentRemovalFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "content removal enforcement failed", err)
}

// ErrTimelineFilteringFailed returns an error indicating timeline filtering had errors.
func ErrTimelineFilteringFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "timeline filtering had errors", err)
}

// Extraction and processing errors

// ErrFailedToExtractReview returns an error indicating failed to extract review.
func ErrFailedToExtractReview(err error) error {
	return errors.NewLambdaInternalError(errors.CodeEventProcessingFailed, "failed to extract review", err)
}

// ErrFailedToExtractEvent returns an error indicating failed to extract event.
func ErrFailedToExtractEvent(err error) error {
	return errors.NewLambdaInternalError(errors.CodeEventProcessingFailed, "failed to extract event", err)
}

// ErrFailedToExtractDecision returns an error indicating failed to extract decision.
func ErrFailedToExtractDecision(err error) error {
	return errors.NewLambdaInternalError(errors.CodeEventProcessingFailed, "failed to extract decision", err)
}

// Moderator selection errors

// ErrFailedToGetAvailableModerators returns an error indicating failed to get available moderators.
func ErrFailedToGetAvailableModerators(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get available moderators", err)
}

// ErrFailedToGetAdminList returns an error indicating failed to get admin list for fallback notification.
func ErrFailedToGetAdminList(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get admin list for fallback notification", err)
}

// Automatic processing errors

// ErrFailedToAddAutomaticReview returns an error indicating failed to add automatic review.
func ErrFailedToAddAutomaticReview(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to add automatic review", err)
}

// ErrFailedToProcessAutomaticReview returns an error indicating failed to process automatic review.
func ErrFailedToProcessAutomaticReview(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to process automatic review", err)
}

// Individual enforcement operation errors

// ErrUserUpdateFailed returns an error indicating user update failed.
func ErrUserUpdateFailed(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "user update failed", err)
}

// ErrTimelineFilteringOp returns an error indicating timeline filtering operation failed.
func ErrTimelineFilteringOp(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "timeline filtering operation failed", err)
}

// ErrSearchOperationFailed returns an error indicating search operation failed.
func ErrSearchOperationFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "search operation failed", err)
}

// ErrFederationOpFailed returns an error indicating federation operation failed.
func ErrFederationOpFailed(err error) error {
	return errors.NewFederationInternalError(errors.CodeDeliveryFailed, "federation operation failed", err)
}

// ErrObjectDeletionFailed returns an error indicating object deletion failed.
func ErrObjectDeletionFailed(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "object deletion failed", err)
}

// ErrTimelineRemovalFailed returns an error indicating timeline removal failed.
func ErrTimelineRemovalFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "timeline removal failed", err)
}

// ErrSearchRemovalFailed returns an error indicating search removal failed.
func ErrSearchRemovalFailed(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "search removal failed", err)
}

// ErrFederationDeletionFailed returns an error indicating federation deletion failed.
func ErrFederationDeletionFailed(err error) error {
	return errors.NewFederationInternalError(errors.CodeDeliveryFailed, "federation deletion failed", err)
}

// Batch processing errors

// ErrFailedToProcessRecords returns an error indicating failed to process records.
func ErrFailedToProcessRecords(err error) error {
	return errors.NewLambdaInternalError(errors.CodeSQSProcessingFailed, "failed to process records", err)
}
