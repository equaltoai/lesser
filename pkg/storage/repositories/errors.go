package repositories

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/dynamorm/pkg/errors"
)

// ErrorUtils provides utilities for standardizing error handling across repositories
type ErrorUtils struct{}

// NewErrorUtils creates a new ErrorUtils instance
func NewErrorUtils() *ErrorUtils {
	return &ErrorUtils{}
}

// HandleNotFound converts DynamORM not found errors to domain-specific errors
func (e *ErrorUtils) HandleNotFound(err error, entityType, identifier string) error {
	if errors.IsNotFound(err) {
		return fmt.Errorf("%s not found: %s", entityType, identifier)
	}
	return err
}

// HandleGetError standardizes error handling for Get operations
func (e *ErrorUtils) HandleGetError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}
	
	if errors.IsNotFound(err) {
		return fmt.Errorf("%s not found: %s", entityType, identifier)
	}
	
	return fmt.Errorf("failed to get %s: %w", entityType, err)
}

// HandleCreateError standardizes error handling for Create operations
func (e *ErrorUtils) HandleCreateError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}
	
	if errors.IsConditionFailed(err) {
		return fmt.Errorf("%s already exists: %s", entityType, identifier)
	}
	
	return fmt.Errorf("failed to create %s: %w", entityType, err)
}

// HandleUpdateError standardizes error handling for Update operations
func (e *ErrorUtils) HandleUpdateError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}
	
	if errors.IsNotFound(err) {
		return fmt.Errorf("%s not found for update: %s", entityType, identifier)
	}
	
	return fmt.Errorf("failed to update %s: %w", entityType, err)
}

// HandleDeleteError standardizes error handling for Delete operations
func (e *ErrorUtils) HandleDeleteError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}
	
	// For deletes, we typically don't treat "not found" as an error
	if errors.IsNotFound(err) {
		return nil
	}
	
	return fmt.Errorf("failed to delete %s: %w", entityType, err)
}

// HandleQueryError standardizes error handling for Query operations
func (e *ErrorUtils) HandleQueryError(err error, entityType, queryType string) error {
	if err == nil {
		return nil
	}
	
	return fmt.Errorf("failed to query %s (%s): %w", entityType, queryType, err)
}

// IsConditionalCheckFailed checks if error is a conditional check failure
func (e *ErrorUtils) IsConditionalCheckFailed(err error) bool {
	return errors.IsConditionFailed(err)
}

// IsNotFound checks if error is a not found error
func (e *ErrorUtils) IsNotFound(err error) bool {
	return errors.IsNotFound(err)
}

// Common entity type constants for consistent error messages
const (
	EntityUser                = "user"
	EntityActor               = "actor"
	EntityObject              = "object"
	EntityFollow              = "follow"
	EntityBlock               = "block"
	EntityMute                = "mute"
	EntityList                = "list"
	EntityHashtag             = "hashtag"
	EntityMedia               = "media"
	EntityOAuthState          = "OAuth state"
	EntityAuthCode            = "authorization code"
	EntityRefreshToken        = "refresh token"
	EntityOAuthClient         = "OAuth client"
	EntityWebAuthnCredential  = "WebAuthn credential"
	EntityWebAuthnChallenge   = "WebAuthn challenge"
	EntityWalletCredential    = "wallet credential"
	EntityWalletChallenge     = "wallet challenge"
	EntitySession             = "session"
	EntityPasswordReset       = "password reset"
	EntityTimelineEntry       = "timeline entry"
	EntityConversation        = "conversation"
	EntityBookmark            = "bookmark"
	EntityFilter              = "filter"
	EntityFilterKeyword       = "filter keyword"
	EntityFilterStatus        = "filter status"
	EntityReport              = "report"
	EntityFlag                = "flag"
	EntityModerationEvent     = "moderation event"
	EntityModerationDecision  = "moderation decision"
	EntityModerationPattern   = "moderation pattern"
)

// Global error utils instance
var ErrorHandler = NewErrorUtils()

// MapDynamoDBError maps DynamoDB/DynamORM errors to storage errors
func MapDynamoDBError(err error) error {
	if err == nil {
		return nil
	}
	
	// Check for DynamORM error types first
	if errors.IsNotFound(err) {
		return storage.ErrNotFound
	}
	
	if errors.IsConditionFailed(err) {
		return storage.ErrAlreadyExists
	}
	
	// Fall back to string matching for other errors
	errStr := err.Error()
	
	// Validation errors
	if strings.Contains(errStr, "validation failed") || strings.Contains(errStr, "invalid") {
		return storage.ErrInvalidInput
	}
	
	// Authorization errors
	if strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "forbidden") {
		return storage.ErrUnauthorized
	}
	
	// Default to original error wrapped with context
	return fmt.Errorf("database error: %w", err)
}

// MapErrorWithContext wraps an error with additional context
func MapErrorWithContext(err error, context string) error {
	if err == nil {
		return nil
	}
	
	mappedErr := MapDynamoDBError(err)
	return fmt.Errorf("%s: %w", context, mappedErr)
}