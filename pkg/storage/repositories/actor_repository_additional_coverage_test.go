package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type stubActorRepoDeps struct {
	followingByUser map[string][]string
	prefs           map[string]map[string]any
	getFollowingErr error
	setPrefErr      error
}

func (s *stubActorRepoDeps) GetFollowing(ctx context.Context, username string, _ int, _ string) ([]string, string, error) {
	if s.getFollowingErr != nil {
		return nil, "", s.getFollowingErr
	}
	return s.followingByUser[username], "", nil
}

func (s *stubActorRepoDeps) GetPreference(_ context.Context, username, key string) (any, error) {
	if s.prefs == nil {
		return nil, nil
	}
	return s.prefs[username][key], nil
}

func (s *stubActorRepoDeps) SetPreference(_ context.Context, username, key string, value any) error {
	if s.setPrefErr != nil {
		return s.setPrefErr
	}
	if s.prefs == nil {
		s.prefs = map[string]map[string]any{}
	}
	if s.prefs[username] == nil {
		s.prefs[username] = map[string]any{}
	}
	s.prefs[username][key] = value
	return nil
}

type actorUpdateBuilder struct {
	execErr error
}

func (b *actorUpdateBuilder) Set(string, any) core.UpdateBuilder                 { return b }
func (b *actorUpdateBuilder) SetIfNotExists(string, any, any) core.UpdateBuilder { return b }
func (b *actorUpdateBuilder) Add(string, any) core.UpdateBuilder                 { return b }
func (b *actorUpdateBuilder) AddAll(string, any) core.UpdateBuilder              { return b }
func (b *actorUpdateBuilder) Increment(string) core.UpdateBuilder                { return b }
func (b *actorUpdateBuilder) Decrement(string) core.UpdateBuilder                { return b }
func (b *actorUpdateBuilder) Remove(string) core.UpdateBuilder                   { return b }
func (b *actorUpdateBuilder) Delete(string, any) core.UpdateBuilder              { return b }
func (b *actorUpdateBuilder) AppendToList(string, any) core.UpdateBuilder        { return b }
func (b *actorUpdateBuilder) PrependToList(string, any) core.UpdateBuilder       { return b }
func (b *actorUpdateBuilder) RemoveFromListAt(string, int) core.UpdateBuilder    { return b }
func (b *actorUpdateBuilder) SetListElement(string, int, any) core.UpdateBuilder { return b }
func (b *actorUpdateBuilder) Condition(string, string, any) core.UpdateBuilder   { return b }
func (b *actorUpdateBuilder) OrCondition(string, string, any) core.UpdateBuilder { return b }
func (b *actorUpdateBuilder) ConditionExists(string) core.UpdateBuilder          { return b }
func (b *actorUpdateBuilder) ConditionNotExists(string) core.UpdateBuilder       { return b }
func (b *actorUpdateBuilder) ConditionVersion(int64) core.UpdateBuilder          { return b }
func (b *actorUpdateBuilder) ReturnValues(string) core.UpdateBuilder             { return b }
func (b *actorUpdateBuilder) Execute() error                                     { return b.execErr }
func (b *actorUpdateBuilder) ExecuteWithResult(any) error                        { return b.execErr }

func TestActorRepository_helper_functions_and_conversions(t *testing.T) {
	verified := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	out := convertActorFields([]models.ActorField{
		{Name: "n1", Value: "v1", VerifiedAt: &verified},
		{Name: "n2", Value: "v2", VerifiedAt: nil},
	})
	require.Len(t, out, 2)
	assert.Equal(t, verified, out[0].VerifiedAt)
	assert.True(t, out[1].VerifiedAt.IsZero())

	back := convertStorageActorFields([]storage.ActorField{
		{Name: "n1", Value: "v1", VerifiedAt: verified},
		{Name: "n2", Value: "v2"},
	})
	require.Len(t, back, 2)
	assert.NotNil(t, back[0].VerifiedAt)
	assert.Nil(t, back[1].VerifiedAt)

	assert.Equal(t, "USERNAME_SEARCH#al", func() string { pk, _ := buildActorGSI1Keys("Alice"); return pk }())
	assert.Equal(t, "NAME_SEARCH#al", func() string { pk, _ := buildActorGSI2Keys("Alice", "alice"); return pk }())
	assert.Equal(t, "ACTOR_RANK#10K+", func() string { pk, _ := buildActorGSI4Keys(10000, "alice"); return pk }())
	assert.Contains(t, func() string { pk, _ := buildActorGSI5Keys(time.Unix(1, 0), "alice"); return pk }(), "ACTIVE#")

	assert.Equal(t, "10K+", followerCountBucket(10000))
	assert.Equal(t, "1K+", followerCountBucket(1000))
	assert.Equal(t, "100+", followerCountBucket(100))
	assert.Equal(t, "10+", followerCountBucket(10))
	assert.Equal(t, "0-9", followerCountBucket(0))

	logger := zap.NewNop()
	repo := NewActorRepository(nil, "test-table", logger)
	assert.Equal(t, "", repo.resolveActorDomain(nil))
	assert.Equal(t, "example.com", repo.resolveActorDomain(&activitypub.Actor{URL: "https://example.com/users/alice"}))
	assert.Equal(t, "example.net", repo.resolveActorDomain(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.net/users/alice"}}))
	assert.Equal(t, "", repo.resolveActorDomain(&activitypub.Actor{URL: "not a url"}))
}

func TestActorRepository_modelToActivityPubActor(t *testing.T) {
	repo := &ActorRepository{}
	_, err := repo.modelToActivityPubActor(&models.Actor{})
	assert.Error(t, err)

	a := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}
	got, err := repo.modelToActivityPubActor(&models.Actor{Actor: a})
	assert.NoError(t, err)
	assert.Equal(t, a, got)
}

func TestActorRepository_basic_getters_and_update_actor(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()

	// GetActor + GetActorWithMetadata
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err := repo.GetActor(ctx, "alice")
	assert.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice", Discoverable: true}
		m.CreatedAt = time.Now()
		m.UpdatedAt = time.Now()
		m.Fields = []models.ActorField{{Name: "n", Value: "v"}}
	}).Once()
	actor, meta, err := repo.GetActorWithMetadata(ctx, "alice")
	assert.NoError(t, err)
	assert.NotNil(t, actor)
	assert.NotNil(t, meta)
	assert.Len(t, meta.Fields, 1)

	// UpdateActor: version=0 triggers seeding path; update builder path can error
	seedBuilder := &actorUpdateBuilder{execErr: errors.New("ConditionalCheckFailedException")}
	updateBuilder := &actorUpdateBuilder{execErr: nil}
	mockQuery.On("UpdateBuilder").Return(seedBuilder).Once()
	mockQuery.On("UpdateBuilder").Return(updateBuilder).Once()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Username = "alice"
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice", Name: "Alice", URL: "https://example.com/users/alice"}
		m.FollowerCount = 1
		m.Version = 0
	}).Once()

	err = repo.UpdateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice", Name: "Alice"})
	assert.NoError(t, err)

	// Execute failure path
	mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: nil}).Once() // seed
	mockQuery.On("UpdateBuilder").Return(&actorUpdateBuilder{execErr: errors.New("update failed")}).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Username = "alice"
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}
		m.FollowerCount = 1
		m.Version = 0
	}).Once()
	assert.Error(t, repo.UpdateActor(ctx, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}))

	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
}

func TestActorRepository_search_suggestions_and_account_suggestions(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	// getRecentActiveActors: stop after the first query (needs >= limit*2 results)
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob", Discoverable: true}},
			{Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob", Discoverable: true}},
		}
	}).Once()
	actors, err := repo.getRecentActiveActors(ctx, 1)
	assert.NoError(t, err)
	assert.Len(t, actors, 1)

	// GetSearchSuggestions success
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "alice"},
			{Username: "alicia"},
		}
	}).Once()
	suggestions, err := repo.GetSearchSuggestions(ctx, "al")
	assert.NoError(t, err)
	assert.Len(t, suggestions, 2)

	// Account suggestions: deps nil returns empty
	got, err := repo.GetAccountSuggestions(ctx, "alice", 5)
	assert.NoError(t, err)
	assert.Empty(t, got)

	// Account suggestions with deps, including dismissed candidate and fillRemainingSuggestions
	deps := &stubActorRepoDeps{
		followingByUser: map[string][]string{
			"alice": {"https://example.com/users/bob"},
			"bob":   {"https://example.com/users/carla", "https://example.com/users/dan"},
		},
		prefs: map[string]map[string]any{
			"alice": {"dismissed_suggestion:https://example.com/users/dan": true},
		},
	}
	repo.SetDependencies(deps)

	// GetActor for "carla" should succeed and be discoverable
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/carla"}, PreferredUsername: "carla", Discoverable: true}
	}).Once()

	got, err = repo.GetAccountSuggestions(ctx, "alice", 1)
	assert.NoError(t, err)
	assert.NotEmpty(t, got)

	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
}

func TestActorRepository_migration_and_remote_actor_cache(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	// CheckAlsoKnownAs not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err := repo.CheckAlsoKnownAs(ctx, "alice", "target")
	assert.Error(t, err)

	// CheckAlsoKnownAs nil actor payload
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = nil
	}).Once()
	found, err := repo.CheckAlsoKnownAs(ctx, "alice", "target")
	assert.NoError(t, err)
	assert.False(t, found)

	// CheckAlsoKnownAs found
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{AlsoKnownAs: []string{"target"}}
	}).Once()
	found, err = repo.CheckAlsoKnownAs(ctx, "alice", "target")
	assert.NoError(t, err)
	assert.True(t, found)

	// GetActorMigrationInfo nil actor -> error
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
	assert.Equal(t, "b", info.MovedTo)

	// GetCachedRemoteActor expired
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.RemoteActor)
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote/users/alice"}}
		m.ExpiresAt = time.Now().Add(-time.Minute)
	}).Once()
	_, err = repo.GetCachedRemoteActor(ctx, "alice@remote")
	assert.Error(t, err)

	// GetCachedRemoteActor not found, invalid payload, and success
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err = repo.GetCachedRemoteActor(ctx, "bob@remote")
	assert.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.RemoteActor)
		m.Actor = nil
		m.ExpiresAt = time.Now().Add(time.Minute)
	}).Once()
	_, err = repo.GetCachedRemoteActor(ctx, "bad@remote")
	assert.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.RemoteActor)
		m.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "https://remote/users/carla"},
			Inbox:      "https://remote/users/carla/inbox",
		}
		m.ExpiresAt = time.Now().Add(time.Minute)
	}).Once()
	actor, err := repo.GetCachedRemoteActor(ctx, "carla@remote")
	assert.NoError(t, err)
	assert.Equal(t, "https://remote/users/carla", actor.ID)

	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
}

func TestActorRepository_update_migration_fields_and_preferences(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	// UpdateAlsoKnownAs not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	assert.Error(t, repo.UpdateAlsoKnownAs(ctx, "alice", []string{"x"}))

	// UpdateAlsoKnownAs actor payload missing
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = nil
	}).Once()
	assert.Error(t, repo.UpdateAlsoKnownAs(ctx, "alice", []string{"x"}))

	// UpdateAlsoKnownAs update error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{}
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	assert.Error(t, repo.UpdateAlsoKnownAs(ctx, "alice", []string{"x"}))

	// UpdateMovedTo success
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{}
	}).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	assert.NoError(t, repo.UpdateMovedTo(ctx, "alice", "https://remote/users/alice"))

	// RemoveAccountSuggestion dependency errors and success
	assert.Error(t, repo.RemoveAccountSuggestion(ctx, "alice", "bob"))
	repo.SetDependencies(&stubActorRepoDeps{setPrefErr: errors.New("pref failed")})
	assert.Error(t, repo.RemoveAccountSuggestion(ctx, "alice", "bob"))
	repo.SetDependencies(&stubActorRepoDeps{})
	assert.NoError(t, repo.RemoveAccountSuggestion(ctx, "alice", "bob"))
}

func TestActorRepository_private_key_retrieval_error_branches(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")

	cfg := lesserconfig.Get()
	previousKey := cfg.KMSKeyID
	cfg.KMSKeyID = "alias/test"
	defer func() { cfg.KMSKeyID = previousKey }()

	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	// Not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err := repo.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)

	// Invalid base64 (after KMS encryptor loads)
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PrivateKey = "not-base64"
	}).Once()
	_, err = repo.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)
}

func TestActorRepository_numeric_id_and_account_search_queries(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	// GetActorByNumericID mapping not found
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err := repo.GetActorByNumericID(ctx, "123")
	assert.Error(t, err)

	// Mapping found, but actor data missing triggers modelToActivityPubActor error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.NumericIDMapping)
		m.Username = "alice"
	}).Once()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err = repo.GetActorByNumericID(ctx, "123")
	assert.Error(t, err)

	// SearchAccounts username query, then name fallback
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice", Discoverable: true}},
			{Username: "nilactor", Actor: nil},
		}
	}).Once()
	// SearchAccounts now loads full actor records (GSI projections may be KEYS_ONLY).
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Username = "alice"
		m.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			PreferredUsername: "alice",
			Discoverable:      true,
		}
	}).Once()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	results, err := repo.SearchAccounts(ctx, "al", 10, false, 0)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = nil
	}).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{{Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob", Discoverable: true}}}
	}).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Username = "bob"
		m.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/bob"},
			PreferredUsername: "bob",
			Discoverable:      true,
		}
	}).Once()
	results, err = repo.SearchAccounts(ctx, "bo", 10, false, 0)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// GetSearchSuggestions error branch
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()
	_, err = repo.GetSearchSuggestions(ctx, "al")
	assert.Error(t, err)
}

func TestActorRepository_EnsureNumericIDMapping_createsMissingMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			PreferredUsername: "alice",
		}
	}).Once()
	mockQuery.On("Create").Return(nil).Once()

	require.NoError(t, repo.EnsureNumericIDMapping(ctx, "alice"))
}

func TestActorRepository_EnsureNumericIDMapping_skipsBlankAndExistingMapping(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	require.NoError(t, repo.EnsureNumericIDMapping(ctx, "   "))

	mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.NumericIDMapping)
		m.Username = "alice"
	}).Once()

	require.NoError(t, repo.EnsureNumericIDMapping(ctx, "alice"))
}

func TestActorRepository_EnsureNumericIDMapping_returnsLookupError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(errors.New("boom")).Once()

	require.Error(t, repo.EnsureNumericIDMapping(ctx, "alice"))
}

func TestActorRepository_EnsureNumericIDMapping_conditionFailedWithExistingMappingIsIdempotent(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			PreferredUsername: "alice",
		}
	}).Once()
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.NumericIDMapping)
		m.Username = "alice"
	}).Once()

	require.NoError(t, repo.EnsureNumericIDMapping(ctx, "alice"))
}

func TestActorRepository_EnsureNumericIDMapping_normalizesMixedCaseIdentity(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var currentModel any
	var createdMapping *models.NumericIDMapping

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		currentModel = args.Get(0)
	}).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.NumericIDMapping")).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/Agent-0"},
			PreferredUsername: "Agent-0",
		}
	}).Once()
	mockQuery.On("Create").Run(func(mock.Arguments) {
		if mapping, ok := currentModel.(*models.NumericIDMapping); ok {
			copyMapping := *mapping
			createdMapping = &copyMapping
		}
	}).Return(nil).Once()

	repo := NewActorRepository(mockDB, "test-table", logger)
	require.NoError(t, repo.EnsureNumericIDMapping(ctx, "Agent-0"))
	require.NotNil(t, createdMapping)
	require.Equal(t, common.GenerateNumericID("agent-0"), createdMapping.NumericID)
	require.Equal(t, "agent-0", createdMapping.Username)
	require.Equal(t, "https://example.com/users/agent-0", createdMapping.ActorID)
}

func TestActorRepository_misc_methods(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", logger)

	assert.Equal(t, "alice", repo.extractUsernameFromActorID("https://example.com/users/@alice"))
	assert.Equal(t, "bob", repo.extractUsernameFromActorID("@bob"))

	// GetActorByUsername handles missing actor payload via modelToActivityPubActor
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.Actor = nil
	}).Once()
	_, err := repo.GetActorByUsername(ctx, "alice")
	assert.Error(t, err)

	// UpdateActorLastStatusTime + SetActorFields + DeleteActor
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PK = "ACTOR#alice"
		m.SK = "PROFILE"
		m.Username = "alice"
		m.Actor = &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"}
	}).Times(2)
	mockQuery.On("Update", mock.Anything).Return(nil).Twice()

	assert.NoError(t, repo.UpdateActorLastStatusTime(ctx, "alice"))
	assert.NoError(t, repo.SetActorFields(ctx, "alice", []storage.ActorField{{Name: "n", Value: "v"}}))

	mockQuery.On("Delete").Return(nil).Twice()
	assert.NoError(t, repo.DeleteActor(ctx, "alice"))
}
