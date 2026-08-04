package repositories

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActorRepository_GetActorPrivateKey_base64_decode_error(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Select", mock.Anything).Return(mockQuery)

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PrivateKey = "not-base64"
	}).Once()

	repo := NewActorRepository(mockDB, "test-table", logger)

	cfg := lesserconfig.Get()
	prevKey := cfg.KMSKeyID
	cfg.KMSKeyID = "dummy-kms-key"
	defer func() { cfg.KMSKeyID = prevKey }()

	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	_, err := repo.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private key is not in expected encrypted format")
}

func TestActorRepository_migration_and_suggestion_branches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Select", mock.Anything).Return(mockQuery)

	deps := &stubActorRepoDeps{
		followingByUser: map[string][]string{
			"alice": {"https://example.com/users/bob"},
		},
		prefs: map[string]map[string]any{
			"alice": {
				"dismissed_suggestion:https://example.com/users/bob": true,
			},
		},
	}

	repo := NewActorRepository(mockDB, "test-table", logger)
	repo.SetDependencies(deps)

	// getUserFollowing success path
	gotFollowing, err := repo.getUserFollowing(ctx, "alice", logger)
	assert.NoError(t, err)
	assert.Equal(t, deps.followingByUser["alice"], gotFollowing)

	// shouldSkipCandidate: already follows
	assert.True(t, repo.shouldSkipCandidate(ctx, "alice", "bob", map[string]bool{"bob": true}, map[string]bool{}))

	// shouldSkipCandidate: dismissed suggestion
	assert.True(t, repo.shouldSkipCandidate(ctx, "alice", "https://example.com/users/bob", map[string]bool{}, map[string]bool{}))

	// shouldSkipCandidate: not dismissed
	assert.False(t, repo.shouldSkipCandidate(ctx, "alice", "https://example.com/users/carol", map[string]bool{}, map[string]bool{}))

	// loadActorIfDiscoverable: blank actor ID fails validation
	assert.Nil(t, repo.loadActorIfDiscoverable(ctx, ""))

	// loadActorIfDiscoverable: username extraction yields empty
	assert.Nil(t, repo.loadActorIfDiscoverable(ctx, "https://example.com/users/"))

	// loadActorIfDiscoverable: GetActor not found -> nil
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	assert.Nil(t, repo.loadActorIfDiscoverable(ctx, "https://example.com/users/missing"))

	// loadActorIfDiscoverable: actor not discoverable -> nil
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{PreferredUsername: "hidden", Discoverable: false}
	}).Once()
	assert.Nil(t, repo.loadActorIfDiscoverable(ctx, "https://example.com/users/hidden"))

	// loadActorIfDiscoverable: discoverable -> actor
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{PreferredUsername: "d", Discoverable: true}
	}).Once()
	assert.NotNil(t, repo.loadActorIfDiscoverable(ctx, "https://example.com/users/d"))

	// UpdateAlsoKnownAs not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	assert.Error(t, repo.UpdateAlsoKnownAs(ctx, "missing", []string{"x"}))

	// UpdateAlsoKnownAs actor missing
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Actor = nil
	}).Once()
	assert.Error(t, repo.UpdateAlsoKnownAs(ctx, "alice", []string{"x"}))

	// UpdateMovedTo update error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Actor = &activitypub.Actor{}
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.UpdateMovedTo(ctx, "alice", "https://remote/users/alice"))

	// CheckAlsoKnownAs not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err = repo.CheckAlsoKnownAs(ctx, "missing", "x")
	assert.Error(t, err)

	// CheckAlsoKnownAs actor nil -> false,nil
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = nil
	}).Once()
	found, err := repo.CheckAlsoKnownAs(ctx, "alice", "x")
	assert.NoError(t, err)
	assert.False(t, found)

	// CheckAlsoKnownAs match -> true,nil
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{AlsoKnownAs: []string{"https://remote/users/alice"}}
	}).Once()
	found, err = repo.CheckAlsoKnownAs(ctx, "alice", "https://remote/users/alice")
	assert.NoError(t, err)
	assert.True(t, found)

	// GetActorMigrationInfo actor nil -> error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = nil
	}).Once()
	_, err = repo.GetActorMigrationInfo(ctx, "alice")
	assert.Error(t, err)

	// GetActorMigrationInfo success
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{AlsoKnownAs: []string{"a"}, MovedTo: "b"}
	}).Once()
	info, err := repo.GetActorMigrationInfo(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, []string{"a"}, info.AlsoKnownAs)
	assert.Equal(t, "b", info.MovedTo)

	// Ensure we didn't accidentally leak real AWS env for subsequent tests
	_ = os.Unsetenv("AWS_PROFILE")
}
