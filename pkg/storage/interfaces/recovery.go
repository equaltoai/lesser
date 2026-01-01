// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// RecoveryRepository defines the interface for account recovery operations.
// This handles trustees, recovery requests, recovery codes, and recovery tokens.
type RecoveryRepository interface {
	// Trustee operations

	// StoreTrustee stores a trustee configuration for social recovery
	StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error

	// GetTrustees retrieves all trustees for a user
	GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error)

	// DeleteTrustee removes a trustee
	DeleteTrustee(ctx context.Context, username, trusteeActorID string) error

	// UpdateTrusteeConfirmed updates the confirmed status of a trustee
	UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error

	// Recovery request operations

	// StoreRecoveryRequest stores a social recovery request
	StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error

	// GetRecoveryRequest retrieves a recovery request by ID
	GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error)

	// UpdateRecoveryRequest updates a recovery request
	UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error

	// DeleteRecoveryRequest deletes a recovery request
	DeleteRecoveryRequest(ctx context.Context, requestID string) error

	// GetActiveRecoveryRequests gets all active recovery requests for a user
	GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error)

	// Recovery code operations

	// StoreRecoveryCode stores a recovery code
	StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error

	// GetRecoveryCodes retrieves all recovery codes for a user
	GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error)

	// MarkRecoveryCodeUsed marks a recovery code as used
	MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error

	// DeleteAllRecoveryCodes deletes all recovery codes for a user
	DeleteAllRecoveryCodes(ctx context.Context, username string) error

	// CountUnusedRecoveryCodes counts how many unused recovery codes the user has
	CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error)

	// Recovery token operations

	// StoreRecoveryToken stores a generic recovery token with data
	StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error

	// GetRecoveryToken retrieves a recovery token by key
	GetRecoveryToken(ctx context.Context, key string) (map[string]any, error)

	// DeleteRecoveryToken deletes a recovery token
	DeleteRecoveryToken(ctx context.Context, key string) error
}
