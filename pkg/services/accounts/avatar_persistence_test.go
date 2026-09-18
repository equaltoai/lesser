package accounts

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// newAvatarPersistenceService wires the accounts service over a REAL
// AccountRepository driving the real tabletheory update chain against an
// in-memory DynamoDB client. The avatar id therefore travels the same versioned
// UpdateAccount path production uses, so these tests prove the field persists
// rather than proving a hand-rolled fake's semantics.
func newAvatarPersistenceService(t *testing.T) (*Service, *repositories.AccountRepository) {
	t.Helper()

	logger := zap.NewNop()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.User{}))
	require.NoError(t, db.CreateTable(&models.Actor{}))

	user := &models.User{Username: "alice", Role: "user", Approved: true, Version: 1}
	require.NoError(t, user.UpdateKeys())
	require.NoError(t, db.Model(user).Create())

	actor := &models.Actor{
		Username: "alice",
		Actor:    avatarTestLocalActor("alice", avatarTestDomain, ""),
		Version:  1,
	}
	require.NoError(t, actor.UpdateKeys())
	require.NoError(t, db.Model(actor).Create())

	accountRepo := repositories.NewAccountRepository(db, "test-table", avatarTestDomain, logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	storage := &avatarTestStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		account:               accountRepo,
		actor:                 repositories.NewActorRepository(db, "test-table", logger, avatarTestDomain),
	}

	return NewService(storage, streaming.NewMockPublisher(), nil, nil, nil, logger, avatarTestDomain), accountRepo
}

// TestAvatarPersistence_SetThenClearRoundTripsThroughTheRealRepository is the
// set -> re-read -> clear -> re-read proof on real persistence: the recorded
// avatar id is written to the user row, survives a read, authorizes the clear,
// and is emptied by the clear. Every read here goes back through the repository
// rather than reusing the value the service returned.
func TestAvatarPersistence_SetThenClearRoundTripsThroughTheRealRepository(t *testing.T) {
	svc, accountRepo := newAvatarPersistenceService(t)
	ctx := context.Background()
	avatarID := "550e8400-e29b-41d4-a716-446655440000"
	servedURL := "https://" + avatarTestDomain + "/api/v1/avatars/" + avatarID

	_, err := svc.SetAvatar(ctx, &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    avatarID,
		ContentType: "image/png",
	})
	require.NoError(t, err)

	stored, err := accountRepo.GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, stored.User)
	require.Equal(t, avatarID, stored.User.AvatarID, "the minted avatar id must persist to the user row")
	require.Equal(t, servedURL, stored.User.Avatar)
	require.NotNil(t, stored.Actor)
	require.NotNil(t, stored.Actor.Icon)
	require.Equal(t, servedURL, stored.Actor.Icon.URL,
		"the actor icon must persist alongside the user record")

	cleared, err := svc.ClearAvatar(ctx, &ClearAvatarCommand{Username: "alice"})
	require.NoError(t, err)
	require.Equal(t, avatarID, cleared.PreviousAvatarID,
		"the clear must report the id the row recorded, so the handler can delete exactly that object")

	after, err := accountRepo.GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, after.User)
	require.Empty(t, after.User.AvatarID, "the clear must empty the recorded id on the row itself")
	require.Empty(t, after.User.Avatar)
	require.NotNil(t, after.Actor)
	require.Nil(t, after.Actor.Icon, "the clear must empty the stored actor icon")
}

// TestAvatarPersistence_SecondSetReplacesTheRecordedID covers the re-upload
// shape against real persistence: the row records the newly minted id and the
// service reports the previously recorded one for cleanup.
func TestAvatarPersistence_SecondSetReplacesTheRecordedID(t *testing.T) {
	svc, accountRepo := newAvatarPersistenceService(t)
	ctx := context.Background()
	firstID := "550e8400-e29b-41d4-a716-446655440000"
	secondID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	_, err := svc.SetAvatar(ctx, &SetAvatarCommand{Username: "alice", AvatarID: firstID, ContentType: "image/png"})
	require.NoError(t, err)

	second, err := svc.SetAvatar(ctx, &SetAvatarCommand{Username: "alice", AvatarID: secondID, ContentType: "image/png"})
	require.NoError(t, err)
	require.Equal(t, firstID, second.PreviousAvatarID)

	stored, err := accountRepo.GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, secondID, stored.User.AvatarID)
	require.Equal(t, "https://"+avatarTestDomain+"/api/v1/avatars/"+secondID, stored.User.Avatar)
}
