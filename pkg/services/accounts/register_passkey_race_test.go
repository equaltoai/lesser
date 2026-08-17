package accounts

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

func TestService_RegisterAccount_WithPasskeyRegistrationProof_ConcurrentSameProofPreservesWinner(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(2)
	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-1",
		Username:        "alice",
		CeremonyID:      "ceremony-1",
		CredentialID:    "cred-1",
		PublicKey:       []byte("credential-public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       11,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	services := []*Service{
		newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY A", "PRIVATE KEY A"),
		newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY B", "PRIVATE KEY B"),
	}

	type result struct {
		account *RegisterAccountResult
		err     error
	}
	results := make([]result, len(services))

	var wg sync.WaitGroup
	wg.Add(len(services))
	for i, svc := range services {
		i := i
		svc := svc
		go func() {
			defer wg.Done()
			results[i].account, results[i].err = svc.RegisterAccount(ctx, &RegisterAccountCommand{
				Username:                 "alice",
				Agreement:                true,
				Locale:                   "en",
				PasskeyRegistrationProof: "proof-1",
			})
		}()
	}
	wg.Wait()

	var winner *RegisterAccountResult
	var loserErr error
	for _, res := range results {
		if res.err == nil {
			require.Nil(t, winner, "expected exactly one successful registration")
			winner = res.account
			continue
		}
		require.Nil(t, loserErr, "expected exactly one failed registration")
		loserErr = res.err
	}

	require.NotNil(t, winner, "one registration must succeed")
	require.Error(t, loserErr, "one registration must fail cleanly")
	require.True(t,
		strings.Contains(loserErr.Error(), "already exists") || strings.Contains(loserErr.Error(), "already taken"),
		"expected duplicate registration loser to fail cleanly, got %q", loserErr.Error(),
	)

	storedAccount, err := accountRepo.GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, storedAccount)
	require.NotNil(t, storedAccount.Actor)
	require.NotNil(t, storedAccount.Actor.PublicKey)
	require.Equal(t, winner.Actor.PublicKey.PublicKeyPem, storedAccount.Actor.PublicKey.PublicKeyPem)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, credentials, 1)
	require.Equal(t, "cred-1", credentials[0].ID)
	require.Equal(t, "alice", credentials[0].UserID)
	require.Equal(t, []byte("credential-public-key"), credentials[0].PublicKey)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-1")
	require.NoError(t, err)
	require.True(t, proof.Consumed)
	require.Equal(t, 1, db.consumeSuccessCount())
}

func TestService_RegisterAccount_WithPasskeyRegistrationProof_RejectsDifferentUsernameBeforeCreate(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(0)
	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-1",
		Username:        "alice",
		CeremonyID:      "ceremony-1",
		CredentialID:    "cred-1",
		PublicKey:       []byte("credential-public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       11,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	svc := newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY", "PRIVATE KEY")

	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "mallory",
		Agreement:                true,
		Locale:                   "en",
		PasskeyRegistrationProof: "proof-1",
	})
	require.Nil(t, got)
	require.Error(t, err)
	require.Contains(t, err.Error(), "different username")

	_, err = accountRepo.GetAccount(ctx, "mallory")
	require.Error(t, err)
	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "mallory")
	require.NoError(t, err)
	require.Empty(t, credentials)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-1")
	require.NoError(t, err)
	require.False(t, proof.Consumed)
}

func TestService_RegisterAccount_WithPasskeyRegistrationProof_RejectsCaseDifferingUsernameBeforeCreate(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(0)
	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-1",
		Username:        "Alice",
		CeremonyID:      "ceremony-1",
		CredentialID:    "cred-1",
		PublicKey:       []byte("credential-public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       11,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	svc := newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY", "PRIVATE KEY")

	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "alice",
		Agreement:                true,
		Locale:                   "en",
		PasskeyRegistrationProof: "proof-1",
	})
	require.Nil(t, got)
	require.Error(t, err)
	require.Contains(t, err.Error(), "different username")

	_, err = accountRepo.GetAccount(ctx, "alice")
	require.Error(t, err)
	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "alice")
	require.NoError(t, err)
	require.Empty(t, credentials)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-1")
	require.NoError(t, err)
	require.False(t, proof.Consumed)
}

func newRegistrationPasskeyTestStorage(t *testing.T, db dynamormcore.DB, logger *zap.Logger) (*repositories.AccountRepository, *permissiveAccountsStorage) {
	t.Helper()

	accountRepo := repositories.NewAccountRepository(db, "test-table", "example.com", logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	quoteRepo := repositories.NewQuoteRepository(db, "test-table", logger, nil)
	activityRepo := testmocks.NewMockActivityRepository()
	activityRepo.On("RecordActivity", mock.Anything, "registration", mock.Anything, mock.Anything).Return(nil).Maybe()

	storageImpl := &permissiveAccountsStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		db:                    db,
		tableName:             "test-table",
		logger:                logger,
		account:               accountRepo,
		quote:                 quoteRepo,
		activity:              activityRepo,
	}

	return accountRepo, storageImpl
}

func newRegistrationPasskeyTestService(storageImpl *permissiveAccountsStorage, logger *zap.Logger, publicKeyPEM string, privateKeyPEM string) *Service {
	return NewService(
		storageImpl,
		streaming.NewMockPublisher(),
		nil,
		staticCryptoService{
			publicKeyPEM:  []byte(publicKeyPEM),
			privateKeyPEM: []byte(privateKeyPEM),
			key:           struct{}{},
		},
		staticAuthService{hash: "hash"},
		logger,
		"example.com",
	)
}

func storeRegistrationWalletChallenge(t *testing.T, repo *repositories.AccountRepository, username string, challengeID string) {
	t.Helper()

	baseTime := time.Now().UTC()
	require.NoError(t, repo.StoreWalletChallenge(context.Background(), &storage.WalletChallenge{
		ID:        challengeID,
		Username:  username,
		Address:   "0xabc",
		ChainID:   1,
		Nonce:     "nonce",
		Message:   "message",
		IssuedAt:  baseTime,
		ExpiresAt: baseTime.Add(time.Hour),
	}))
}

type registrationPasskeyBarrier struct {
	mu      sync.Mutex
	release chan struct{}
	target  int
	arrived int
}

func newRegistrationPasskeyBarrier(target int) *registrationPasskeyBarrier {
	if target <= 0 {
		return nil
	}
	return &registrationPasskeyBarrier{
		release: make(chan struct{}),
		target:  target,
	}
}

func (b *registrationPasskeyBarrier) Wait() {
	if b == nil {
		return
	}

	b.mu.Lock()
	b.arrived++
	if b.arrived == b.target {
		close(b.release)
	}
	release := b.release
	b.mu.Unlock()

	<-release
}

type registrationPasskeyDB struct {
	state *registrationPasskeyState
}

type registrationPasskeyState struct {
	mu sync.Mutex

	actors           map[string]*models.Actor
	credentials      map[string]*models.WebAuthnCredential
	numericMappings  map[string]*models.NumericIDMapping
	preferences      map[string]*models.UserPreference
	proofs           map[string]*models.PasskeyRegistrationProof
	quotePermissions map[string]*models.QuotePermissions
	users            map[string]*models.User

	consumeSuccesses  int
	userCreateBarrier *registrationPasskeyBarrier
	createFailures    map[string]error
	deleteFailures    map[string]error
}

func newRegistrationPasskeyDB(concurrentUserCreates int) *registrationPasskeyDB {
	return &registrationPasskeyDB{
		state: &registrationPasskeyState{
			actors:            make(map[string]*models.Actor),
			credentials:       make(map[string]*models.WebAuthnCredential),
			numericMappings:   make(map[string]*models.NumericIDMapping),
			preferences:       make(map[string]*models.UserPreference),
			proofs:            make(map[string]*models.PasskeyRegistrationProof),
			quotePermissions:  make(map[string]*models.QuotePermissions),
			users:             make(map[string]*models.User),
			userCreateBarrier: newRegistrationPasskeyBarrier(concurrentUserCreates),
			createFailures:    make(map[string]error),
			deleteFailures:    make(map[string]error),
		},
	}
}

func (db *registrationPasskeyDB) failCreateOnce(typeName string, err error) {
	db.state.mu.Lock()
	defer db.state.mu.Unlock()
	db.state.createFailures[typeName] = err
}

func (db *registrationPasskeyDB) failDeleteOnce(typeName string, err error) {
	db.state.mu.Lock()
	defer db.state.mu.Unlock()
	db.state.deleteFailures[typeName] = err
}

func (db *registrationPasskeyDB) Model(model any) dynamormcore.Query {
	return &registrationPasskeyQuery{
		state:  db.state,
		model:  model,
		wheres: make(map[string]registrationPasskeyWhere),
		index:  "",
	}
}

func (db *registrationPasskeyDB) Migrate() error                              { return nil }
func (db *registrationPasskeyDB) AutoMigrate(...any) error                    { return nil }
func (db *registrationPasskeyDB) Close() error                                { return nil }
func (db *registrationPasskeyDB) WithContext(context.Context) dynamormcore.DB { return db }

func (db *registrationPasskeyDB) consumeSuccessCount() int {
	db.state.mu.Lock()
	defer db.state.mu.Unlock()
	return db.state.consumeSuccesses
}

type registrationPasskeyWhere struct {
	op    string
	value any
}

type registrationPasskeyQuery struct {
	state       *registrationPasskeyState
	model       any
	wheres      map[string]registrationPasskeyWhere
	index       string
	ifNotExists bool
}

func (q *registrationPasskeyQuery) Where(field, op string, value any) dynamormcore.Query {
	q.wheres[field] = registrationPasskeyWhere{op: op, value: value}
	return q
}

func (q *registrationPasskeyQuery) Index(name string) dynamormcore.Query                    { q.index = name; return q }
func (q *registrationPasskeyQuery) Filter(string, string, any) dynamormcore.Query           { return q }
func (q *registrationPasskeyQuery) OrFilter(string, string, any) dynamormcore.Query         { return q }
func (q *registrationPasskeyQuery) FilterGroup(func(dynamormcore.Query)) dynamormcore.Query { return q }
func (q *registrationPasskeyQuery) OrFilterGroup(func(dynamormcore.Query)) dynamormcore.Query {
	return q
}
func (q *registrationPasskeyQuery) IfNotExists() dynamormcore.Query                      { q.ifNotExists = true; return q }
func (q *registrationPasskeyQuery) IfExists() dynamormcore.Query                         { return q }
func (q *registrationPasskeyQuery) WithCondition(string, string, any) dynamormcore.Query { return q }
func (q *registrationPasskeyQuery) WithConditionExpression(string, map[string]any) dynamormcore.Query {
	return q
}
func (q *registrationPasskeyQuery) OrderBy(string, string) dynamormcore.Query       { return q }
func (q *registrationPasskeyQuery) Limit(int) dynamormcore.Query                    { return q }
func (q *registrationPasskeyQuery) Offset(int) dynamormcore.Query                   { return q }
func (q *registrationPasskeyQuery) Select(...string) dynamormcore.Query             { return q }
func (q *registrationPasskeyQuery) ConsistentRead() dynamormcore.Query              { return q }
func (q *registrationPasskeyQuery) WithRetry(int, time.Duration) dynamormcore.Query { return q }
func (q *registrationPasskeyQuery) Cursor(string) dynamormcore.Query                { return q }
func (q *registrationPasskeyQuery) SetCursor(string) error                          { return nil }
func (q *registrationPasskeyQuery) WithContext(context.Context) dynamormcore.Query  { return q }
func (q *registrationPasskeyQuery) Scan(any) error                                  { return fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) ParallelScan(int32, int32) dynamormcore.Query    { return q }
func (q *registrationPasskeyQuery) ScanAllSegments(any, int32) error {
	return fmt.Errorf("unsupported")
}
func (q *registrationPasskeyQuery) BatchGet([]any, any) error { return fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) BatchGetWithOptions([]any, any, *dynamormcore.BatchGetOptions) error {
	return fmt.Errorf("unsupported")
}
func (q *registrationPasskeyQuery) BatchGetBuilder() dynamormcore.BatchGetBuilder { return nil }
func (q *registrationPasskeyQuery) BatchCreate(any) error                         { return fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) BatchDelete([]any) error                       { return fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) BatchWrite([]any, []any) error                 { return fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) BatchUpdateWithOptions([]any, []string, ...any) error {
	return fmt.Errorf("unsupported")
}
func (q *registrationPasskeyQuery) Count() (int64, error) { return 0, fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) CreateOrUpdate() error { return fmt.Errorf("unsupported") }
func (q *registrationPasskeyQuery) AllPaginated(any) (*dynamormcore.PaginatedResult, error) {
	return nil, fmt.Errorf("unsupported")
}

func (q *registrationPasskeyQuery) First(dest any) error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	if handled, err := q.loadUser(dest); handled {
		return err
	}
	if handled, err := q.loadActor(dest); handled {
		return err
	}
	if handled, err := q.loadNumericMapping(dest); handled {
		return err
	}
	if handled, err := q.loadPasskeyProof(dest); handled {
		return err
	}
	if handled, err := q.loadWebAuthnCredential(dest); handled {
		return err
	}
	if handled, err := q.loadUserPreference(dest); handled {
		return err
	}
	if handled, err := q.loadQuotePermissions(dest); handled {
		return err
	}

	return fmt.Errorf("unsupported destination %T", dest)
}

func (q *registrationPasskeyQuery) All(dest any) error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	slicePtr := reflect.ValueOf(dest)
	if slicePtr.Kind() != reflect.Ptr || slicePtr.IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer")
	}
	sliceValue := slicePtr.Elem()
	if sliceValue.Kind() != reflect.Slice {
		return fmt.Errorf("destination must point to a slice")
	}

	sliceValue.Set(reflect.MakeSlice(sliceValue.Type(), 0, 0))

	switch sliceValue.Type().Elem() {
	case reflect.TypeOf(models.WebAuthnCredential{}):
		pk := q.whereString("PK")
		for _, credential := range q.state.credentials {
			if credential == nil || credential.PK != pk {
				continue
			}
			if where, ok := q.wheres["SK"]; ok && strings.EqualFold(where.op, "BEGINS_WITH") {
				prefix, _ := where.value.(string)
				if !strings.HasPrefix(credential.SK, prefix) {
					continue
				}
			}
			sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(*cloneRegistrationPasskeyCredential(credential))))
		}
		return nil
	default:
		return fmt.Errorf("unsupported destination %T", dest)
	}
}

func (q *registrationPasskeyQuery) Create() error {
	if user, ok := q.model.(*models.User); ok && user != nil {
		q.state.userCreateBarrier.Wait()
	}

	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	if err := consumeRegistrationPasskeyFailure(q.state.createFailures, q.model); err != nil {
		return err
	}

	switch model := q.model.(type) {
	case *models.User:
		if model == nil {
			return fmt.Errorf("nil user model")
		}
		if q.ifNotExists {
			if _, exists := q.state.users[model.PK]; exists {
				return dynamormerrors.ErrConditionFailed
			}
		}
		q.state.users[model.PK] = cloneRegistrationPasskeyUser(model)
		return nil
	case *models.Actor:
		if model == nil {
			return fmt.Errorf("nil actor model")
		}
		if q.ifNotExists {
			if _, exists := q.state.actors[model.PK]; exists {
				return dynamormerrors.ErrConditionFailed
			}
		}
		q.state.actors[model.PK] = cloneRegistrationPasskeyActor(model)
		return nil
	case *models.NumericIDMapping:
		if model == nil {
			return fmt.Errorf("nil numeric mapping model")
		}
		if _, exists := q.state.numericMappings[model.PK]; exists {
			return dynamormerrors.ErrConditionFailed
		}
		q.state.numericMappings[model.PK] = cloneRegistrationPasskeyNumericMapping(model)
		return nil
	case *models.PasskeyRegistrationProof:
		if model == nil {
			return fmt.Errorf("nil passkey proof model")
		}
		if q.ifNotExists {
			if _, exists := q.state.proofs[model.PK]; exists {
				return dynamormerrors.ErrConditionFailed
			}
		}
		q.state.proofs[model.PK] = cloneRegistrationPasskeyProof(model)
		return nil
	case *models.WebAuthnCredential:
		if model == nil {
			return fmt.Errorf("nil credential model")
		}
		if q.ifNotExists {
			if _, exists := q.state.credentials[model.GSI1PK]; exists {
				return dynamormerrors.ErrConditionFailed
			}
		}
		q.state.credentials[model.GSI1PK] = cloneRegistrationPasskeyCredential(model)
		return nil
	case *models.UserPreference:
		if model == nil {
			return fmt.Errorf("nil user preference model")
		}
		q.state.preferences[registrationPasskeyKey(model.PK, model.SK)] = cloneRegistrationPasskeyPreference(model)
		return nil
	case *models.QuotePermissions:
		if model == nil {
			return fmt.Errorf("nil quote permissions model")
		}
		if q.ifNotExists {
			if _, exists := q.state.quotePermissions[model.PK]; exists {
				return dynamormerrors.ErrConditionFailed
			}
		}
		q.state.quotePermissions[model.PK] = cloneRegistrationPasskeyQuotePermissions(model)
		return nil
	default:
		return fmt.Errorf("unsupported model %T", q.model)
	}
}

func (q *registrationPasskeyQuery) Update(fields ...string) error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	switch model := q.model.(type) {
	case *models.UserPreference:
		if model == nil {
			return fmt.Errorf("nil user preference model")
		}
		key := registrationPasskeyKey(model.PK, model.SK)
		if _, exists := q.state.preferences[key]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		q.state.preferences[key] = cloneRegistrationPasskeyPreference(model)
		return nil
	default:
		if len(fields) > 0 {
			return fmt.Errorf("unsupported update model %T", q.model)
		}
		return nil
	}
}

func (q *registrationPasskeyQuery) Delete() error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	if err := consumeRegistrationPasskeyFailure(q.state.deleteFailures, q.model); err != nil {
		return err
	}

	switch q.model.(type) {
	case *models.User:
		pk := q.whereString("PK")
		if _, exists := q.state.users[pk]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.users, pk)
		return nil
	case *models.Actor:
		pk := q.whereString("PK")
		if _, exists := q.state.actors[pk]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.actors, pk)
		return nil
	case *models.NumericIDMapping:
		pk := q.whereString("PK")
		if _, exists := q.state.numericMappings[pk]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.numericMappings, pk)
		return nil
	case *models.PasskeyRegistrationProof:
		pk := q.whereString("PK")
		if _, exists := q.state.proofs[pk]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.proofs, pk)
		return nil
	case *models.WebAuthnCredential:
		pk := q.whereString("PK")
		sk := q.whereString("SK")
		for key, credential := range q.state.credentials {
			if credential != nil && credential.PK == pk && credential.SK == sk {
				delete(q.state.credentials, key)
				return nil
			}
		}
		return dynamormerrors.ErrItemNotFound
	case *models.UserPreference:
		key := registrationPasskeyKey(q.whereString("PK"), q.whereString("SK"))
		if _, exists := q.state.preferences[key]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.preferences, key)
		return nil
	case *models.QuotePermissions:
		pk := q.whereString("PK")
		if _, exists := q.state.quotePermissions[pk]; !exists {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.quotePermissions, pk)
		return nil
	default:
		return fmt.Errorf("unsupported delete model %T", q.model)
	}
}

func consumeRegistrationPasskeyFailure(failures map[string]error, model any) error {
	if len(failures) == 0 || model == nil {
		return nil
	}

	typeName := reflect.TypeOf(model).String()
	err, ok := failures[typeName]
	if !ok {
		return nil
	}

	delete(failures, typeName)
	return err
}

func (q *registrationPasskeyQuery) UpdateBuilder() dynamormcore.UpdateBuilder {
	return &registrationPasskeyUpdateBuilder{
		state:      q.state,
		model:      q.model,
		proofPK:    q.whereString("PK"),
		sets:       make(map[string]any),
		conditions: make([]registrationPasskeyCondition, 0),
	}
}

func (q *registrationPasskeyQuery) whereString(field string) string {
	where, ok := q.wheres[field]
	if !ok {
		return ""
	}
	value, _ := where.value.(string)
	return value
}

func (q *registrationPasskeyQuery) loadUser(dest any) (bool, error) {
	switch target := dest.(type) {
	case *models.User:
		pk := q.whereString("PK")
		user, ok := q.state.users[pk]
		if !ok {
			return true, dynamormerrors.ErrItemNotFound
		}
		*target = *cloneRegistrationPasskeyUser(user)
		return true, nil
	default:
		typeName := reflect.TypeOf(dest).String()
		if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
			return false, nil
		}

		pk := q.whereString("PK")
		user, ok := q.state.users[pk]
		if !ok {
			return true, dynamormerrors.ErrItemNotFound
		}

		value := reflect.ValueOf(dest).Elem()
		setAccountsProjectionField(value, "Table", "test-table")
		setAccountsProjectionField(value, "PK", user.PK)
		setAccountsProjectionField(value, "SK", user.SK)
		if typeName == "*repositories.userMetadataProjection" {
			setAccountsProjectionField(value, "Metadata", cloneRegistrationPasskeyMetadata(user.Metadata))
			return true, nil
		}

		setAccountsProjectionField(value, "Username", user.Username)
		setAccountsProjectionField(value, "Email", user.Email)
		setAccountsProjectionField(value, "PasswordHash", user.PasswordHash)
		setAccountsProjectionField(value, "DisplayName", user.DisplayName)
		setAccountsProjectionField(value, "Note", user.Note)
		setAccountsProjectionField(value, "Avatar", user.Avatar)
		setAccountsProjectionField(value, "Header", user.Header)
		setAccountsProjectionField(value, "URL", user.URL)
		setAccountsProjectionField(value, "Locked", user.Locked)
		setAccountsProjectionField(value, "Discoverable", user.Discoverable)
		setAccountsProjectionField(value, "Fields", cloneRegistrationPasskeyFields(user.Fields))
		setAccountsProjectionField(value, "CreatedAt", user.CreatedAt)
		setAccountsProjectionField(value, "UpdatedAt", user.UpdatedAt)
		setAccountsProjectionField(value, "Approved", user.Approved)
		setAccountsProjectionField(value, "Suspended", user.Suspended)
		setAccountsProjectionField(value, "Silenced", user.Silenced)
		setAccountsProjectionField(value, "Role", user.Role)
		setAccountsProjectionField(value, "Locale", user.Locale)
		setAccountsProjectionField(value, "RecoveryMethods", append([]string(nil), user.RecoveryMethods...))
		setAccountsProjectionField(value, "AllowNSFW", user.AllowNSFW)
		setAccountsProjectionField(value, "RequireNSFWWarning", user.RequireNSFWWarning)
		setAccountsProjectionField(value, "Metadata", cloneRegistrationPasskeyMetadata(user.Metadata))
		setAccountsProjectionField(value, "IsAgent", user.IsAgent)
		setAccountsProjectionField(value, "AgentType", user.AgentType)
		setAccountsProjectionField(value, "AgentCapabilities", user.AgentCapabilities)
		setAccountsProjectionField(value, "AgentVersion", user.AgentVersion)
		setAccountsProjectionField(value, "AgentOwner", user.AgentOwner)
		setAccountsProjectionField(value, "AgentCreatedBy", user.AgentCreatedBy)
		setAccountsProjectionField(value, "AgentPublicKey", user.AgentPublicKey)
		setAccountsProjectionField(value, "AgentKeyType", user.AgentKeyType)
		setAccountsProjectionField(value, "Version", user.Version)
		return true, nil
	}
}

func (q *registrationPasskeyQuery) loadActor(dest any) (bool, error) {
	target, ok := dest.(*models.Actor)
	if !ok {
		return false, nil
	}

	pk := q.whereString("PK")
	actor, exists := q.state.actors[pk]
	if !exists {
		return true, dynamormerrors.ErrItemNotFound
	}
	*target = *cloneRegistrationPasskeyActor(actor)
	return true, nil
}

func (q *registrationPasskeyQuery) loadNumericMapping(dest any) (bool, error) {
	target, ok := dest.(*models.NumericIDMapping)
	if !ok {
		return false, nil
	}

	pk := q.whereString("PK")
	mapping, exists := q.state.numericMappings[pk]
	if !exists {
		return true, dynamormerrors.ErrItemNotFound
	}
	*target = *cloneRegistrationPasskeyNumericMapping(mapping)
	return true, nil
}

func (q *registrationPasskeyQuery) loadPasskeyProof(dest any) (bool, error) {
	target, ok := dest.(*models.PasskeyRegistrationProof)
	if !ok {
		return false, nil
	}

	pk := q.whereString("PK")
	proof, exists := q.state.proofs[pk]
	if !exists {
		return true, dynamormerrors.ErrItemNotFound
	}
	*target = *cloneRegistrationPasskeyProof(proof)
	return true, nil
}

func (q *registrationPasskeyQuery) loadWebAuthnCredential(dest any) (bool, error) {
	target, ok := dest.(*models.WebAuthnCredential)
	if !ok {
		return false, nil
	}

	var credential *models.WebAuthnCredential
	if q.index == "gsi1" {
		credential = q.state.credentials[q.whereString("gsi1PK")]
	} else {
		pk := q.whereString("PK")
		sk := q.whereString("SK")
		for _, candidate := range q.state.credentials {
			if candidate != nil && candidate.PK == pk && candidate.SK == sk {
				credential = candidate
				break
			}
		}
	}
	if credential == nil {
		return true, dynamormerrors.ErrItemNotFound
	}
	*target = *cloneRegistrationPasskeyCredential(credential)
	return true, nil
}

func (q *registrationPasskeyQuery) loadUserPreference(dest any) (bool, error) {
	target, ok := dest.(*models.UserPreference)
	if !ok {
		return false, nil
	}

	pref, exists := q.state.preferences[registrationPasskeyKey(q.whereString("PK"), q.whereString("SK"))]
	if !exists {
		return true, dynamormerrors.ErrItemNotFound
	}
	*target = *cloneRegistrationPasskeyPreference(pref)
	return true, nil
}

func (q *registrationPasskeyQuery) loadQuotePermissions(dest any) (bool, error) {
	target, ok := dest.(*models.QuotePermissions)
	if !ok {
		return false, nil
	}

	permissions, exists := q.state.quotePermissions[q.whereString("PK")]
	if !exists {
		return true, dynamormerrors.ErrItemNotFound
	}
	*target = *cloneRegistrationPasskeyQuotePermissions(permissions)
	return true, nil
}

type registrationPasskeyCondition struct {
	field string
	op    string
	value any
}

type registrationPasskeyUpdateBuilder struct {
	state      *registrationPasskeyState
	model      any
	proofPK    string
	sets       map[string]any
	conditions []registrationPasskeyCondition
}

func (b *registrationPasskeyUpdateBuilder) Set(field string, value any) dynamormcore.UpdateBuilder {
	b.sets[field] = value
	return b
}

func (b *registrationPasskeyUpdateBuilder) SetIfNotExists(string, any, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) Add(string, any) dynamormcore.UpdateBuilder    { return b }
func (b *registrationPasskeyUpdateBuilder) Increment(string) dynamormcore.UpdateBuilder   { return b }
func (b *registrationPasskeyUpdateBuilder) Decrement(string) dynamormcore.UpdateBuilder   { return b }
func (b *registrationPasskeyUpdateBuilder) Remove(string) dynamormcore.UpdateBuilder      { return b }
func (b *registrationPasskeyUpdateBuilder) Delete(string, any) dynamormcore.UpdateBuilder { return b }
func (b *registrationPasskeyUpdateBuilder) AppendToList(string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) PrependToList(string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) RemoveFromListAt(string, int) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) SetListElement(string, int, any) dynamormcore.UpdateBuilder {
	return b
}

func (b *registrationPasskeyUpdateBuilder) Condition(field string, op string, value any) dynamormcore.UpdateBuilder {
	b.conditions = append(b.conditions, registrationPasskeyCondition{field: field, op: op, value: value})
	return b
}

func (b *registrationPasskeyUpdateBuilder) OrCondition(string, string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) ConditionExists(string) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) ConditionNotExists(string) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) ConditionVersion(int64) dynamormcore.UpdateBuilder {
	return b
}
func (b *registrationPasskeyUpdateBuilder) ReturnValues(string) dynamormcore.UpdateBuilder { return b }

func (b *registrationPasskeyUpdateBuilder) Execute() error {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	proof, exists := b.state.proofs[b.proofPK]
	if !exists {
		return dynamormerrors.ErrItemNotFound
	}

	for _, condition := range b.conditions {
		if !registrationPasskeyConditionMet(proof, condition) {
			return dynamormerrors.ErrConditionFailed
		}
	}

	for field, value := range b.sets {
		switch field {
		case "Consumed":
			proof.Consumed = value.(bool)
		case "ConsumedAt":
			proof.ConsumedAt = value.(time.Time)
		}
	}
	if proof.Consumed {
		b.state.consumeSuccesses++
	}
	b.state.proofs[b.proofPK] = cloneRegistrationPasskeyProof(proof)
	return nil
}

func (b *registrationPasskeyUpdateBuilder) ExecuteWithResult(any) error { return b.Execute() }

func registrationPasskeyConditionMet(proof *models.PasskeyRegistrationProof, condition registrationPasskeyCondition) bool {
	switch condition.field {
	case "Consumed":
		expected, ok := condition.value.(bool)
		return ok && condition.op == "=" && proof.Consumed == expected
	case "TTL":
		expected, ok := condition.value.(int64)
		if !ok {
			return false
		}
		switch condition.op {
		case ">":
			return proof.TTL > expected
		case "=":
			return proof.TTL == expected
		default:
			return false
		}
	case "Username":
		expected, ok := condition.value.(string)
		return ok && condition.op == "=" && proof.Username == expected
	case "CeremonyID":
		expected, ok := condition.value.(string)
		return ok && condition.op == "=" && proof.CeremonyID == expected
	default:
		return false
	}
}

func registrationPasskeyKey(pk string, sk string) string {
	return pk + "|" + sk
}

func cloneRegistrationPasskeyActor(src *models.Actor) *models.Actor {
	if src == nil {
		return nil
	}
	clone := *src
	if src.Actor != nil {
		actorClone := *src.Actor
		if src.Actor.PublicKey != nil {
			publicKeyClone := *src.Actor.PublicKey
			actorClone.PublicKey = &publicKeyClone
		}
		clone.Actor = &actorClone
	}
	if src.LastStatusAt != nil {
		lastStatusAt := *src.LastStatusAt
		clone.LastStatusAt = &lastStatusAt
	}
	clone.Fields = append([]models.ActorField(nil), src.Fields...)
	return &clone
}

func cloneRegistrationPasskeyCredential(src *models.WebAuthnCredential) *models.WebAuthnCredential {
	if src == nil {
		return nil
	}
	clone := *src
	clone.PublicKey = append([]byte(nil), src.PublicKey...)
	clone.AAGUID = append([]byte(nil), src.AAGUID...)
	return &clone
}

func cloneRegistrationPasskeyFields(src []map[string]string) []map[string]string {
	if src == nil {
		return nil
	}
	clone := make([]map[string]string, 0, len(src))
	for _, field := range src {
		clonedField := make(map[string]string, len(field))
		for key, value := range field {
			clonedField[key] = value
		}
		clone = append(clone, clonedField)
	}
	return clone
}

func cloneRegistrationPasskeyMetadata(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(src))
	for key, value := range src {
		clone[key] = value
	}
	return clone
}

func cloneRegistrationPasskeyNumericMapping(src *models.NumericIDMapping) *models.NumericIDMapping {
	if src == nil {
		return nil
	}
	clone := *src
	return &clone
}

func cloneRegistrationPasskeyPreference(src *models.UserPreference) *models.UserPreference {
	if src == nil {
		return nil
	}
	clone := *src
	return &clone
}

func cloneRegistrationPasskeyQuotePermissions(src *models.QuotePermissions) *models.QuotePermissions {
	if src == nil {
		return nil
	}
	clone := *src
	clone.BlockList = append([]string(nil), src.BlockList...)
	return &clone
}

func cloneRegistrationPasskeyUser(src *models.User) *models.User {
	if src == nil {
		return nil
	}
	clone := *src
	clone.Fields = cloneRegistrationPasskeyFields(src.Fields)
	clone.RecoveryMethods = append([]string(nil), src.RecoveryMethods...)
	clone.Metadata = cloneRegistrationPasskeyMetadata(src.Metadata)
	return &clone
}

func cloneRegistrationPasskeyProof(src *models.PasskeyRegistrationProof) *models.PasskeyRegistrationProof {
	if src == nil {
		return nil
	}

	clone := *src
	clone.PublicKey = append([]byte(nil), src.PublicKey...)
	clone.AAGUID = append([]byte(nil), src.AAGUID...)
	return &clone
}
