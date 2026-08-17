package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type authenticatorInventory struct {
	passkeys int
	wallets  int
}

type authenticatorInventoryRepoStub struct {
	passkeys   []*storage.WebAuthnCredential
	wallets    []*storage.WalletCredential
	passkeyErr error
	walletErr  error
}

func (s authenticatorInventoryRepoStub) GetUserWebAuthnCredentials(context.Context, string) ([]*storage.WebAuthnCredential, error) {
	if s.passkeyErr != nil {
		return nil, s.passkeyErr
	}
	return s.passkeys, nil
}

func (s authenticatorInventoryRepoStub) GetUserWalletCredentials(context.Context, string) ([]*storage.WalletCredential, error) {
	if s.walletErr != nil {
		return nil, s.walletErr
	}
	return s.wallets, nil
}

func TestPlanAuthenticatorRemoval_PrefersSameKindSurvivorsAndSurfacesErrors(t *testing.T) {
	t.Parallel()

	t.Run("passkey removal prefers another passkey before wallet", func(t *testing.T) {
		t.Parallel()

		plan, err := planAuthenticatorRemoval(
			context.Background(),
			authenticatorInventoryRepoStub{
				passkeys: []*storage.WebAuthnCredential{{ID: "cred-1"}, {ID: "cred-2"}},
				wallets:  []*storage.WalletCredential{{Address: "0xabc"}},
			},
			"alice",
			authenticatorRemovalPasskey,
			"cred-1",
			"",
		)
		require.NoError(t, err)
		require.Equal(t, authenticatorRemovalPlan{survivingPasskeyID: "cred-2"}, plan)
	})

	t.Run("wallet removal prefers another wallet before passkey", func(t *testing.T) {
		t.Parallel()

		plan, err := planAuthenticatorRemoval(
			context.Background(),
			authenticatorInventoryRepoStub{
				passkeys: []*storage.WebAuthnCredential{{ID: "cred-1"}},
				wallets:  []*storage.WalletCredential{{Address: "0xabc"}, {Address: "0xdef"}},
			},
			"alice",
			authenticatorRemovalWallet,
			"",
			"0xabc",
		)
		require.NoError(t, err)
		require.Equal(t, authenticatorRemovalPlan{survivingWalletAddress: "0xdef"}, plan)
	})

	t.Run("unknown removal kind is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := planAuthenticatorRemoval(
			context.Background(),
			authenticatorInventoryRepoStub{},
			"alice",
			authenticatorRemovalKind("unknown"),
			"",
			"",
		)
		require.ErrorIs(t, err, ErrLastAuthMethodDelete)
	})

	t.Run("wallet inventory errors are surfaced", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		_, err := planAuthenticatorRemoval(
			context.Background(),
			authenticatorInventoryRepoStub{walletErr: boom},
			"alice",
			authenticatorRemovalPasskey,
			"cred-1",
			"",
		)
		require.ErrorIs(t, err, boom)
	})
}

func TestWebAuthnService_DeleteCredential_RejectsLastPasskeyWithoutWallet(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	only := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{only}
	repo.credentialsByID[only.ID] = only

	svc := &WebAuthnService{repo: repo}
	err := svc.DeleteCredential(context.Background(), "alice", only.ID)
	require.ErrorIs(t, err, ErrLastAuthMethodDelete)
}

func TestWalletService_UnlinkWallet_RejectsLastWalletWithoutPasskey(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
	repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]

	svc := &WalletService{repo: repo, logger: zap.NewNop()}
	err := svc.UnlinkWallet(context.Background(), "alice", "0xabc")
	require.ErrorIs(t, err, ErrLastAuthMethodDelete)
}

func TestAuthenticatorInvariant_AllowsRemovalWhenAnotherAuthenticatorRemains(t *testing.T) {
	t.Parallel()

	t.Run("delete_passkey_with_second_passkey_present", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		cred1 := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk1")}
		cred2 := &storage.WebAuthnCredential{ID: "cred-2", UserID: "alice", PublicKey: []byte("pk2")}
		repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{cred1, cred2}
		repo.credentialsByID[cred1.ID] = cred1
		repo.credentialsByID[cred2.ID] = cred2

		svc := &WebAuthnService{repo: repo}
		require.NoError(t, svc.DeleteCredential(context.Background(), "alice", cred1.ID))
	})

	t.Run("unlink_wallet_with_passkey_present", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWalletRepo()
		repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
			{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
		}
		repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
		repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]

		svc := &WalletService{repo: repo, logger: zap.NewNop()}
		require.NoError(t, svc.UnlinkWallet(context.Background(), "alice", "0xabc"))
	})
}

func TestAuthenticatorInvariant_MirroredRemovalRulesStayInSync(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		passkeyScenario authenticatorInventory
		walletScenario  authenticatorInventory
		wantReject      bool
	}{
		{
			name:            "last_authenticator_rejected_in_both_paths",
			passkeyScenario: authenticatorInventory{passkeys: 1, wallets: 0},
			walletScenario:  authenticatorInventory{passkeys: 0, wallets: 1},
			wantReject:      true,
		},
		{
			name:            "alternate_authenticator_present_allows_both_paths",
			passkeyScenario: authenticatorInventory{passkeys: 1, wallets: 1},
			walletScenario:  authenticatorInventory{passkeys: 1, wallets: 1},
			wantReject:      false,
		},
		{
			name:            "same_type_spare_allows_both_paths",
			passkeyScenario: authenticatorInventory{passkeys: 2, wallets: 0},
			walletScenario:  authenticatorInventory{passkeys: 0, wallets: 2},
			wantReject:      false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deleteErr := deletePasskeyForInventory(tc.passkeyScenario)
			unlinkErr := unlinkWalletForInventory(tc.walletScenario)

			require.Equal(t, tc.wantReject, errors.Is(deleteErr, ErrLastAuthMethodDelete))
			require.Equal(t, tc.wantReject, errors.Is(unlinkErr, ErrLastAuthMethodDelete))
		})
	}
}

func deletePasskeyForInventory(inv authenticatorInventory) error {
	repo := newInMemoryWebAuthnRepo()
	for i := 0; i < inv.passkeys; i++ {
		id := fmt.Sprintf("cred-%d", i)
		cred := &storage.WebAuthnCredential{ID: id, UserID: "alice", PublicKey: []byte(id)}
		repo.credentialsByUsername["alice"] = append(repo.credentialsByUsername["alice"], cred)
		repo.credentialsByID[id] = cred
	}
	for i := 0; i < inv.wallets; i++ {
		repo.walletsByUsername["alice"] = append(repo.walletsByUsername["alice"], &storage.WalletCredential{
			Username: "alice",
			Address:  fmt.Sprintf("0xwallet%d", i),
			Type:     "ethereum",
			ChainID:  1,
		})
	}

	svc := &WebAuthnService{repo: repo}
	if len(repo.credentialsByUsername["alice"]) == 0 {
		return nil
	}
	return svc.DeleteCredential(context.Background(), "alice", repo.credentialsByUsername["alice"][0].ID)
}

func unlinkWalletForInventory(inv authenticatorInventory) error {
	repo := newInMemoryWalletRepo()
	for i := 0; i < inv.passkeys; i++ {
		id := fmt.Sprintf("cred-%d", i)
		repo.passkeysByUser["alice"] = append(repo.passkeysByUser["alice"], &storage.WebAuthnCredential{
			ID:        id,
			UserID:    "alice",
			PublicKey: []byte(id),
		})
	}
	for i := 0; i < inv.wallets; i++ {
		address := fmt.Sprintf("0xwallet%d", i)
		wallet := &storage.WalletCredential{Username: "alice", Address: address, Type: "ethereum", ChainID: 1}
		repo.walletsByAddr[address] = wallet
		repo.walletsByUserAddr[walletKey("alice", address)] = wallet
	}

	svc := &WalletService{repo: repo, logger: zap.NewNop()}
	if len(repo.walletsByAddr) == 0 {
		return nil
	}
	return svc.UnlinkWallet(context.Background(), "alice", "0xwallet0")
}
