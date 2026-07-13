package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
)

// DirectMessageStatusStageFn stages a prepared status row onto a caller-owned transaction.
// ConversationRepository uses this callback so DM orchestration can share one transaction
// without taking ownership of how Status rows are created.
type DirectMessageStatusStageFn func(tx core.TransactionBuilder, status *models.Status) error

// CanonicalStatusCreateRepository defines the canonical first-party status create contract.
// Callers that only need the status row should use CreateStatus. Callers that also own a
// companion transaction must prepare the status once, stage it through StageStatusCreate,
// and then run FinalizeCreatedStatus after the transaction commits.
type CanonicalStatusCreateRepository interface {
	CreateStatus(ctx context.Context, status *models.Status) error
	PrepareStatusCreate(status *models.Status) error
	StageStatusCreate(tx core.TransactionBuilder, status *models.Status) error
	FinalizeCreatedStatus(ctx context.Context, status *models.Status) error
}
