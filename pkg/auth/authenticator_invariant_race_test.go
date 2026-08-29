package auth

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
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

func TestAuthenticatorInvariant_ConcurrentCrossKindRemovalPreservesSurvivor(t *testing.T) {
	t.Parallel()

	t.Run("sequential control updates live inventory", func(t *testing.T) {
		repo := newAuthenticatorInvariantAccountRepo(t, newAuthenticatorInvariantDB(), 0)
		seedAuthenticatorInventory(t, repo, "alice", "cred-1", "0xabc")

		webAuthnSvc := &WebAuthnService{repo: repo}
		walletSvc := &WalletService{repo: repo, logger: zap.NewNop()}

		require.NoError(t, webAuthnSvc.DeleteCredential(context.Background(), "alice", "cred-1"))
		err := walletSvc.UnlinkWallet(context.Background(), "alice", "0xabc")
		require.ErrorIs(t, err, ErrLastAuthMethodDelete)

		passkeys, wallets := authenticatorCounts(t, repo, "alice")
		require.Equal(t, 0, passkeys)
		require.Equal(t, 1, wallets)
	})

	t.Run("concurrent passkey delete and wallet unlink leave exactly one authenticator", func(t *testing.T) {
		repo := newAuthenticatorInvariantAccountRepo(t, newAuthenticatorInvariantDB(), 2)
		seedAuthenticatorInventory(t, repo, "alice", "cred-1", "0xabc")

		webAuthnSvc := &WebAuthnService{repo: repo}
		walletSvc := &WalletService{repo: repo, logger: zap.NewNop()}

		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			errs[0] = webAuthnSvc.DeleteCredential(context.Background(), "alice", "cred-1")
		}()
		go func() {
			defer wg.Done()
			errs[1] = walletSvc.UnlinkWallet(context.Background(), "alice", "0xabc")
		}()
		wg.Wait()

		successes := 0
		var loser error
		for _, err := range errs {
			if err == nil {
				successes++
				continue
			}
			require.Nil(t, loser, "expected exactly one clean loser")
			loser = err
		}

		require.Equal(t, 1, successes, "expected exactly one removal to commit")
		require.ErrorIs(t, loser, ErrLastAuthMethodDelete)

		passkeys, wallets := authenticatorCounts(t, repo, "alice")
		require.Equal(t, 1, passkeys+wallets, "account must retain exactly one authenticator")
	})
}

type authenticatorInvariantAccountRepo struct {
	*repositories.AccountRepository
	deleteBarrier *authenticatorInvariantBarrier
}

type conditionedWebAuthnDeleter interface {
	DeleteWebAuthnCredentialConditionedOnSurvivor(
		ctx context.Context,
		username string,
		credentialID string,
		survivingPasskeyID string,
		survivingWalletAddress string,
	) error
}

type conditionedWalletDeleter interface {
	DeleteWalletCredentialConditionedOnSurvivor(
		ctx context.Context,
		username, address, walletType string,
		survivingPasskeyID string,
		survivingWalletAddress string,
	) error
}

func newAuthenticatorInvariantAccountRepo(t *testing.T, db dynamormcore.DB, concurrentDeletes int) *authenticatorInvariantAccountRepo {
	t.Helper()

	repo := repositories.NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)
	return &authenticatorInvariantAccountRepo{
		AccountRepository: repo,
		deleteBarrier:     newAuthenticatorInvariantBarrier(concurrentDeletes),
	}
}

func (r *authenticatorInvariantAccountRepo) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	r.deleteBarrier.Wait()
	return r.AccountRepository.DeleteWebAuthnCredential(ctx, credentialID)
}

func (r *authenticatorInvariantAccountRepo) DeleteWalletCredential(ctx context.Context, username, address string) error {
	r.deleteBarrier.Wait()
	return r.AccountRepository.DeleteWalletCredential(ctx, username, address)
}

func (r *authenticatorInvariantAccountRepo) DeleteWebAuthnCredentialConditionedOnSurvivor(
	ctx context.Context,
	username string,
	credentialID string,
	survivingPasskeyID string,
	survivingWalletAddress string,
) error {
	r.deleteBarrier.Wait()
	if conditioned, ok := any(r.AccountRepository).(conditionedWebAuthnDeleter); ok {
		return conditioned.DeleteWebAuthnCredentialConditionedOnSurvivor(
			ctx,
			username,
			credentialID,
			survivingPasskeyID,
			survivingWalletAddress,
		)
	}
	return r.AccountRepository.DeleteWebAuthnCredential(ctx, credentialID)
}

func (r *authenticatorInvariantAccountRepo) DeleteWalletCredentialConditionedOnSurvivor(
	ctx context.Context,
	username, address, walletType string,
	survivingPasskeyID string,
	survivingWalletAddress string,
) error {
	r.deleteBarrier.Wait()
	if conditioned, ok := any(r.AccountRepository).(conditionedWalletDeleter); ok {
		return conditioned.DeleteWalletCredentialConditionedOnSurvivor(
			ctx,
			username,
			address,
			walletType,
			survivingPasskeyID,
			survivingWalletAddress,
		)
	}
	return r.AccountRepository.DeleteWalletCredential(ctx, username, address)
}

func seedAuthenticatorInventory(t *testing.T, repo *authenticatorInvariantAccountRepo, username, credentialID, walletAddress string) {
	t.Helper()

	require.NoError(t, repo.CreateWebAuthnCredential(context.Background(), &storage.WebAuthnCredential{
		ID:         credentialID,
		UserID:     username,
		PublicKey:  []byte("pk"),
		CreatedAt:  time.Now().Add(-time.Minute),
		LastUsedAt: time.Now().Add(-time.Minute),
	}))
	require.NoError(t, repo.StoreWalletCredential(context.Background(), &storage.WalletCredential{
		Username: username,
		Address:  walletAddress,
		ChainID:  1,
		Type:     "ethereum",
		LinkedAt: time.Now().Add(-time.Minute),
		LastUsed: time.Now().Add(-time.Minute),
	}))
}

func authenticatorCounts(t *testing.T, repo *authenticatorInvariantAccountRepo, username string) (int, int) {
	t.Helper()

	passkeys, err := repo.GetUserWebAuthnCredentials(context.Background(), username)
	require.NoError(t, err)
	wallets, err := repo.GetUserWalletCredentials(context.Background(), username)
	require.NoError(t, err)
	return len(passkeys), len(wallets)
}

type authenticatorInvariantBarrier struct {
	mu      sync.Mutex
	release chan struct{}
	target  int
	arrived int
}

func newAuthenticatorInvariantBarrier(target int) *authenticatorInvariantBarrier {
	if target <= 0 {
		return nil
	}
	return &authenticatorInvariantBarrier{
		release: make(chan struct{}),
		target:  target,
	}
}

func (b *authenticatorInvariantBarrier) Wait() {
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

type authenticatorInvariantDB struct {
	state *authenticatorInvariantState
}

type authenticatorInvariantState struct {
	mu sync.Mutex

	credentials   map[string]*models.WebAuthnCredential
	wallets       map[string]*models.WalletCredential
	walletIndexes map[string]*models.WalletIndex
}

func newAuthenticatorInvariantDB() *authenticatorInvariantDB {
	return &authenticatorInvariantDB{
		state: &authenticatorInvariantState{
			credentials:   make(map[string]*models.WebAuthnCredential),
			wallets:       make(map[string]*models.WalletCredential),
			walletIndexes: make(map[string]*models.WalletIndex),
		},
	}
}

func (db *authenticatorInvariantDB) Model(model any) dynamormcore.Query {
	return &authenticatorInvariantQuery{
		state:  db.state,
		model:  model,
		wheres: make(map[string]authenticatorInvariantWhere),
	}
}

func (db *authenticatorInvariantDB) Migrate() error                              { return nil }
func (db *authenticatorInvariantDB) AutoMigrate(...any) error                    { return nil }
func (db *authenticatorInvariantDB) Close() error                                { return nil }
func (db *authenticatorInvariantDB) WithContext(context.Context) dynamormcore.DB { return db }
func (db *authenticatorInvariantDB) Transact() dynamormcore.TransactionBuilder {
	return &authenticatorInvariantTransactionBuilder{state: db.state}
}
func (db *authenticatorInvariantDB) TransactWrite(_ context.Context, fn func(dynamormcore.TransactionBuilder) error) error {
	builder := &authenticatorInvariantTransactionBuilder{state: db.state}
	if err := fn(builder); err != nil {
		return err
	}
	return builder.Execute()
}

type authenticatorInvariantWhere struct {
	op    string
	value any
}

type authenticatorInvariantQuery struct {
	state  *authenticatorInvariantState
	model  any
	wheres map[string]authenticatorInvariantWhere
	index  string
}

func (q *authenticatorInvariantQuery) Where(field, op string, value any) dynamormcore.Query {
	q.wheres[field] = authenticatorInvariantWhere{op: op, value: value}
	return q
}

func (q *authenticatorInvariantQuery) Index(name string) dynamormcore.Query            { q.index = name; return q }
func (q *authenticatorInvariantQuery) Filter(string, string, any) dynamormcore.Query   { return q }
func (q *authenticatorInvariantQuery) OrFilter(string, string, any) dynamormcore.Query { return q }
func (q *authenticatorInvariantQuery) FilterGroup(func(dynamormcore.Query)) dynamormcore.Query {
	return q
}
func (q *authenticatorInvariantQuery) OrFilterGroup(func(dynamormcore.Query)) dynamormcore.Query {
	return q
}
func (q *authenticatorInvariantQuery) IfNotExists() dynamormcore.Query                      { return q }
func (q *authenticatorInvariantQuery) IfExists() dynamormcore.Query                         { return q }
func (q *authenticatorInvariantQuery) WithCondition(string, string, any) dynamormcore.Query { return q }
func (q *authenticatorInvariantQuery) WithConditionExpression(string, map[string]any) dynamormcore.Query {
	return q
}
func (q *authenticatorInvariantQuery) OrderBy(string, string) dynamormcore.Query       { return q }
func (q *authenticatorInvariantQuery) Limit(int) dynamormcore.Query                    { return q }
func (q *authenticatorInvariantQuery) Offset(int) dynamormcore.Query                   { return q }
func (q *authenticatorInvariantQuery) Select(...string) dynamormcore.Query             { return q }
func (q *authenticatorInvariantQuery) ConsistentRead() dynamormcore.Query              { return q }
func (q *authenticatorInvariantQuery) WithRetry(int, time.Duration) dynamormcore.Query { return q }
func (q *authenticatorInvariantQuery) Cursor(string) dynamormcore.Query                { return q }
func (q *authenticatorInvariantQuery) SetCursor(string) error                          { return nil }
func (q *authenticatorInvariantQuery) WithContext(context.Context) dynamormcore.Query  { return q }
func (q *authenticatorInvariantQuery) Scan(any) error                                  { return fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) ParallelScan(int32, int32) dynamormcore.Query    { return q }
func (q *authenticatorInvariantQuery) ScanAllSegments(any, int32) error {
	return fmt.Errorf("unsupported")
}
func (q *authenticatorInvariantQuery) BatchGet([]any, any) error { return fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) BatchGetWithOptions([]any, any, *dynamormcore.BatchGetOptions) error {
	return fmt.Errorf("unsupported")
}
func (q *authenticatorInvariantQuery) BatchGetBuilder() dynamormcore.BatchGetBuilder { return nil }
func (q *authenticatorInvariantQuery) BatchCreate(any) error                         { return fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) BatchDelete([]any) error                       { return fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) BatchWrite([]any, []any) error {
	return fmt.Errorf("unsupported")
}
func (q *authenticatorInvariantQuery) BatchUpdateWithOptions([]any, []string, ...any) error {
	return fmt.Errorf("unsupported")
}
func (q *authenticatorInvariantQuery) Count() (int64, error) { return 0, fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) CreateOrUpdate() error { return fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) AllPaginated(dest any) (*dynamormcore.PaginatedResult, error) {
	if err := q.All(dest); err != nil {
		return nil, err
	}
	return &dynamormcore.PaginatedResult{HasMore: false}, nil
}

func (q *authenticatorInvariantQuery) First(dest any) error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	switch target := dest.(type) {
	case *models.WebAuthnCredential:
		if q.index == "gsi1" {
			credential := q.state.credentials[q.whereString("gsi1PK")]
			if credential == nil {
				return dynamormerrors.ErrItemNotFound
			}
			*target = *cloneAuthenticatorInvariantCredential(credential)
			return nil
		}
		pk := q.whereString("PK")
		sk := q.whereString("SK")
		for _, credential := range q.state.credentials {
			if credential != nil && credential.PK == pk && credential.SK == sk {
				*target = *cloneAuthenticatorInvariantCredential(credential)
				return nil
			}
		}
		return dynamormerrors.ErrItemNotFound
	case *models.WalletCredential:
		pk := q.whereString("PK")
		sk := q.whereString("SK")
		for _, wallet := range q.state.wallets {
			if wallet != nil && wallet.PK == pk && wallet.SK == sk {
				*target = *cloneAuthenticatorInvariantWallet(wallet)
				return nil
			}
		}
		return dynamormerrors.ErrItemNotFound
	default:
		return fmt.Errorf("unsupported destination %T", dest)
	}
}

func (q *authenticatorInvariantQuery) All(dest any) error {
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
		prefix := q.whereString("SK")
		for _, credential := range q.state.credentials {
			if credential == nil || credential.PK != pk {
				continue
			}
			if where, ok := q.wheres["SK"]; ok && strings.EqualFold(where.op, "BEGINS_WITH") && !strings.HasPrefix(credential.SK, prefix) {
				continue
			}
			sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(*cloneAuthenticatorInvariantCredential(credential))))
		}
		return nil
	case reflect.TypeOf(models.WalletCredential{}):
		pk := q.whereString("PK")
		prefix := q.whereString("SK")
		for _, wallet := range q.state.wallets {
			if wallet == nil || wallet.PK != pk {
				continue
			}
			if where, ok := q.wheres["SK"]; ok && strings.EqualFold(where.op, "BEGINS_WITH") && !strings.HasPrefix(wallet.SK, prefix) {
				continue
			}
			sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(*cloneAuthenticatorInvariantWallet(wallet))))
		}
		return nil
	case reflect.TypeOf(models.WalletIndex{}):
		pk := q.whereString("PK")
		for _, index := range q.state.walletIndexes {
			if index == nil || index.PK != pk {
				continue
			}
			sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(*cloneAuthenticatorInvariantWalletIndex(index))))
		}
		return nil
	default:
		return fmt.Errorf("unsupported destination %T", dest)
	}
}

func (q *authenticatorInvariantQuery) Create() error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	switch model := q.model.(type) {
	case *models.WebAuthnCredential:
		if model == nil {
			return fmt.Errorf("nil WebAuthn credential")
		}
		q.state.credentials[model.GSI1PK] = cloneAuthenticatorInvariantCredential(model)
		return nil
	case *models.WalletCredential:
		if model == nil {
			return fmt.Errorf("nil wallet credential")
		}
		q.state.wallets[authenticatorInvariantWalletKey(model.Username, model.Address)] = cloneAuthenticatorInvariantWallet(model)
		q.state.walletIndexes[authenticatorInvariantWalletIndexKey(
			fmt.Sprintf("WALLET#%s#%s", model.Type, strings.ToLower(model.Address)),
			fmt.Sprintf("USER#%s", model.Username),
		)] = &models.WalletIndex{
			PK:         fmt.Sprintf("WALLET#%s#%s", model.Type, strings.ToLower(model.Address)),
			SK:         fmt.Sprintf("USER#%s", model.Username),
			Username:   model.Username,
			Address:    strings.ToLower(model.Address),
			WalletType: model.Type,
		}
		return nil
	case *models.WalletIndex:
		if model == nil {
			return fmt.Errorf("nil wallet index")
		}
		q.state.walletIndexes[authenticatorInvariantWalletIndexKey(model.PK, model.SK)] = cloneAuthenticatorInvariantWalletIndex(model)
		return nil
	default:
		return fmt.Errorf("unsupported model %T", q.model)
	}
}

func (q *authenticatorInvariantQuery) Update(...string) error                    { return fmt.Errorf("unsupported") }
func (q *authenticatorInvariantQuery) UpdateBuilder() dynamormcore.UpdateBuilder { return nil }

func (q *authenticatorInvariantQuery) Delete() error {
	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	switch q.model.(type) {
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
	case *models.WalletCredential:
		pk := q.whereString("PK")
		sk := q.whereString("SK")
		for key, wallet := range q.state.wallets {
			if wallet != nil && wallet.PK == pk && wallet.SK == sk {
				delete(q.state.wallets, key)
				return nil
			}
		}
		return dynamormerrors.ErrItemNotFound
	case *models.WalletIndex:
		key := authenticatorInvariantWalletIndexKey(q.whereString("PK"), q.whereString("SK"))
		if _, ok := q.state.walletIndexes[key]; !ok {
			return dynamormerrors.ErrItemNotFound
		}
		delete(q.state.walletIndexes, key)
		return nil
	default:
		return fmt.Errorf("unsupported delete model %T", q.model)
	}
}

func (q *authenticatorInvariantQuery) whereString(field string) string {
	where, ok := q.wheres[field]
	if !ok {
		return ""
	}
	value, _ := where.value.(string)
	return value
}

type authenticatorInvariantTransactionBuilder struct {
	state      *authenticatorInvariantState
	operations []authenticatorInvariantTransactionOperation
}

type authenticatorInvariantTransactionOperation struct {
	kind       string
	model      any
	conditions []dynamormcore.TransactCondition
}

func (b *authenticatorInvariantTransactionBuilder) Put(any, ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	return b
}
func (b *authenticatorInvariantTransactionBuilder) Create(any, ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	return b
}
func (b *authenticatorInvariantTransactionBuilder) Update(any, []string, ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	return b
}
func (b *authenticatorInvariantTransactionBuilder) UpdateWithBuilder(any, func(dynamormcore.UpdateBuilder) error, ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	return b
}
func (b *authenticatorInvariantTransactionBuilder) Delete(model any, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	b.operations = append(b.operations, authenticatorInvariantTransactionOperation{kind: "delete", model: model, conditions: append([]dynamormcore.TransactCondition(nil), conditions...)})
	return b
}
func (b *authenticatorInvariantTransactionBuilder) ConditionCheck(model any, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	b.operations = append(b.operations, authenticatorInvariantTransactionOperation{kind: "condition_check", model: model, conditions: append([]dynamormcore.TransactCondition(nil), conditions...)})
	return b
}
func (b *authenticatorInvariantTransactionBuilder) WithContext(context.Context) dynamormcore.TransactionBuilder {
	return b
}

func (b *authenticatorInvariantTransactionBuilder) Execute() error {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	for idx, operation := range b.operations {
		if !authenticatorInvariantConditionsMet(b.state, operation.model, operation.conditions) {
			return &dynamormerrors.TransactionError{
				Err:            dynamormerrors.ErrConditionFailed,
				Operation:      operation.kind,
				OperationIndex: idx,
				Reason:         "ConditionalCheckFailed",
			}
		}
	}

	for idx, operation := range b.operations {
		if operation.kind != "delete" {
			continue
		}
		if !authenticatorInvariantDeleteModel(b.state, operation.model) {
			return &dynamormerrors.TransactionError{
				Err:            dynamormerrors.ErrConditionFailed,
				Operation:      operation.kind,
				OperationIndex: idx,
				Reason:         "ConditionalCheckFailed",
			}
		}
	}

	return nil
}

func (b *authenticatorInvariantTransactionBuilder) ExecuteWithContext(context.Context) error {
	return b.Execute()
}

func authenticatorInvariantConditionsMet(state *authenticatorInvariantState, model any, conditions []dynamormcore.TransactCondition) bool {
	for _, condition := range conditions {
		switch condition.Kind {
		case dynamormcore.TransactConditionKindPrimaryKeyExists:
			if !authenticatorInvariantModelExists(state, model) {
				return false
			}
		case dynamormcore.TransactConditionKindPrimaryKeyNotExists:
			if authenticatorInvariantModelExists(state, model) {
				return false
			}
		}
	}
	return true
}

func authenticatorInvariantModelExists(state *authenticatorInvariantState, model any) bool {
	switch typed := model.(type) {
	case *models.WebAuthnCredential:
		for _, credential := range state.credentials {
			if credential != nil && credential.PK == typed.PK && credential.SK == typed.SK {
				return true
			}
		}
	case *models.WalletCredential:
		for _, wallet := range state.wallets {
			if wallet != nil && wallet.PK == typed.PK && wallet.SK == typed.SK {
				return true
			}
		}
	case *models.WalletIndex:
		_, ok := state.walletIndexes[authenticatorInvariantWalletIndexKey(typed.PK, typed.SK)]
		return ok
	}
	return false
}

func authenticatorInvariantDeleteModel(state *authenticatorInvariantState, model any) bool {
	switch typed := model.(type) {
	case *models.WebAuthnCredential:
		for key, credential := range state.credentials {
			if credential != nil && credential.PK == typed.PK && credential.SK == typed.SK {
				delete(state.credentials, key)
				return true
			}
		}
	case *models.WalletCredential:
		for key, wallet := range state.wallets {
			if wallet != nil && wallet.PK == typed.PK && wallet.SK == typed.SK {
				delete(state.wallets, key)
				return true
			}
		}
	case *models.WalletIndex:
		key := authenticatorInvariantWalletIndexKey(typed.PK, typed.SK)
		if _, ok := state.walletIndexes[key]; ok {
			delete(state.walletIndexes, key)
			return true
		}
	}
	return false
}

func cloneAuthenticatorInvariantCredential(src *models.WebAuthnCredential) *models.WebAuthnCredential {
	if src == nil {
		return nil
	}
	clone := *src
	clone.PublicKey = append([]byte(nil), src.PublicKey...)
	clone.AAGUID = append([]byte(nil), src.AAGUID...)
	return &clone
}

func cloneAuthenticatorInvariantWallet(src *models.WalletCredential) *models.WalletCredential {
	if src == nil {
		return nil
	}
	clone := *src
	return &clone
}

func cloneAuthenticatorInvariantWalletIndex(src *models.WalletIndex) *models.WalletIndex {
	if src == nil {
		return nil
	}
	clone := *src
	return &clone
}

func authenticatorInvariantWalletKey(username, address string) string {
	return username + "|" + strings.ToLower(address)
}

func authenticatorInvariantWalletIndexKey(pk, sk string) string {
	return pk + "|" + sk
}
