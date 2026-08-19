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

// ConsumeAuthorizationCodeAndCreateRefreshToken atomically burns a validated
// authorization code and persists the refresh credential issued for it. A
// failed refresh-token write therefore leaves the code retryable instead of
// returning an unbacked credential to the client.
func (r *AccountRepository) ConsumeAuthorizationCodeAndCreateRefreshToken(
	ctx context.Context,
	code string,
	token *storage.RefreshToken,
) error {
	code = strings.TrimSpace(code)
	if code == "" || token == nil {
		return storage.ErrInvalidInput
	}

	codeModel := &models.AuthorizationCode{Code: code}
	if err := codeModel.UpdateKeys(); err != nil {
		return fmt.Errorf("prepare authorization code transaction key: %w", err)
	}
	tokenModel := refreshTokenModelFromStorage(token)
	if err := tokenModel.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare refresh token transaction item: %w", err)
	}

	err := r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(codeModel, tabletheory.IfExists())
		tx.Create(tokenModel, tabletheory.IfNotExists())
		return nil
	})
	if transactionConditionFailedAt(err, 0) {
		return errors.Join(storage.ErrNotFound, err)
	}
	if err != nil {
		return fmt.Errorf("consume authorization code and create refresh token: %w", err)
	}

	return nil
}

// ConsumeOAuthDeviceSessionAndCreateRefreshToken atomically transitions an
// approved device session to consumed and persists its refresh credential.
// Infrastructure failure keeps the approved session retryable.
func (r *AccountRepository) ConsumeOAuthDeviceSessionAndCreateRefreshToken(
	ctx context.Context,
	session *storage.OAuthDeviceSession,
	token *storage.RefreshToken,
	consumedAt time.Time,
) error {
	if session == nil || token == nil || strings.TrimSpace(session.DeviceCodeHash) == "" {
		return storage.ErrInvalidInput
	}

	sessionModel := oauthDeviceSessionModelFromStorage(session)
	if err := sessionModel.UpdateKeys(); err != nil {
		return fmt.Errorf("prepare oauth device session transaction key: %w", err)
	}
	tokenModel := refreshTokenModelFromStorage(token)
	if err := tokenModel.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare device refresh token transaction item: %w", err)
	}
	consumedAt = consumedAt.UTC()
	sessionModel.Status = "consumed"
	sessionModel.ConsumedAt = &consumedAt
	sessionModel.UpdatedAt = consumedAt

	err := r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.UpdateWithBuilder(sessionModel, func(update core.UpdateBuilder) error {
			update.Set("Status", "consumed").
				Set("ConsumedAt", consumedAt).
				Set("UpdatedAt", consumedAt)
			return nil
		}, tabletheory.IfExists(), tabletheory.Condition("Status", "=", "approved"))
		tx.Create(tokenModel, tabletheory.IfNotExists())
		return nil
	})
	if transactionConditionFailedAt(err, 0) {
		return errors.Join(storage.ErrNotFound, err)
	}
	if err != nil {
		return fmt.Errorf("consume oauth device session and create refresh token: %w", err)
	}

	session.Status = "consumed"
	session.ConsumedAt = consumedAt
	session.UpdatedAt = consumedAt
	return nil
}

// RotateRefreshToken atomically revokes the presented standard refresh token
// and creates its sole successor. Legacy rows without lineage are adopted into
// a new family in the same transaction.
func (r *AccountRepository) RotateRefreshToken(
	ctx context.Context,
	predecessor, successor *storage.RefreshToken,
	now time.Time,
) error {
	if predecessor == nil || successor == nil || strings.TrimSpace(predecessor.Token) == "" {
		return storage.ErrInvalidInput
	}

	expectedVersion := predecessor.Version
	legacy := strings.TrimSpace(predecessor.FamilyID) == "" || predecessor.Generation < 1
	trackLastUsed := !predecessor.LastUsedAt.IsZero()
	if legacy {
		predecessor.FamilyID = successor.FamilyID
		predecessor.Generation = successor.Generation - 1
	}
	predecessor.Current = false
	predecessor.Revoked = true
	predecessor.RevokedAt = now.UTC()
	predecessor.RevokedReason = "rotated"
	predecessor.ReplacedByHash = storage.RefreshTokenReplacementHash(successor.Token)
	if trackLastUsed {
		predecessor.LastUsedAt = now.UTC()
	}

	predecessorModel := refreshTokenModelFromStorage(predecessor)
	if err := predecessorModel.UpdateKeys(); err != nil {
		return fmt.Errorf("prepare refresh predecessor transaction item: %w", err)
	}
	successorModel := refreshTokenModelFromStorage(successor)
	if err := successorModel.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare refresh successor transaction item: %w", err)
	}
	conditions := refreshTokenVersionConditions(expectedVersion)
	conditions = append(conditions, tabletheory.IfExists())
	if legacy {
		conditions = append(conditions, tabletheory.Condition("FamilyID", "attribute_not_exists", nil))
	} else {
		conditions = append(conditions,
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
				Set("RevokedAt", predecessor.RevokedAt).
				Set("RevokedReason", predecessor.RevokedReason).
				Set("ReplacedByHash", predecessor.ReplacedByHash).
				Set("GSI2PK", predecessorModel.GSI2PK).
				Set("GSI2SK", predecessorModel.GSI2SK).
				Increment("Version")
			if trackLastUsed {
				update.Set("LastUsedAt", predecessor.LastUsedAt)
			}
			return nil
		}, conditions...)
		tx.Create(successorModel, tabletheory.IfNotExists())
		return nil
	})
	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}

	predecessor.Version = expectedVersion + 1
	return nil
}

// RedeemRefreshTokenRetry performs the one permitted grace rescue. It marks
// the stale predecessor redeemed, revokes its active replacement, and creates
// a new sole family head in one transaction.
func (r *AccountRepository) RedeemRefreshTokenRetry(
	ctx context.Context,
	stale, active, successor *storage.RefreshToken,
	now time.Time,
) error {
	if stale == nil || active == nil || successor == nil ||
		strings.TrimSpace(stale.Token) == "" || strings.TrimSpace(active.Token) == "" {
		return storage.ErrInvalidInput
	}

	staleVersion := stale.Version
	activeVersion := active.Version
	trackLastUsed := !active.LastUsedAt.IsZero()
	now = now.UTC()
	stale.RetryRedeemedAt = now
	active.Current = false
	active.Revoked = true
	active.RevokedAt = now
	active.RevokedReason = "retry_rescued"
	active.ReplacedByHash = storage.RefreshTokenReplacementHash(successor.Token)
	if trackLastUsed {
		active.LastUsedAt = now
	}

	staleModel := refreshTokenModelFromStorage(stale)
	activeModel := refreshTokenModelFromStorage(active)
	if err := staleModel.UpdateKeys(); err != nil {
		return fmt.Errorf("prepare stale refresh transaction item: %w", err)
	}
	if err := activeModel.UpdateKeys(); err != nil {
		return fmt.Errorf("prepare active refresh transaction item: %w", err)
	}
	successorModel := refreshTokenModelFromStorage(successor)
	if err := successorModel.BeforeCreate(); err != nil {
		return fmt.Errorf("prepare retry successor transaction item: %w", err)
	}
	staleConditions := append(refreshTokenVersionConditions(staleVersion),
		tabletheory.IfExists(),
		tabletheory.Condition("RetryRedeemedAt", "attribute_not_exists", nil),
	)
	activeConditions := append(refreshTokenVersionConditions(activeVersion),
		tabletheory.IfExists(),
		tabletheory.Condition("Current", "=", true),
		tabletheory.Condition("Revoked", "=", false),
	)

	err := r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.UpdateWithBuilder(staleModel, func(update core.UpdateBuilder) error {
			update.Set("RetryRedeemedAt", now).Increment("Version")
			return nil
		}, staleConditions...)
		tx.UpdateWithBuilder(activeModel, func(update core.UpdateBuilder) error {
			update.Set("Current", false).
				Set("Revoked", true).
				Set("RevokedAt", now).
				Set("RevokedReason", active.RevokedReason).
				Set("ReplacedByHash", active.ReplacedByHash).
				Increment("Version")
			if trackLastUsed {
				update.Set("LastUsedAt", now)
			}
			return nil
		}, activeConditions...)
		tx.Create(successorModel, tabletheory.IfNotExists())
		return nil
	})
	if err != nil {
		return fmt.Errorf("redeem refresh retry: %w", err)
	}

	stale.Version = staleVersion + 1
	active.Version = activeVersion + 1
	return nil
}

func refreshTokenVersionConditions(version int) []core.TransactCondition {
	if version == 0 {
		// Legacy rows predate the version attribute. Keep their adoption inside
		// the rotation transaction: ADD/Increment creates version=1 only if the
		// predecessor still exists and still has no version. A separate seed
		// UpdateItem could recreate a row deleted concurrently by /oauth/revoke.
		// TableTheory v3.0.4 supports raw conditions on transactional
		// UpdateWithBuilder operations, whereas this OR/missing-attribute case is
		// not delegated to the query-path UpdateBuilder.
		return []core.TransactCondition{tabletheory.ConditionExpression("attribute_not_exists(version)", nil)}
	}
	return []core.TransactCondition{tabletheory.AtVersion(int64(version))}
}

func transactionConditionFailedAt(err error, operationIndex int) bool {
	if err == nil || !errors.Is(err, tableerrors.ErrConditionFailed) {
		return false
	}
	var txErr *tableerrors.TransactionError
	return errors.As(err, &txErr) && txErr.OperationIndex == operationIndex
}
