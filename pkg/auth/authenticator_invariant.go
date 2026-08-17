package auth

import (
	"context"

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

func ensureAuthenticatorRemovalAllowed(
	ctx context.Context,
	repo authenticatorInventoryRepository,
	username string,
	removing authenticatorRemovalKind,
) error {
	passkeys, err := repo.GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		return err
	}

	wallets, err := repo.GetUserWalletCredentials(ctx, username)
	if err != nil {
		return err
	}

	passkeyCount := len(passkeys)
	walletCount := len(wallets)

	switch removing {
	case authenticatorRemovalPasskey:
		if passkeyCount == 1 && walletCount == 0 {
			return ErrLastAuthMethodDelete
		}
	case authenticatorRemovalWallet:
		if walletCount == 1 && passkeyCount == 0 {
			return ErrLastAuthMethodDelete
		}
	}

	return nil
}
