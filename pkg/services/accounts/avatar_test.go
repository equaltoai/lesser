package accounts

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

const avatarTestDomain = "example.com"

type avatarTestState struct {
	mu    sync.Mutex
	user  *models.User
	actor *models.Actor
}

type avatarTestDB struct{ state *avatarTestState }

func newAvatarTestDB(user *models.User, actor *models.Actor) *avatarTestDB {
	return &avatarTestDB{state: &avatarTestState{user: user, actor: actor}}
}

func (db *avatarTestDB) Model(model any) dynamormcore.Query {
	return &avatarTestQuery{state: db.state, model: model, wheres: make(map[string]any)}
}

func (db *avatarTestDB) Migrate() error           { return nil }
func (db *avatarTestDB) AutoMigrate(...any) error { return nil }
func (db *avatarTestDB) Close() error             { return nil }
func (db *avatarTestDB) WithContext(context.Context) dynamormcore.DB {
	return db
}

type avatarTestQuery struct {
	state  *avatarTestState
	model  any
	wheres map[string]any
}

func (q *avatarTestQuery) Where(field, _ string, value any) dynamormcore.Query {
	q.wheres[field] = value
	return q
}

func (q *avatarTestQuery) Index(string) dynamormcore.Query                 { return q }
func (q *avatarTestQuery) Filter(string, string, any) dynamormcore.Query   { return q }
func (q *avatarTestQuery) OrFilter(string, string, any) dynamormcore.Query { return q }
func (q *avatarTestQuery) FilterGroup(func(dynamormcore.Query)) dynamormcore.Query {
	return q
}
func (q *avatarTestQuery) OrFilterGroup(func(dynamormcore.Query)) dynamormcore.Query { return q }
func (q *avatarTestQuery) IfNotExists() dynamormcore.Query                           { return q }
func (q *avatarTestQuery) IfExists() dynamormcore.Query                              { return q }
func (q *avatarTestQuery) WithCondition(string, string, any) dynamormcore.Query      { return q }
func (q *avatarTestQuery) WithConditionExpression(string, map[string]any) dynamormcore.Query {
	return q
}
func (q *avatarTestQuery) OrderBy(string, string) dynamormcore.Query { return q }
func (q *avatarTestQuery) Limit(int) dynamormcore.Query              { return q }
func (q *avatarTestQuery) Offset(int) dynamormcore.Query             { return q }
func (q *avatarTestQuery) Select(...string) dynamormcore.Query       { return q }
func (q *avatarTestQuery) ConsistentRead() dynamormcore.Query        { return q }
func (q *avatarTestQuery) WithRetry(int, time.Duration) dynamormcore.Query {
	return q
}
func (q *avatarTestQuery) All(any) error { return errors.New("unsupported") }
func (q *avatarTestQuery) AllPaginated(any) (*dynamormcore.PaginatedResult, error) {
	return nil, errors.New("unsupported")
}
func (q *avatarTestQuery) Count() (int64, error)                        { return 0, errors.New("unsupported") }
func (q *avatarTestQuery) CreateOrUpdate() error                        { return nil }
func (q *avatarTestQuery) Update(...string) error                       { return nil }
func (q *avatarTestQuery) Scan(any) error                               { return errors.New("unsupported") }
func (q *avatarTestQuery) ParallelScan(int32, int32) dynamormcore.Query { return q }
func (q *avatarTestQuery) ScanAllSegments(any, int32) error             { return nil }
func (q *avatarTestQuery) BatchGet([]any, any) error                    { return nil }
func (q *avatarTestQuery) BatchGetWithOptions([]any, any, *dynamormcore.BatchGetOptions) error {
	return nil
}
func (q *avatarTestQuery) BatchGetBuilder() dynamormcore.BatchGetBuilder { return nil }
func (q *avatarTestQuery) BatchCreate(any) error                         { return nil }
func (q *avatarTestQuery) BatchDelete([]any) error                       { return nil }
func (q *avatarTestQuery) BatchWrite([]any, []any) error                 { return nil }
func (q *avatarTestQuery) BatchUpdateWithOptions([]any, []string, ...any) error {
	return nil
}
func (q *avatarTestQuery) Cursor(string) dynamormcore.Query               { return q }
func (q *avatarTestQuery) SetCursor(string) error                         { return nil }
func (q *avatarTestQuery) WithContext(context.Context) dynamormcore.Query { return q }
func (q *avatarTestQuery) Create() error                                  { return nil }
func (q *avatarTestQuery) Delete() error                                  { return nil }

func (q *avatarTestQuery) First(dest any) error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	switch reflect.TypeOf(dest).String() {
	case "*repositories.userCoreProjection":
		if q.state.user == nil {
			return dynamormerrors.ErrItemNotFound
		}
		return copyAvatarFields(dest, q.state.user)
	case "*repositories.userMetadataProjection":
		setAvatarField(dest, "Metadata", map[string]interface{}{})
		return nil
	case "*models.User":
		target, ok := dest.(*models.User)
		if !ok || q.state.user == nil {
			return dynamormerrors.ErrItemNotFound
		}
		*target = *q.state.user
		return nil
	case "*models.Actor":
		target, ok := dest.(*models.Actor)
		if !ok || q.state.actor == nil {
			return dynamormerrors.ErrItemNotFound
		}
		*target = cloneAvatarActorModel(q.state.actor)
		return nil
	default:
		return dynamormerrors.ErrItemNotFound
	}
}

func (q *avatarTestQuery) UpdateBuilder() dynamormcore.UpdateBuilder {
	return &avatarTestUpdateBuilder{state: q.state, model: q.model, sets: make(map[string]any)}
}

type avatarTestUpdateBuilder struct {
	state *avatarTestState
	model any
	sets  map[string]any
}

func (b *avatarTestUpdateBuilder) Set(field string, value any) dynamormcore.UpdateBuilder {
	b.sets[field] = value
	return b
}

func (b *avatarTestUpdateBuilder) SetIfNotExists(string, any, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) Add(string, any) dynamormcore.UpdateBuilder    { return b }
func (b *avatarTestUpdateBuilder) Increment(string) dynamormcore.UpdateBuilder   { return b }
func (b *avatarTestUpdateBuilder) Decrement(string) dynamormcore.UpdateBuilder   { return b }
func (b *avatarTestUpdateBuilder) Remove(string) dynamormcore.UpdateBuilder      { return b }
func (b *avatarTestUpdateBuilder) Delete(string, any) dynamormcore.UpdateBuilder { return b }
func (b *avatarTestUpdateBuilder) AppendToList(string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) PrependToList(string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) RemoveFromListAt(string, int) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) SetListElement(string, int, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) Condition(string, string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) OrCondition(string, string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) ConditionExists(string) dynamormcore.UpdateBuilder { return b }
func (b *avatarTestUpdateBuilder) ConditionNotExists(string) dynamormcore.UpdateBuilder {
	return b
}
func (b *avatarTestUpdateBuilder) ConditionVersion(int64) dynamormcore.UpdateBuilder { return b }
func (b *avatarTestUpdateBuilder) ReturnValues(string) dynamormcore.UpdateBuilder    { return b }
func (b *avatarTestUpdateBuilder) ExecuteWithResult(any) error                       { return b.Execute() }

func (b *avatarTestUpdateBuilder) Execute() error {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	switch reflect.TypeOf(b.model).String() {
	case "*models.User":
		if b.state.user == nil {
			return dynamormerrors.ErrItemNotFound
		}
		applyAvatarSets(b.state.user, b.sets)
		return nil
	case "*repositories.actorProfileUpdateProjection":
		if b.state.actor == nil {
			return dynamormerrors.ErrItemNotFound
		}
		if value, ok := b.sets["Actor"]; ok {
			if actor := avatarSetActor(value); actor != nil {
				b.state.actor.Actor = actor
			}
		}
		return nil
	default:
		return nil
	}
}

func avatarSetActor(value any) *activitypub.Actor {
	field := reflect.ValueOf(value).FieldByName("Actor")
	if !field.IsValid() {
		return nil
	}
	actor, _ := field.Interface().(*activitypub.Actor)
	return actor
}

func applyAvatarSets(target any, sets map[string]any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return
	}
	elem := value.Elem()
	for name, set := range sets {
		field := elem.FieldByName(name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		incoming := reflect.ValueOf(set)
		if incoming.IsValid() && incoming.Type().AssignableTo(field.Type()) {
			field.Set(incoming)
		}
	}
}

func copyAvatarFields(dst, src any) error {
	dstValue := reflect.ValueOf(dst)
	srcValue := reflect.ValueOf(src)
	if dstValue.Kind() != reflect.Ptr || dstValue.IsNil() ||
		srcValue.Kind() != reflect.Ptr || srcValue.IsNil() {
		return errors.New("copyAvatarFields requires pointers")
	}
	dstElem := dstValue.Elem()
	srcElem := srcValue.Elem()
	for i := 0; i < dstElem.NumField(); i++ {
		field := dstElem.Field(i)
		if !field.CanSet() {
			continue
		}
		source := srcElem.FieldByName(dstElem.Type().Field(i).Name)
		if source.IsValid() && source.Type().AssignableTo(field.Type()) {
			field.Set(source)
		}
	}
	return nil
}

func setAvatarField(target any, name string, value any) {
	field := reflect.ValueOf(target).Elem().FieldByName(name)
	if field.IsValid() && field.CanSet() {
		field.Set(reflect.ValueOf(value))
	}
}

func cloneAvatarActorModel(model *models.Actor) models.Actor {
	clone := *model
	if model.Actor != nil {
		actor := *model.Actor
		clone.Actor = &actor
	}
	return clone
}

type avatarTestStorage struct {
	*MockRepositoryStorage
	account *repositories.AccountRepository
	actor   interfaces.ActorRepository
}

func (s *avatarTestStorage) Account() *repositories.AccountRepository { return s.account }
func (s *avatarTestStorage) Actor() interfaces.ActorRepository        { return s.actor }

func avatarTestLocalActor(username, domain, iconURL string) *activitypub.Actor {
	base := fmt.Sprintf("https://%s/users/%s", domain, username)
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.PersonType,
			ID:      base,
		},
		PreferredUsername: username,
		Name:              username,
		Summary:           "bio",
		URL:               fmt.Sprintf("https://%s/@%s", domain, username),
		Inbox:             base + "/inbox",
		Outbox:            base + "/outbox",
		Followers:         base + "/followers",
		Following:         base + "/following",
		Liked:             base + "/liked",
		Endpoints:         &activitypub.Endpoints{SharedInbox: fmt.Sprintf("https://%s/inbox", domain)},
		Discoverable:      true,
	}
	if iconURL != "" {
		actor.Icon = &activitypub.Image{URL: iconURL, MediaType: "image/png"}
	}
	return actor
}

func newAvatarAccountsService(t *testing.T, avatarURL, avatarID string) (*Service, *avatarTestDB) {
	t.Helper()

	logger := zap.NewNop()
	user := &models.User{
		Username: "alice",
		Approved: true,
		Role:     "user",
		Version:  1,
		Avatar:   avatarURL,
		AvatarID: avatarID,
	}
	require.NoError(t, user.UpdateKeys())

	actorModel := &models.Actor{
		Username: "alice",
		Actor:    avatarTestLocalActor("alice", avatarTestDomain, avatarURL),
		Version:  1,
	}
	require.NoError(t, actorModel.UpdateKeys())

	db := newAvatarTestDB(user, actorModel)
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

	svc := NewService(storage, streaming.NewMockPublisher(), nil, nil, nil, logger, avatarTestDomain)
	return svc, db
}

func TestSetAvatar_WritesUserAndActorIcon(t *testing.T) {
	svc, _ := newAvatarAccountsService(t, "", "")
	ctx := context.Background()
	avatarID := "550e8400-e29b-41d4-a716-446655440000"
	expectedURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, avatarID)

	result, err := svc.SetAvatar(ctx, &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    avatarID,
		ContentType: "image/png",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Account)
	require.NotNil(t, result.Account.User)
	require.NotNil(t, result.Account.Actor)

	assert.Equal(t, expectedURL, result.Account.User.Avatar)
	assert.Equal(t, avatarID, result.Account.User.AvatarID)
	require.NotNil(t, result.Account.Actor.Icon)
	assert.Equal(t, expectedURL, result.Account.Actor.Icon.URL)
	assert.Equal(t, "image/png", result.Account.Actor.Icon.MediaType)

	persisted, err := svc.GetAccount(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, expectedURL, persisted.User.Avatar)
	assert.Equal(t, avatarID, persisted.User.AvatarID, "the minted avatar id must be recorded on the account's own row")
	require.NotNil(t, persisted.Actor.Icon)
	assert.Equal(t, expectedURL, persisted.Actor.Icon.URL)
	assert.Equal(t, "image/png", persisted.Actor.Icon.MediaType)
}

func TestSetAvatar_RejectsInvalidAvatarID(t *testing.T) {
	svc, _ := newAvatarAccountsService(t, "", "")

	_, err := svc.SetAvatar(context.Background(), &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    "../../etc/passwd",
		ContentType: "image/png",
	})
	assert.ErrorIs(t, err, ErrInvalidAvatarID)
}

func TestSetAvatar_ReportsThePreviousRecordedIDOnReupload(t *testing.T) {
	previousID := "550e8400-e29b-41d4-a716-446655440000"
	previousURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, previousID)
	svc, _ := newAvatarAccountsService(t, previousURL, previousID)
	ctx := context.Background()
	nextID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	result, err := svc.SetAvatar(ctx, &SetAvatarCommand{
		Username:    "alice",
		AvatarID:    nextID,
		ContentType: "image/png",
	})
	require.NoError(t, err)
	assert.Equal(t, previousID, result.PreviousAvatarID, "a re-upload must hand back the id this account previously recorded")
	assert.Equal(t, nextID, result.Account.User.AvatarID)

	persisted, err := svc.GetAccount(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, nextID, persisted.User.AvatarID)
}

func TestClearAvatar_ClearsUserAndActorIcon(t *testing.T) {
	oldID := "550e8400-e29b-41d4-a716-446655440000"
	oldURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, oldID)
	svc, _ := newAvatarAccountsService(t, oldURL, oldID)
	ctx := context.Background()

	before, err := svc.GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, oldURL, before.User.Avatar)
	require.Equal(t, oldID, before.User.AvatarID)
	require.NotNil(t, before.Actor)
	require.NotNil(t, before.Actor.Icon)
	require.Equal(t, oldURL, before.Actor.Icon.URL)

	result, err := svc.ClearAvatar(ctx, &ClearAvatarCommand{Username: "alice"})
	require.NoError(t, err)
	require.NotNil(t, result.Account)
	assert.Equal(t, oldID, result.PreviousAvatarID)
	assert.Empty(t, result.Account.User.Avatar)
	assert.Empty(t, result.Account.User.AvatarID)
	assert.Nil(t, result.Account.Actor.Icon)

	persisted, err := svc.GetAccount(ctx, "alice")
	require.NoError(t, err)
	assert.Empty(t, persisted.User.Avatar)
	assert.Empty(t, persisted.User.AvatarID)
	require.NotNil(t, persisted.Actor)
	assert.Nil(t, persisted.Actor.Icon)
}

// TestClearAvatar_WithoutARecordedIDAuthorizesNoDeletion is the cross-user
// deletion regression: a row that predates the recorded avatar id can hold
// another account's served avatar URL in user.Avatar, and clearing it must still
// yield no id for the handler to delete. Deletion is authorized only by the id
// recorded on the account's own row, never by parsing a stored URL.
func TestClearAvatar_WithoutARecordedIDAuthorizesNoDeletion(t *testing.T) {
	victimID := "11111111-2222-4333-8444-555555555555"
	victimURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, victimID)

	svc, _ := newAvatarAccountsService(t, victimURL, "")
	ctx := context.Background()

	result, err := svc.ClearAvatar(ctx, &ClearAvatarCommand{Username: "alice"})
	require.NoError(t, err)
	assert.Empty(t, result.PreviousAvatarID,
		"a row with no recorded avatar id must authorize no deletion, even when its stored URL is avatar-serve shaped")
	assert.Empty(t, result.Account.User.Avatar)

	persisted, err := svc.GetAccount(ctx, "alice")
	require.NoError(t, err)
	assert.Empty(t, persisted.User.Avatar)
	assert.Empty(t, persisted.User.AvatarID)
}

// TestClearAvatar_IgnoresAMalformedRecordedID keeps the recorded id honest: a
// value that is not a lowercase avatar id authorizes nothing.
func TestClearAvatar_IgnoresAMalformedRecordedID(t *testing.T) {
	victimID := "11111111-2222-4333-8444-555555555555"
	victimURL := fmt.Sprintf("https://%s/api/v1/avatars/%s", avatarTestDomain, victimID)

	svc, _ := newAvatarAccountsService(t, victimURL, "../../"+victimID)
	ctx := context.Background()

	result, err := svc.ClearAvatar(ctx, &ClearAvatarCommand{Username: "alice"})
	require.NoError(t, err)
	assert.Empty(t, result.PreviousAvatarID)
}

// TestUpdateProfile_RejectsAnAvatarURL is the service-level half of the ruling
// that a credentials/profile update never carries an avatar URL. Every caller
// shares this gate, including the streaming update_profile command.
func TestUpdateProfile_RejectsAnAvatarURL(t *testing.T) {
	svc, _ := newAvatarAccountsService(t, "", "")
	ctx := context.Background()

	_, err := svc.UpdateProfile(ctx, &UpdateProfileCommand{
		Username:  "alice",
		UpdaterID: "alice",
		Avatar:    "https://example.com/api/v1/avatars/11111111-2222-4333-8444-555555555555",
	})
	require.ErrorIs(t, err, ErrAvatarURLNotAccepted)

	persisted, err := svc.GetAccount(ctx, "alice")
	require.NoError(t, err)
	assert.Empty(t, persisted.User.Avatar)
	assert.Empty(t, persisted.User.AvatarID)
}
