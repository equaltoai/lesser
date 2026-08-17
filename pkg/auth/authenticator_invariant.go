package auth

import (
	"context"
	stdErrors "errors"
	"strings"

	lessererrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

type authenticatorInventoryRepository interface {
	GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error)
	GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error)
}

type authenticatorRemovalKind string

const (
	authenticatorRemovalPasskey authenticatorRemovalKind = "passkey"
	authenticatorRemovalWallet  authenticatorRemovalKind = "wallet"
)

type authenticatorRemovalPlan struct {
	survivingPasskeyID     string
	survivingWalletAddress string
}

func planAuthenticatorRemoval(
	ctx context.Context,
	repo authenticatorInventoryRepository,
	username string,
	removing authenticatorRemovalKind,
	targetCredentialID string,
	targetWalletAddress string,
) (authenticatorRemovalPlan, error) {
	passkeys, err := repo.GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		return authenticatorRemovalPlan{}, err
	}

	wallets, err := repo.GetUserWalletCredentials(ctx, username)
	if err != nil {
		return authenticatorRemovalPlan{}, err
	}

	switch removing {
	case authenticatorRemovalPasskey:
		for _, passkey := range passkeys {
			if passkey == nil || passkey.ID == targetCredentialID {
				continue
			}
			return authenticatorRemovalPlan{survivingPasskeyID: passkey.ID}, nil
		}
		for _, wallet := range wallets {
			if wallet == nil {
				continue
			}
			return authenticatorRemovalPlan{survivingWalletAddress: wallet.Address}, nil
		}
	case authenticatorRemovalWallet:
		for _, wallet := range wallets {
			if wallet == nil || strings.EqualFold(wallet.Address, targetWalletAddress) {
				continue
			}
			return authenticatorRemovalPlan{survivingWalletAddress: wallet.Address}, nil
		}
		for _, passkey := range passkeys {
			if passkey == nil {
				continue
			}
			return authenticatorRemovalPlan{survivingPasskeyID: passkey.ID}, nil
		}
	default:
		return authenticatorRemovalPlan{}, ErrLastAuthMethodDelete
	}

	return authenticatorRemovalPlan{}, ErrLastAuthMethodDelete
}

func classifyGuardedAuthenticatorRemovalFailure(err error) error {
	if err == nil {
		return nil
	}
	if dynamormerrors.IsConditionFailed(err) || stdErrors.Is(err, dynamormerrors.ErrTransactionConflict) {
		return ErrLastAuthMethodDelete
	}
	return err
}

func classifyGuardedWebAuthnRemovalFailure(
	ctx context.Context,
	repo webAuthnRepository,
	username string,
	credentialID string,
	err error,
) error {
	if err == nil {
		return nil
	}
	if stdErrors.Is(err, dynamormerrors.ErrTransactionConflict) {
		return ErrLastAuthMethodDelete
	}
	if !dynamormerrors.IsConditionFailed(err) {
		return err
	}

	credential, lookupErr := repo.GetWebAuthnCredential(ctx, credentialID)
	if lookupErr != nil {
		if isAuthenticatorRemovalTargetNotFound(lookupErr) {
			return ErrCredentialNotFound
		}
		return lookupErr
	}
	if credential == nil || credential.UserID != username {
		return ErrCredentialNotFound
	}

	return ErrLastAuthMethodDelete
}

func isAuthenticatorRemovalTargetNotFound(err error) bool {
	if err == nil {
		return false
	}
	if lessererrors.HasCode(err, lessererrors.CodeNotFound) || dynamormerrors.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
