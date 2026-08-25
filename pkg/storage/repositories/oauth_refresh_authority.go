package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	tableerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

// OAuthRefreshAuthorityMaxSlots bounds concurrent live families per authority tuple.
const OAuthRefreshAuthorityMaxSlots = 8

// ErrOAuthRefreshWalkBudgetExceeded signals that replay work exhausted its bounded window.
var ErrOAuthRefreshWalkBudgetExceeded = errors.New("oauth refresh replay walk budget exceeded")

// GetRefreshTokenConsistent is the authoritative direct read used by refresh CAS.
func (r *AccountRepository) GetRefreshTokenConsistent(ctx context.Context, token string) (*storage.RefreshToken, error) {
	model := models.RefreshToken{Token: strings.TrimSpace(token)}
	if err := model.UpdateKeys(); err != nil {
		return nil, storage.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("PK", "=", model.PK).
		Where("SK", "=", model.SK).
		ConsistentRead().
		First(&model)
	if err != nil {
		if tableerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("read refresh token consistently: %w", err)
	}
	result := refreshTokenStorageFromModel(model)
	return result, nil
}

// GetOAuthRefreshAuthority performs the strongly-consistent singleton read.
func (r *AccountRepository) GetOAuthRefreshAuthority(ctx context.Context, username, clientID, resource string) (*models.OAuthRefreshAuthority, error) {
	authority := &models.OAuthRefreshAuthority{Username: username, ClientID: clientID, Resource: resource}
	if err := authority.UpdateKeys(); err != nil {
		return nil, storage.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Model(&models.OAuthRefreshAuthority{}).
		Where("PK", "=", authority.PK).
		Where("SK", "=", authority.SK).
		ConsistentRead().
		First(authority)
	if err != nil {
		if tableerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("read oauth refresh authority: %w", err)
	}
	return authority, nil
}

// OAuthRefreshAuthorityWithHead returns a copy whose LRU slot list names the
// supplied family head. The caller must CAS the returned revision.
func OAuthRefreshAuthorityWithHead(
	current *models.OAuthRefreshAuthority,
	username, clientID, resource, familyID, headHash string,
	generation int,
	expiresAt, now time.Time,
) *models.OAuthRefreshAuthority {
	next := &models.OAuthRefreshAuthority{
		Username:  username,
		ClientID:  clientID,
		Resource:  resource,
		UpdatedAt: now.UTC(),
	}
	if current != nil {
		*next = *current
		next.Slots = append([]models.OAuthRefreshFamilySlot(nil), current.Slots...)
		next.UpdatedAt = now.UTC()
	}

	kept := next.Slots[:0]
	for _, slot := range next.Slots {
		if slot.FamilyID != familyID && slot.ExpiresAt.After(now) {
			kept = append(kept, slot)
		}
	}
	kept = append(kept, models.OAuthRefreshFamilySlot{
		FamilyID:      familyID,
		HeadTokenHash: headHash,
		Generation:    generation,
		ExpiresAt:     expiresAt.UTC(),
		UpdatedAt:     now.UTC(),
	})
	if len(kept) > OAuthRefreshAuthorityMaxSlots {
		kept = kept[len(kept)-OAuthRefreshAuthorityMaxSlots:]
	}
	next.Slots = kept
	if expiresAt.Unix() > next.TTL {
		next.TTL = expiresAt.Unix()
	}
	_ = next.UpdateKeys()
	return next
}

// RotateRefreshTokenWithAuthority atomically rotates the credential, appends
// its encrypted successor edge, and advances the tuple authority CAS.
func (r *AccountRepository) RotateRefreshTokenWithAuthority(
	ctx context.Context,
	predecessor, successor *storage.RefreshToken,
	currentAuthority, nextAuthority *models.OAuthRefreshAuthority,
	now time.Time,
) error {
	if predecessor == nil || successor == nil || nextAuthority == nil || strings.TrimSpace(predecessor.Token) == "" {
		return storage.ErrInvalidInput
	}

	expectedTokenVersion := predecessor.Version
	legacy := strings.TrimSpace(predecessor.FamilyID) == "" || predecessor.Generation < 1
	if legacy {
		predecessor.FamilyID = successor.FamilyID
		predecessor.Generation = successor.Generation - 1
	}
	now = now.UTC()
	predecessor.Current = false
	predecessor.Revoked = true
	predecessor.RevokedAt = now
	predecessor.RevokedReason = "rotated"
	predecessor.ReplacedByHash = storage.RefreshTokenReplacementHash(successor.Token)
	if !predecessor.LastUsedAt.IsZero() {
		predecessor.LastUsedAt = now
	}

	predecessorModel := refreshTokenModelFromStorage(predecessor)
	if err := predecessorModel.UpdateKeys(); err != nil {
		return fmt.Errorf("prepare refresh predecessor: %w", err)
	}
	successorModel := refreshTokenModelFromStorage(successor)
	if err := successorModel.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare refresh successor: %w", err)
	}
	artifact := &models.OAuthRefreshSuccessorArtifact{
		FamilyID:            successor.FamilyID,
		PredecessorHash:     storage.RefreshTokenReplacementHash(predecessor.Token),
		SuccessorHash:       storage.RefreshTokenReplacementHash(successor.Token),
		SuccessorToken:      successor.Token,
		SuccessorGeneration: successor.Generation,
		CreatedAt:           now,
		TTL:                 successor.ExpiresAt.Unix(),
	}
	if err := artifact.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare refresh successor artifact: %w", err)
	}
	if err := nextAuthority.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare refresh authority: %w", err)
	}

	tokenConditions := append(refreshTokenVersionConditions(expectedTokenVersion), tabletheory.IfExists())
	if legacy {
		tokenConditions = append(tokenConditions, tabletheory.Condition("FamilyID", "attribute_not_exists", nil))
	} else {
		tokenConditions = append(tokenConditions,
			tabletheory.Condition("Current", "=", true),
			tabletheory.Condition("Revoked", "=", false),
		)
	}

	err := r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.UpdateWithBuilder(predecessorModel, func(update core.UpdateBuilder) error {
			update.Set("FamilyID", predecessor.FamilyID).
				Set("Generation", predecessor.Generation).
				Set("Current", false).
				Set("Revoked", true).
				Set("RevokedAt", now).
				Set("RevokedReason", predecessor.RevokedReason).
				Set("ReplacedByHash", predecessor.ReplacedByHash).
				Set("GSI2PK", predecessorModel.GSI2PK).
				Set("GSI2SK", predecessorModel.GSI2SK).
				Increment("Version")
			if !predecessor.LastUsedAt.IsZero() {
				update.Set("LastUsedAt", now)
			}
			return nil
		}, tokenConditions...)
		tx.Create(successorModel, tabletheory.IfNotExists())
		tx.Create(artifact, tabletheory.IfNotExists())
		if currentAuthority == nil {
			tx.Create(nextAuthority, tabletheory.IfNotExists())
		} else {
			tx.UpdateWithBuilder(nextAuthority, func(update core.UpdateBuilder) error {
				update.Set("Slots", nextAuthority.Slots).
					Set("UpdatedAt", nextAuthority.UpdatedAt).
					Set("TTL", nextAuthority.TTL).
					Increment("Revision")
				return nil
			}, tabletheory.IfExists(), tabletheory.AtVersion(int64(currentAuthority.Revision)))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rotate refresh token with authority: %w", err)
	}
	predecessor.Version = expectedTokenVersion + 1
	if currentAuthority == nil {
		nextAuthority.Revision = 0
	} else {
		nextAuthority.Revision = currentAuthority.Revision + 1
	}
	return nil
}

// IsOAuthRefreshCASConflict identifies contention without treating the grant as dead.
func IsOAuthRefreshCASConflict(err error) bool {
	return errors.Is(err, tableerrors.ErrConditionFailed) || strings.Contains(strings.ToLower(errString(err)), "conditionalcheckfailed")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetOAuthRefreshSuccessorArtifact consistently reads an encrypted lineage edge.
func (r *AccountRepository) GetOAuthRefreshSuccessorArtifact(ctx context.Context, familyID, predecessorHash string) (*models.OAuthRefreshSuccessorArtifact, error) {
	artifact := &models.OAuthRefreshSuccessorArtifact{FamilyID: familyID, PredecessorHash: predecessorHash}
	if err := artifact.UpdateKeys(); err != nil {
		return nil, storage.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Model(&models.OAuthRefreshSuccessorArtifact{}).
		Where("PK", "=", artifact.PK).
		Where("SK", "=", artifact.SK).
		ConsistentRead().
		First(artifact)
	if err != nil {
		if tableerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("read refresh successor artifact: %w", err)
	}
	return artifact, nil
}

// ChargeOAuthRefreshWalkBudget pre-charges replay work against a per-minute family item.
func (r *AccountRepository) ChargeOAuthRefreshWalkBudget(ctx context.Context, familyID string, charge, limit int, now time.Time) (*models.OAuthRefreshWalkBudget, error) {
	if strings.TrimSpace(familyID) == "" || charge <= 0 || limit < charge {
		return nil, storage.ErrInvalidInput
	}
	window := now.UTC().Truncate(time.Minute).Format("20060102T1504")
	for attempt := 0; attempt < 3; attempt++ {
		current, err := r.getOAuthRefreshWalkBudget(ctx, familyID, window)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		next := &models.OAuthRefreshWalkBudget{
			FamilyID:  familyID,
			Window:    window,
			Consumed:  charge,
			UpdatedAt: now.UTC(),
			TTL:       now.UTC().Add(2 * time.Minute).Unix(),
		}
		if current != nil {
			if current.Consumed+charge > limit {
				return nil, ErrOAuthRefreshWalkBudgetExceeded
			}
			*next = *current
			next.Consumed += charge
			next.UpdatedAt = now.UTC()
		}
		if err := next.BeforeCreate(); err != nil {
			return nil, err
		}
		err = r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
			if current == nil {
				tx.Create(next, tabletheory.IfNotExists())
			} else {
				tx.UpdateWithBuilder(next, func(update core.UpdateBuilder) error {
					update.Set("Consumed", next.Consumed).Set("UpdatedAt", next.UpdatedAt).Increment("Version")
					return nil
				}, tabletheory.IfExists(), tabletheory.AtVersion(int64(current.Version)))
			}
			return nil
		})
		if err == nil {
			return next, nil
		}
		if !IsOAuthRefreshCASConflict(err) {
			return nil, fmt.Errorf("charge refresh replay walk budget: %w", err)
		}
	}
	return nil, fmt.Errorf("charge refresh replay walk budget: %w", tableerrors.ErrConditionFailed)
}

// RefundOAuthRefreshWalkBudget refunds unused pre-charged steps with CAS.
func (r *AccountRepository) RefundOAuthRefreshWalkBudget(ctx context.Context, charged *models.OAuthRefreshWalkBudget, unused int, now time.Time) error {
	if charged == nil || unused <= 0 {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		current, err := r.getOAuthRefreshWalkBudget(ctx, charged.FamilyID, charged.Window)
		if err != nil {
			return fmt.Errorf("read refresh replay budget for refund: %w", err)
		}
		nextConsumed := current.Consumed - unused
		if nextConsumed < 0 {
			nextConsumed = 0
		}
		err = r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
			tx.UpdateWithBuilder(current, func(update core.UpdateBuilder) error {
				update.Set("Consumed", nextConsumed).Set("UpdatedAt", now.UTC()).Increment("Version")
				return nil
			}, tabletheory.IfExists(), tabletheory.AtVersion(int64(current.Version)))
			return nil
		})
		if err == nil {
			return nil
		}
		if !IsOAuthRefreshCASConflict(err) {
			return fmt.Errorf("refund refresh replay walk budget: %w", err)
		}
	}
	return fmt.Errorf("refund refresh replay walk budget: %w", tableerrors.ErrConditionFailed)
}

func (r *AccountRepository) getOAuthRefreshWalkBudget(ctx context.Context, familyID, window string) (*models.OAuthRefreshWalkBudget, error) {
	budget := &models.OAuthRefreshWalkBudget{FamilyID: familyID, Window: window}
	if err := budget.UpdateKeys(); err != nil {
		return nil, storage.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Model(&models.OAuthRefreshWalkBudget{}).
		Where("PK", "=", budget.PK).
		Where("SK", "=", budget.SK).
		ConsistentRead().
		First(budget)
	if err != nil {
		if tableerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return budget, nil
}
