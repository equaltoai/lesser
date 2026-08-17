package auth

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
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
