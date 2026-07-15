package repositories

import (
	"context"
	"strings"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// GetSoulBindingIdempotencyReceipt returns a soul-binding idempotency receipt, or (nil, nil) when absent.
func (r *InstanceRepository) GetSoulBindingIdempotencyReceipt(ctx context.Context, callerID string, idempotencyKey string) (*models.InstanceSoulBindingIdempotencyReceipt, error) {
	if r == nil || r.soulBindingReceiptRepo == nil {
		return nil, nil
	}

	callerID = strings.TrimSpace(callerID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if callerID == "" || idempotencyKey == "" {
		return nil, nil
	}

	receipt := &models.InstanceSoulBindingIdempotencyReceipt{}
	err := r.soulBindingReceiptRepo.Get(
		ctx,
		models.SoulBindingIdempotencyPartitionKey(callerID),
		models.SoulBindingIdempotencySortKey(idempotencyKey),
		receipt,
	)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if strings.TrimSpace(receipt.PK) == "" || strings.TrimSpace(receipt.SK) == "" {
		return nil, nil
	}
	return receipt, nil
}

// CreateSoulBindingIdempotencyReceipt creates a new receipt if the key is not already reserved.
func (r *InstanceRepository) CreateSoulBindingIdempotencyReceipt(ctx context.Context, receipt *models.InstanceSoulBindingIdempotencyReceipt) error {
	if r == nil || r.soulBindingReceiptRepo == nil {
		return nil
	}
	return r.soulBindingReceiptRepo.CreateIfNotExists(ctx, receipt)
}

// UpdateSoulBindingIdempotencyReceipt updates an existing soul-binding idempotency receipt.
func (r *InstanceRepository) UpdateSoulBindingIdempotencyReceipt(ctx context.Context, receipt *models.InstanceSoulBindingIdempotencyReceipt) error {
	if r == nil || r.soulBindingReceiptRepo == nil {
		return nil
	}
	return r.soulBindingReceiptRepo.Update(ctx, receipt)
}
