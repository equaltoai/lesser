package main

import "errors"

// Error constants for moderation processor
var (
	// Record format validation errors
	ErrInvalidReviewPKFormat   = errors.New("invalid review PK format")
	ErrInvalidReviewSKFormat   = errors.New("invalid review SK format")
	ErrNotReviewRecord         = errors.New("not a review record")
	ErrNotEventRecord          = errors.New("not an event record")
	ErrNotDecisionRecord       = errors.New("not a decision record")
	
	// Action and processing errors
	ErrUnknownActionType       = errors.New("unknown action type")
	
	// Moderator and admin errors
	ErrFailedToRetrieveModerators = errors.New("failed to retrieve both moderators and admins from storage")
	ErrNoAdminsAvailableForFallback = errors.New("no admins available for fallback")
	
	// Enforcement errors
	ErrEnforcementFailed       = errors.New("enforcement failed")
	ErrContentRemovalFailed    = errors.New("content removal enforcement failed")
	ErrTimelineFilteringFailed = errors.New("timeline filtering had errors")
	
	// Extraction and processing errors
	ErrFailedToExtractReview   = errors.New("failed to extract review")
	ErrFailedToExtractEvent    = errors.New("failed to extract event")
	ErrFailedToExtractDecision = errors.New("failed to extract decision")
	
	// Moderator selection errors
	ErrFailedToGetAvailableModerators = errors.New("failed to get available moderators")
	ErrFailedToGetAdminList          = errors.New("failed to get admin list for fallback notification")
	
	// Automatic processing errors
	ErrFailedToAddAutomaticReview     = errors.New("failed to add automatic review")
	ErrFailedToProcessAutomaticReview = errors.New("failed to process automatic review")
	
	// Individual enforcement operation errors
	ErrUserUpdateFailed       = errors.New("user update failed")
	ErrTimelineFilteringOp    = errors.New("timeline filtering operation failed")
	ErrSearchOperationFailed  = errors.New("search operation failed")
	ErrFederationOpFailed     = errors.New("federation operation failed")
	ErrObjectDeletionFailed   = errors.New("object deletion failed")
	ErrTimelineRemovalFailed  = errors.New("timeline removal failed")
	ErrSearchRemovalFailed    = errors.New("search removal failed")
	ErrFederationDeletionFailed = errors.New("federation deletion failed")
	
	// Batch processing errors
	ErrFailedToProcessRecords  = errors.New("failed to process records")
)