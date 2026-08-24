// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ErrUploadGrantConsumed reports that an upload grant was already consumed by
// a concurrent or prior finalize. The consume is version-conditioned, so this
// surfaces the single-use race rather than a double admit.
var ErrUploadGrantConsumed = errors.New("upload grant has already been consumed")

// UploadGrantRepository defines storage for one-time, hash-bound, actor-scoped
// upload grants minted by the presigned-companion transport.
type UploadGrantRepository interface {
	// CreateUploadGrant persists a freshly minted grant. It is a conditional
	// create keyed on the unguessable grant ID, so a colliding mint fails.
	CreateUploadGrant(ctx context.Context, grant *models.UploadGrant) error

	// GetUploadGrant loads one grant within the owner's partition. The owner
	// scoping is part of the key construction, so a caller can never read
	// another actor's grant through this surface.
	GetUploadGrant(ctx context.Context, ownerID, grantID string) (*models.UploadGrant, error)

	// ConsumeUploadGrant atomically transitions a MINTED grant to the given
	// terminal status, conditioned on the observed version and the MINTED
	// state, so exactly one concurrent finalize wins. On a race it returns an
	// error wrapping ErrUploadGrantConsumed.
	ConsumeUploadGrant(ctx context.Context, grant *models.UploadGrant, status, failureReason string, now time.Time) error
}
