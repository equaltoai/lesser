package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActorRepository_more_error_branches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()

	repo := NewActorRepository(mockDB, "test-table", logger)

	// GetActorByUsername success
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice", Discoverable: true}
	}).Once()
	got, err := repo.GetActorByUsername(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, "alice", got.PreferredUsername)

	// GetActorByUsername not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err = repo.GetActorByUsername(ctx, "missing")
	assert.Error(t, err)

	// GetActorWithMetadata non-notfound error path
	mockQuery.On("First", mock.Anything).Return(errors.New("db failed")).Once()
	_, _, err = repo.GetActorWithMetadata(ctx, "alice")
	assert.Error(t, err)

	// UpdateMovedTo update failure path
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Actor = &activitypub.Actor{}
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.UpdateMovedTo(ctx, "alice", "https://remote/users/alice"))

	// UpdateActorLastStatusTime update failure path
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Actor = &activitypub.Actor{}
	}).Once()
	expectActorLastStatusUpdateBuilder(mockQuery, errors.New("update failed"))
	assert.Error(t, repo.UpdateActorLastStatusTime(ctx, "alice"))

	// SetActorFields update failure path
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Actor = &activitypub.Actor{}
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.SetActorFields(ctx, "alice", []storage.ActorField{{Name: "n", Value: "v"}}))

	// DeleteActor generic error path
	mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
	assert.Error(t, repo.DeleteActor(ctx, "alice"))

	// GetActorPrivateKey encryptor missing path
	cfg := lesserconfig.Get()
	previous := cfg.KMSKeyID
	cfg.KMSKeyID = ""
	defer func() { cfg.KMSKeyID = previous }()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PrivateKey = "ignored"
	}).Once()
	_, err = repo.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)

	// UpdateActor: UpdateBuilder Execute error path
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Username = "alice"
		m.Actor = &activitypub.Actor{PreferredUsername: "alice"}
		m.FollowerCount = 0
		m.Version = 1
	}).Once()
	mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: errors.New("exec failed")}).Once()
	assert.Error(t, repo.UpdateActor(ctx, &activitypub.Actor{PreferredUsername: "alice"}))
}
