package accounts

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const avatarErrorPathID = "550e8400-e29b-41d4-a716-446655440000"

// TestSetAvatar_RejectsAMissingIdentity keeps the validation ahead of any store
// access: a request with no account to attach the avatar to is the caller's
// mistake, not a missing account.
func TestSetAvatar_RejectsAMissingIdentity(t *testing.T) {
	svc, _ := newAvatarAccountsService(t, "", "")
	ctx := context.Background()

	_, err := svc.SetAvatar(ctx, nil)
	assert.ErrorIs(t, err, ErrValidationFailed)

	_, err = svc.SetAvatar(ctx, &SetAvatarCommand{Username: "   ", AvatarID: avatarErrorPathID})
	assert.ErrorIs(t, err, ErrValidationFailed)
}

// TestSetAvatar_ReportsAnUnreadableAccount separates "the store could not be
// read" from "the account does not exist", so a storage outage is never reported
// as a missing account.
func TestSetAvatar_ReportsAnUnreadableAccount(t *testing.T) {
	svc, db := newAvatarAccountsService(t, "", "")
	db.forgetRows()

	_, err := svc.SetAvatar(context.Background(), &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    avatarErrorPathID,
		ContentType: "image/png",
	})
	assert.ErrorIs(t, err, ErrGetAccount)
}

// TestSetAvatar_ReportsAWriteFailure keeps a rejected write distinct from a
// successful attach: the caller must not be told the avatar is live when the
// account row still carries the old value.
func TestSetAvatar_ReportsAWriteFailure(t *testing.T) {
	svc, db := newAvatarAccountsService(t, "", "")
	db.failWrites(errors.New("conditional check failed"))

	_, err := svc.SetAvatar(context.Background(), &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    avatarErrorPathID,
		ContentType: "image/png",
	})
	assert.ErrorIs(t, err, ErrStoreAccount)
}

// TestSetAvatar_ReuploadingTheSameObjectAuthorizesNoDeletion guards the cleanup
// contract: re-recording the id the row already holds yields no previous id, so
// the handler cannot delete the object the account still references.
func TestSetAvatar_ReuploadingTheSameObjectAuthorizesNoDeletion(t *testing.T) {
	servedURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, avatarErrorPathID)
	svc, _ := newAvatarAccountsService(t, servedURL, avatarErrorPathID)

	result, err := svc.SetAvatar(context.Background(), &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    avatarErrorPathID,
		ContentType: "image/png",
	})
	require.NoError(t, err)
	assert.Empty(t, result.PreviousAvatarID, "the id in use is not an orphan")
	assert.Equal(t, avatarErrorPathID, result.Account.User.AvatarID)
}

// TestClearAvatar_RejectsAMissingIdentity mirrors the set path: a clear with no
// account named is rejected before the store is touched.
func TestClearAvatar_RejectsAMissingIdentity(t *testing.T) {
	svc, _ := newAvatarAccountsService(t, "", "")
	ctx := context.Background()

	_, err := svc.ClearAvatar(ctx, nil)
	assert.ErrorIs(t, err, ErrValidationFailed)

	_, err = svc.ClearAvatar(ctx, &ClearAvatarCommand{Username: "  "})
	assert.ErrorIs(t, err, ErrValidationFailed)
}

// TestClearAvatar_ReportsAnUnreadableAccount keeps the "previous avatar id" the
// caller will delete trustworthy: an account that cannot be read yields no clear
// at all rather than a delete against an unknown row.
func TestClearAvatar_ReportsAnUnreadableAccount(t *testing.T) {
	svc, db := newAvatarAccountsService(t, "", "")
	db.forgetRows()

	_, err := svc.ClearAvatar(context.Background(), &ClearAvatarCommand{Username: "alice"})
	assert.ErrorIs(t, err, ErrGetAccount)
}

// TestClearAvatar_ReportsAWriteFailure keeps a failed clear honest: a rejected
// write is reported as a failed clear, never as a cleared avatar.
func TestClearAvatar_ReportsAWriteFailure(t *testing.T) {
	oldURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, avatarErrorPathID)
	svc, db := newAvatarAccountsService(t, oldURL, avatarErrorPathID)
	db.failWrites(errors.New("conditional check failed"))

	_, err := svc.ClearAvatar(context.Background(), &ClearAvatarCommand{Username: "alice"})
	assert.ErrorIs(t, err, ErrClearAvatar)
}

// TestRecordedAvatarIDOnlyTrustsAWellFormedOwnRecord pins what authorizes a
// deletion: the trimmed id on the account's own row, and nothing else. A record
// with no user, or with an id that is not a lowercase avatar id, authorizes no
// deletion at all.
func TestRecordedAvatarIDOnlyTrustsAWellFormedOwnRecord(t *testing.T) {
	assert.Empty(t, recordedAvatarID(nil))
	assert.Empty(t, recordedAvatarID(&storage.Account{}))
	assert.Empty(t, recordedAvatarID(&storage.Account{User: &storage.User{}}))
	assert.Empty(t, recordedAvatarID(&storage.Account{User: &storage.User{AvatarID: "not-a-uuid"}}))

	recorded := recordedAvatarID(&storage.Account{User: &storage.User{AvatarID: "  " + avatarErrorPathID + "\n"}})
	assert.Equal(t, avatarErrorPathID, recorded, "a padded id is still the recorded id")
}
