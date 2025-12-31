package reputation

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// VouchManager handles vouch creation and management
type VouchManager struct {
	store       core.RepositoryStorage
	signer      *Signer
	logger      *zap.Logger
	instanceURL string
}

// NewVouchManager creates a new vouch manager
func NewVouchManager(store core.RepositoryStorage, signer *Signer, instanceURL string, logger *zap.Logger) *VouchManager {
	return &VouchManager{
		store:       store,
		signer:      signer,
		logger:      logger,
		instanceURL: instanceURL,
	}
}

// CreateVouchInput contains the parameters for creating a vouch
type CreateVouchInput struct {
	FromActorID string
	ToActorID   string
	Confidence  float64
	Context     string
}

// CreateVouch creates a new vouch
func (vm *VouchManager) CreateVouch(ctx context.Context, input *CreateVouchInput) (*Vouch, error) {
	// Validate confidence
	if input.Confidence < 0 || input.Confidence > 1 {
		return nil, fmt.Errorf("confidence must be between 0 and 1")
	}

	// Check if voucher can create more vouches this month
	canVouch, err := vm.canCreateVouch(ctx, input.FromActorID)
	if err != nil {
		return nil, fmt.Errorf("failed to check vouch limit: %w", err)
	}
	if !canVouch {
		return nil, fmt.Errorf("monthly vouch limit reached")
	}

	// Get voucher's current reputation
	voucherRep, err := vm.getActorReputation(ctx, input.FromActorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher reputation: %w", err)
	}

	// Check if voucher has sufficient reputation
	if voucherRep < 500 {
		return nil, fmt.Errorf("insufficient reputation to vouch (need 500, have %d)", voucherRep)
	}

	// Create vouch
	vouch := &Vouch{
		ID:                fmt.Sprintf("vouch_%s", uuid.New().String()),
		From:              input.FromActorID,
		To:                input.ToActorID,
		InstanceURL:       vm.instanceURL,
		CreatedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(180 * 24 * time.Hour), // 6 months
		Confidence:        input.Confidence,
		Context:           input.Context,
		VoucherReputation: voucherRep,
		Active:            true,
		Revoked:           false,
	}

	// Sign the vouch
	if err := vm.signer.SignVouch(vouch); err != nil {
		return nil, fmt.Errorf("failed to sign vouch: %w", err)
	}

	// Convert to storage.Vouch
	storageVouch := &storage.Vouch{
		ID:                vouch.ID,
		From:              vouch.From,
		To:                vouch.To,
		CreatedAt:         vouch.CreatedAt,
		ExpiresAt:         &vouch.ExpiresAt,
		Confidence:        vouch.Confidence,
		Context:           vouch.Context,
		VoucherReputation: float64(vouch.VoucherReputation), // Convert int to float64
		Active:            vouch.Active,
		Revoked:           vouch.Revoked,
		RevokedAt:         vouch.RevokedAt,
		Signature:         vouch.Signature,
	}

	// Store using storage interface
	if err := vm.store.User().CreateVouch(ctx, storageVouch); err != nil {
		return nil, fmt.Errorf("failed to store vouch: %w", err)
	}

	vm.logger.Info("Created vouch",
		zap.String("id", vouch.ID),
		zap.String("from", input.FromActorID),
		zap.String("to", input.ToActorID),
		zap.Float64("confidence", input.Confidence))

	return vouch, nil
}

// RevokeVouch revokes an existing vouch
func (vm *VouchManager) RevokeVouch(ctx context.Context, vouchID string, actorID string) error {
	// Get the vouch
	vouch, err := vm.GetVouchByID(ctx, vouchID)
	if err != nil {
		return fmt.Errorf("failed to get vouch: %w", err)
	}

	// Check if actor can revoke (must be the voucher)
	if vouch.From != actorID {
		return fmt.Errorf("only the voucher can revoke their vouch")
	}

	// Update vouch status
	now := time.Now()
	err = vm.store.User().UpdateVouchStatus(ctx, vouchID, false, &now)
	if err != nil {
		return fmt.Errorf("failed to revoke vouch: %w", err)
	}

	vm.logger.Info("Revoked vouch",
		zap.String("id", vouchID),
		zap.String("by", actorID))

	return nil
}

// GetVouchByID retrieves a vouch by ID
func (vm *VouchManager) GetVouchByID(ctx context.Context, vouchID string) (*Vouch, error) {
	storageVouch, err := vm.store.User().GetVouch(ctx, vouchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vouch: %w", err)
	}

	if storageVouch == nil {
		return nil, fmt.Errorf("vouch not found")
	}

	// Convert storage.Vouch to reputation.Vouch
	vouch := &Vouch{
		ID:          storageVouch.ID,
		From:        storageVouch.From,
		To:          storageVouch.To,
		InstanceURL: vm.instanceURL, // Use service's instance URL
		CreatedAt:   storageVouch.CreatedAt,
		ExpiresAt: func() time.Time {
			if storageVouch.ExpiresAt == nil {
				return time.Time{}
			}
			return *storageVouch.ExpiresAt
		}(),
		Confidence:        storageVouch.Confidence,
		Context:           storageVouch.Context,
		VoucherReputation: int(storageVouch.VoucherReputation), // Convert float64 to int
		Active:            storageVouch.Active,
		Revoked:           storageVouch.Revoked,
		RevokedAt:         storageVouch.RevokedAt,
		Signature:         storageVouch.Signature,
	}

	return vouch, nil
}

// GetVouchesForActor gets all vouches for an actor
func (vm *VouchManager) GetVouchesForActor(ctx context.Context, actorID string) ([]Vouch, error) {
	storageVouches, err := vm.store.User().GetVouchesForActor(ctx, actorID, true) // Get only active vouches
	if err != nil {
		return nil, fmt.Errorf("failed to query vouches: %w", err)
	}

	return vm.convertStorageVouchesToReputationVouches(storageVouches), nil
}

// GetVouchesFromActor gets all vouches created by an actor
func (vm *VouchManager) GetVouchesFromActor(ctx context.Context, actorID string) ([]Vouch, error) {
	storageVouches, err := vm.store.User().GetVouchesByActor(ctx, actorID, false) // Get all vouches, not just active
	if err != nil {
		return nil, fmt.Errorf("failed to query vouches: %w", err)
	}

	return vm.convertStorageVouchesToReputationVouches(storageVouches), nil
}

// canCreateVouch checks if an actor can create more vouches this month
func (vm *VouchManager) canCreateVouch(ctx context.Context, actorID string) (bool, error) {
	// Get month count using storage interface
	now := time.Now()
	count, err := vm.store.User().GetMonthlyVouchCount(ctx, actorID, now.Year(), now.Month())
	if err != nil {
		return false, fmt.Errorf("failed to get monthly vouch count: %w", err)
	}

	// Allow max 5 vouches per month
	return count < 5, nil
}

// getActorReputation gets an actor's current reputation score
func (vm *VouchManager) getActorReputation(ctx context.Context, actorID string) (int, error) {
	// Get reputation from storage
	rep, err := vm.store.User().GetReputation(ctx, actorID)
	if err != nil {
		return 0, fmt.Errorf("failed to get reputation: %w", err)
	}

	if rep == nil {
		// No reputation history, return 0
		return 0, nil
	}

	return int(rep.TotalScore), nil
}

// ImportVouch imports a vouch from another instance
func (vm *VouchManager) ImportVouch(ctx context.Context, vouch *Vouch, verifier vouchSignatureVerifier) error {
	// Validate vouch
	if vouch == nil {
		return fmt.Errorf("vouch cannot be nil")
	}

	// Check if vouch is still valid
	now := time.Now()
	if !vouch.Active || vouch.Revoked {
		return fmt.Errorf("vouch is not active or has been revoked")
	}
	if now.After(vouch.ExpiresAt) {
		return fmt.Errorf("vouch has expired")
	}

	// Verify the signature
	valid, err := verifier.VerifyVouchSignature(vouch)
	if err != nil {
		return fmt.Errorf("failed to verify vouch signature: %w", err)
	}
	if !valid {
		return fmt.Errorf("vouch signature is invalid")
	}

	// Check if vouch already exists (prevent duplicates)
	existing, err := vm.GetVouchByID(ctx, vouch.ID)
	if err == nil && existing != nil {
		// Vouch already exists
		vm.logger.Debug("Vouch already imported", zap.String("vouch_id", vouch.ID))
		return nil
	}

	// Check if the voucher has sufficient reputation from their instance
	if vouch.VoucherReputation < 500 {
		return fmt.Errorf("voucher had insufficient reputation at time of vouch (%d < 500)", vouch.VoucherReputation)
	}

	// Convert to storage.Vouch
	storageVouch := &storage.Vouch{
		ID:                vouch.ID,
		From:              vouch.From,
		To:                vouch.To,
		CreatedAt:         vouch.CreatedAt,
		ExpiresAt:         &vouch.ExpiresAt,
		Confidence:        vouch.Confidence,
		Context:           vouch.Context,
		VoucherReputation: float64(vouch.VoucherReputation), // Convert int to float64
		Active:            vouch.Active,
		Revoked:           vouch.Revoked,
		RevokedAt:         vouch.RevokedAt,
		Signature:         vouch.Signature,
	}

	// Store the imported vouch
	if err := vm.store.User().CreateVouch(ctx, storageVouch); err != nil {
		// Check if it's a duplicate error
		if err.Error() == "vouch already exists" {
			vm.logger.Debug("Vouch already exists", zap.String("vouch_id", vouch.ID))
			return nil
		}
		return fmt.Errorf("failed to store imported vouch: %w", err)
	}

	vm.logger.Info("Imported vouch",
		zap.String("id", vouch.ID),
		zap.String("from", vouch.From),
		zap.String("to", vouch.To),
		zap.String("instance", vouch.InstanceURL),
		zap.Float64("confidence", vouch.Confidence))

	return nil
}

// ImportVouches imports multiple vouches in batch
func (vm *VouchManager) ImportVouches(ctx context.Context, vouches []Vouch, verifier vouchSignatureVerifier) (int, error) {
	imported := 0

	for _, vouch := range vouches {
		if err := vm.ImportVouch(ctx, &vouch, verifier); err != nil {
			vm.logger.Warn("Failed to import vouch",
				zap.String("vouch_id", vouch.ID),
				zap.Error(err))
			continue
		}
		imported++
	}

	return imported, nil
}

// convertStorageVouchesToReputationVouches converts a slice of storage.Vouch to reputation.Vouch
func (vm *VouchManager) convertStorageVouchesToReputationVouches(storageVouches []*storage.Vouch) []Vouch {
	vouches := make([]Vouch, 0, len(storageVouches))
	for _, sv := range storageVouches {
		vouch := Vouch{
			ID:          sv.ID,
			From:        sv.From,
			To:          sv.To,
			InstanceURL: vm.instanceURL, // Use service's instance URL
			CreatedAt:   sv.CreatedAt,
			ExpiresAt: func() time.Time {
				if sv.ExpiresAt == nil {
					return time.Time{}
				}
				return *sv.ExpiresAt
			}(),
			Confidence:        sv.Confidence,
			Context:           sv.Context,
			VoucherReputation: int(sv.VoucherReputation), // Convert float64 to int
			Active:            sv.Active,
			Revoked:           sv.Revoked,
			RevokedAt:         sv.RevokedAt,
			Signature:         sv.Signature,
		}
		vouches = append(vouches, vouch)
	}
	return vouches
}
