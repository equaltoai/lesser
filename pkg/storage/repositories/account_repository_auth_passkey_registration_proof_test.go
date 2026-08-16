package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"sync"
	"testing"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

func TestAccountRepository_PasskeyRegistrationProofLifecycle(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	ctx := context.Background()
	proof := &models.PasskeyRegistrationProof{
		ID:              "proof-1",
		Username:        "alice",
		CeremonyID:      "ceremony-1",
		CredentialID:    "cred-1",
		PublicKey:       []byte("pk"),
		AttestationType: "none",
		AAGUID:          []byte{0x01},
		ExpiresAt:       time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(ctx, proof))

	got, err := repo.GetPasskeyRegistrationProof(ctx, proof.ID)
	require.NoError(t, err)
	require.Equal(t, proof.ID, got.ID)
	require.Equal(t, proof.Username, got.Username)
	require.Equal(t, proof.CeremonyID, got.CeremonyID)
	require.Equal(t, proof.CredentialID, got.CredentialID)
	require.False(t, got.Consumed)

	consumed, err := repo.ConsumePasskeyRegistrationProof(ctx, proof.ID, proof.Username, proof.CeremonyID)
	require.NoError(t, err)
	require.True(t, consumed.Consumed)
	require.False(t, consumed.ConsumedAt.IsZero())

	got, err = repo.GetPasskeyRegistrationProof(ctx, proof.ID)
	require.NoError(t, err)
	require.True(t, got.Consumed)
	require.False(t, got.ConsumedAt.IsZero())
}

func TestAccountRepository_ConsumePasskeyRegistrationProof_ExactlyOnce(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	ctx := context.Background()
	proof := &models.PasskeyRegistrationProof{
		ID:           "proof-2",
		Username:     "alice",
		CeremonyID:   "ceremony-2",
		CredentialID: "cred-2",
		PublicKey:    []byte("pk"),
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(ctx, proof))

	results := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.ConsumePasskeyRegistrationProof(ctx, proof.ID, proof.Username, proof.CeremonyID)
			results <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		failures++
	}

	require.Equal(t, 1, successes)
	require.Equal(t, 1, failures)

	stored, err := repo.GetPasskeyRegistrationProof(ctx, proof.ID)
	require.NoError(t, err)
	require.True(t, stored.Consumed)
	require.False(t, stored.ConsumedAt.IsZero())
}

func TestAccountRepository_StorePasskeyRegistrationProof_RejectsNil(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	err := repo.StorePasskeyRegistrationProof(context.Background(), nil)
	require.Error(t, err)
}

func TestAccountRepository_GetPasskeyRegistrationProof_ExpiredDeletesAndReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	now := time.Now().UTC()
	proof := &models.PasskeyRegistrationProof{
		ID:           "expired-proof",
		Username:     "alice",
		CeremonyID:   "ceremony-expired",
		CredentialID: "cred-expired",
		PublicKey:    []byte("pk"),
		CreatedAt:    now.Add(-15 * time.Minute),
		ExpiresAt:    now.Add(-5 * time.Minute),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(context.Background(), proof))

	got, err := repo.GetPasskeyRegistrationProof(context.Background(), proof.ID)
	require.Nil(t, got)
	require.Error(t, err)

	db.state.mu.Lock()
	defer db.state.mu.Unlock()
	_, exists := db.state.proofs[proof.ID]
	require.False(t, exists)
}

func TestAccountRepository_DeletePasskeyRegistrationProof_IsIdempotent(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	proof := &models.PasskeyRegistrationProof{
		ID:           "proof-delete",
		Username:     "alice",
		CeremonyID:   "ceremony-delete",
		CredentialID: "cred-delete",
		PublicKey:    []byte("pk"),
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(context.Background(), proof))
	require.NoError(t, repo.DeletePasskeyRegistrationProof(context.Background(), proof.ID))
	require.NoError(t, repo.DeletePasskeyRegistrationProof(context.Background(), proof.ID))
}

func TestAccountRepository_GetPasskeyRegistrationProof_MissingAndInvalidID(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	got, err := repo.GetPasskeyRegistrationProof(context.Background(), "")
	require.Nil(t, got)
	require.Error(t, err)

	got, err = repo.GetPasskeyRegistrationProof(context.Background(), "missing-proof")
	require.Nil(t, got)
	require.Error(t, err)
}

func TestAccountRepository_StorePasskeyRegistrationProof_RejectsDuplicateID(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	ctx := context.Background()

	proof := &models.PasskeyRegistrationProof{
		ID:           "proof-duplicate",
		Username:     "alice",
		CeremonyID:   "ceremony-duplicate",
		CredentialID: "cred-duplicate",
		PublicKey:    []byte("pk"),
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(ctx, proof))
	require.Error(t, repo.StorePasskeyRegistrationProof(ctx, proof))
}

func TestAccountRepository_ConsumePasskeyRegistrationProof_RejectsMismatchedBinding(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	proof := &models.PasskeyRegistrationProof{
		ID:           "proof-bound",
		Username:     "alice",
		CeremonyID:   "ceremony-bound",
		CredentialID: "cred-bound",
		PublicKey:    []byte("pk"),
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(context.Background(), proof))

	consumed, err := repo.ConsumePasskeyRegistrationProof(context.Background(), proof.ID, "mallory", proof.CeremonyID)
	require.Nil(t, consumed)
	require.Error(t, err)

	stored, getErr := repo.GetPasskeyRegistrationProof(context.Background(), proof.ID)
	require.NoError(t, getErr)
	require.False(t, stored.Consumed)
	require.True(t, stored.ConsumedAt.IsZero())
}

func TestAccountRepository_ConsumePasskeyRegistrationProof_ValidatesRequiredParams(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	ctx := context.Background()

	proof, err := repo.ConsumePasskeyRegistrationProof(ctx, "", "alice", "ceremony")
	require.Nil(t, proof)
	require.Error(t, err)

	proof, err = repo.ConsumePasskeyRegistrationProof(ctx, "proof", "", "ceremony")
	require.Nil(t, proof)
	require.Error(t, err)

	proof, err = repo.ConsumePasskeyRegistrationProof(ctx, "proof", "alice", "")
	require.Nil(t, proof)
	require.Error(t, err)
}

func TestAccountRepository_ConsumePasskeyRegistrationProof_MissingProof(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	proof, err := repo.ConsumePasskeyRegistrationProof(context.Background(), "missing-proof", "alice", "ceremony")
	require.Nil(t, proof)
	require.Error(t, err)
}

func TestAccountRepository_GetPasskeyRegistrationProof_QueryError(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	proof := &models.PasskeyRegistrationProof{
		ID:           "proof-query-error",
		Username:     "alice",
		CeremonyID:   "ceremony-query-error",
		CredentialID: "cred-query-error",
		PublicKey:    []byte("pk"),
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(context.Background(), proof))
	lookupErr := stdErrors.New("lookup failed")
	db.state.lookupErr = lookupErr

	got, err := repo.GetPasskeyRegistrationProof(context.Background(), proof.ID)
	require.Nil(t, got)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.CodeInternal, appErr.Code)
	require.Equal(t, apperrors.CategoryStorage, appErr.Category)
	require.ErrorIs(t, err, lookupErr)
}

func TestAccountRepository_DeletePasskeyRegistrationProof_DeleteError(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	proof := &models.PasskeyRegistrationProof{
		ID:           "proof-delete-error",
		Username:     "alice",
		CeremonyID:   "ceremony-delete-error",
		CredentialID: "cred-delete-error",
		PublicKey:    []byte("pk"),
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC(),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(context.Background(), proof))
	db.state.deleteErr = stdErrors.New("delete failed")

	require.Error(t, repo.DeletePasskeyRegistrationProof(context.Background(), proof.ID))
}

func TestAccountRepository_GetPasskeyRegistrationProof_ExpiredDeleteFailureStillReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := newPasskeyRegistrationProofDB()
	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	now := time.Now().UTC()
	proof := &models.PasskeyRegistrationProof{
		ID:           "expired-proof-delete-error",
		Username:     "alice",
		CeremonyID:   "ceremony-expired-delete-error",
		CredentialID: "cred-expired-delete-error",
		PublicKey:    []byte("pk"),
		CreatedAt:    now.Add(-15 * time.Minute),
		ExpiresAt:    now.Add(-5 * time.Minute),
	}

	require.NoError(t, repo.StorePasskeyRegistrationProof(context.Background(), proof))
	db.state.deleteErr = stdErrors.New("delete failed")

	got, err := repo.GetPasskeyRegistrationProof(context.Background(), proof.ID)
	require.Nil(t, got)
	require.ErrorIs(t, err, storage.ErrNotFound)

	db.state.mu.Lock()
	defer db.state.mu.Unlock()
	_, exists := db.state.proofs[proof.ID]
	require.True(t, exists)
}

type passkeyRegistrationProofDB struct {
	state *passkeyRegistrationProofState
}

type passkeyRegistrationProofState struct {
	mu     sync.Mutex
	proofs map[string]*models.PasskeyRegistrationProof

	lookupErr error
	deleteErr error
}

func newPasskeyRegistrationProofDB() *passkeyRegistrationProofDB {
	return &passkeyRegistrationProofDB{
		state: &passkeyRegistrationProofState{
			proofs: make(map[string]*models.PasskeyRegistrationProof),
		},
	}
}

func (db *passkeyRegistrationProofDB) Model(model any) core.Query {
	return &passkeyRegistrationProofQuery{
		state:  db.state,
		model:  model,
		wheres: make(map[string]any),
	}
}

func (db *passkeyRegistrationProofDB) Migrate() error { return nil }

func (db *passkeyRegistrationProofDB) AutoMigrate(...any) error { return nil }

func (db *passkeyRegistrationProofDB) Close() error { return nil }

func (db *passkeyRegistrationProofDB) WithContext(context.Context) core.DB { return db }

type passkeyRegistrationProofQuery struct {
	state       *passkeyRegistrationProofState
	model       any
	wheres      map[string]any
	ifNotExists bool
}

func (q *passkeyRegistrationProofQuery) Where(field, _ string, value any) core.Query {
	q.wheres[field] = value
	return q
}

func (q *passkeyRegistrationProofQuery) Index(string) core.Query                   { return q }
func (q *passkeyRegistrationProofQuery) Filter(string, string, any) core.Query     { return q }
func (q *passkeyRegistrationProofQuery) OrFilter(string, string, any) core.Query   { return q }
func (q *passkeyRegistrationProofQuery) FilterGroup(func(core.Query)) core.Query   { return q }
func (q *passkeyRegistrationProofQuery) OrFilterGroup(func(core.Query)) core.Query { return q }
func (q *passkeyRegistrationProofQuery) IfNotExists() core.Query                   { q.ifNotExists = true; return q }
func (q *passkeyRegistrationProofQuery) IfExists() core.Query                      { return q }
func (q *passkeyRegistrationProofQuery) WithCondition(string, string, any) core.Query {
	return q
}
func (q *passkeyRegistrationProofQuery) WithConditionExpression(string, map[string]any) core.Query {
	return q
}
func (q *passkeyRegistrationProofQuery) OrderBy(string, string) core.Query       { return q }
func (q *passkeyRegistrationProofQuery) Limit(int) core.Query                    { return q }
func (q *passkeyRegistrationProofQuery) Offset(int) core.Query                   { return q }
func (q *passkeyRegistrationProofQuery) Select(...string) core.Query             { return q }
func (q *passkeyRegistrationProofQuery) ConsistentRead() core.Query              { return q }
func (q *passkeyRegistrationProofQuery) WithRetry(int, time.Duration) core.Query { return q }
func (q *passkeyRegistrationProofQuery) All(any) error                           { return stdErrors.New("unsupported") }
func (q *passkeyRegistrationProofQuery) AllPaginated(any) (*core.PaginatedResult, error) {
	return nil, stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) Count() (int64, error) {
	return 0, stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) CreateOrUpdate() error                { return stdErrors.New("unsupported") }
func (q *passkeyRegistrationProofQuery) Update(...string) error               { return stdErrors.New("unsupported") }
func (q *passkeyRegistrationProofQuery) Scan(any) error                       { return stdErrors.New("unsupported") }
func (q *passkeyRegistrationProofQuery) ParallelScan(int32, int32) core.Query { return q }
func (q *passkeyRegistrationProofQuery) ScanAllSegments(any, int32) error {
	return stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) BatchGet([]any, any) error {
	return stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) BatchGetWithOptions([]any, any, *core.BatchGetOptions) error {
	return stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) BatchGetBuilder() core.BatchGetBuilder { return nil }
func (q *passkeyRegistrationProofQuery) BatchCreate(any) error                 { return stdErrors.New("unsupported") }
func (q *passkeyRegistrationProofQuery) BatchDelete([]any) error               { return stdErrors.New("unsupported") }
func (q *passkeyRegistrationProofQuery) BatchWrite([]any, []any) error {
	return stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) BatchUpdateWithOptions([]any, []string, ...any) error {
	return stdErrors.New("unsupported")
}
func (q *passkeyRegistrationProofQuery) Cursor(string) core.Query               { return q }
func (q *passkeyRegistrationProofQuery) SetCursor(string) error                 { return nil }
func (q *passkeyRegistrationProofQuery) WithContext(context.Context) core.Query { return q }

func (q *passkeyRegistrationProofQuery) First(dest any) error {
	proof, err := q.lookup()
	if err != nil {
		return err
	}

	target, ok := dest.(*models.PasskeyRegistrationProof)
	if !ok {
		return stdErrors.New("unsupported destination")
	}
	*target = *proof
	return nil
}

func (q *passkeyRegistrationProofQuery) Create() error {
	proof, ok := q.model.(*models.PasskeyRegistrationProof)
	if !ok {
		return stdErrors.New("unsupported model")
	}

	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	if q.ifNotExists {
		if _, exists := q.state.proofs[proof.ID]; exists {
			return fmt.Errorf("proof already exists")
		}
	}

	q.state.proofs[proof.ID] = clonePasskeyRegistrationProof(proof)
	return nil
}

func (q *passkeyRegistrationProofQuery) UpdateBuilder() core.UpdateBuilder {
	return &passkeyRegistrationProofUpdateBuilder{
		state:   q.state,
		proofID: q.resolveProofID(),
		sets:    make(map[string]any),
	}
}

func (q *passkeyRegistrationProofQuery) Delete() error {
	proofID := q.resolveProofID()
	if proofID == "" {
		return dynamormerrors.ErrItemNotFound
	}

	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	if q.state.deleteErr != nil {
		return q.state.deleteErr
	}

	if _, exists := q.state.proofs[proofID]; !exists {
		return dynamormerrors.ErrItemNotFound
	}

	delete(q.state.proofs, proofID)
	return nil
}

func (q *passkeyRegistrationProofQuery) lookup() (*models.PasskeyRegistrationProof, error) {
	proofID := q.resolveProofID()
	if proofID == "" {
		return nil, dynamormerrors.ErrItemNotFound
	}

	q.state.mu.Lock()
	defer q.state.mu.Unlock()

	if q.state.lookupErr != nil {
		return nil, q.state.lookupErr
	}

	proof, exists := q.state.proofs[proofID]
	if !exists {
		return nil, dynamormerrors.ErrItemNotFound
	}

	return clonePasskeyRegistrationProof(proof), nil
}

func (q *passkeyRegistrationProofQuery) resolveProofID() string {
	if pk, ok := q.wheres["PK"].(string); ok {
		const prefix = "PASSKEY_REGISTRATION_PROOF#"
		if len(pk) > len(prefix) && pk[:len(prefix)] == prefix {
			return pk[len(prefix):]
		}
	}

	if proof, ok := q.model.(*models.PasskeyRegistrationProof); ok {
		return proof.ID
	}

	return ""
}

type passkeyRegistrationProofUpdateBuilder struct {
	state      *passkeyRegistrationProofState
	proofID    string
	sets       map[string]any
	conditions []passkeyRegistrationProofCondition
}

type passkeyRegistrationProofCondition struct {
	field    string
	operator string
	value    any
}

func (b *passkeyRegistrationProofUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	b.sets[field] = value
	return b
}

func (b *passkeyRegistrationProofUpdateBuilder) SetIfNotExists(string, any, any) core.UpdateBuilder {
	return b
}
func (b *passkeyRegistrationProofUpdateBuilder) Add(string, any) core.UpdateBuilder    { return b }
func (b *passkeyRegistrationProofUpdateBuilder) Increment(string) core.UpdateBuilder   { return b }
func (b *passkeyRegistrationProofUpdateBuilder) Decrement(string) core.UpdateBuilder   { return b }
func (b *passkeyRegistrationProofUpdateBuilder) Remove(string) core.UpdateBuilder      { return b }
func (b *passkeyRegistrationProofUpdateBuilder) Delete(string, any) core.UpdateBuilder { return b }
func (b *passkeyRegistrationProofUpdateBuilder) AppendToList(string, any) core.UpdateBuilder {
	return b
}
func (b *passkeyRegistrationProofUpdateBuilder) PrependToList(string, any) core.UpdateBuilder {
	return b
}
func (b *passkeyRegistrationProofUpdateBuilder) RemoveFromListAt(string, int) core.UpdateBuilder {
	return b
}
func (b *passkeyRegistrationProofUpdateBuilder) SetListElement(string, int, any) core.UpdateBuilder {
	return b
}

func (b *passkeyRegistrationProofUpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	b.conditions = append(b.conditions, passkeyRegistrationProofCondition{field: field, operator: operator, value: value})
	return b
}

func (b *passkeyRegistrationProofUpdateBuilder) OrCondition(string, string, any) core.UpdateBuilder {
	return b
}
func (b *passkeyRegistrationProofUpdateBuilder) ConditionExists(string) core.UpdateBuilder { return b }
func (b *passkeyRegistrationProofUpdateBuilder) ConditionNotExists(string) core.UpdateBuilder {
	return b
}
func (b *passkeyRegistrationProofUpdateBuilder) ConditionVersion(int64) core.UpdateBuilder { return b }
func (b *passkeyRegistrationProofUpdateBuilder) ReturnValues(string) core.UpdateBuilder    { return b }

func (b *passkeyRegistrationProofUpdateBuilder) Execute() error {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()

	proof, exists := b.state.proofs[b.proofID]
	if !exists {
		return dynamormerrors.ErrItemNotFound
	}

	for _, condition := range b.conditions {
		if !passkeyRegistrationProofConditionMet(proof, condition) {
			return fmt.Errorf("conditional check failed")
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

	b.state.proofs[b.proofID] = clonePasskeyRegistrationProof(proof)
	return nil
}

func (b *passkeyRegistrationProofUpdateBuilder) ExecuteWithResult(any) error {
	return b.Execute()
}

func passkeyRegistrationProofConditionMet(proof *models.PasskeyRegistrationProof, condition passkeyRegistrationProofCondition) bool {
	switch condition.field {
	case "Consumed":
		expected, ok := condition.value.(bool)
		return ok && compareBool(proof.Consumed, condition.operator, expected)
	case "TTL":
		expected, ok := condition.value.(int64)
		return ok && compareInt64(proof.TTL, condition.operator, expected)
	case "Username":
		expected, ok := condition.value.(string)
		return ok && compareString(proof.Username, condition.operator, expected)
	case "CeremonyID":
		expected, ok := condition.value.(string)
		return ok && compareString(proof.CeremonyID, condition.operator, expected)
	default:
		return false
	}
}

func compareBool(actual bool, operator string, expected bool) bool {
	if operator != "=" {
		return false
	}
	return actual == expected
}

func compareInt64(actual int64, operator string, expected int64) bool {
	switch operator {
	case "=":
		return actual == expected
	case ">":
		return actual > expected
	default:
		return false
	}
}

func compareString(actual string, operator string, expected string) bool {
	if operator != "=" {
		return false
	}
	return actual == expected
}

func clonePasskeyRegistrationProof(proof *models.PasskeyRegistrationProof) *models.PasskeyRegistrationProof {
	if proof == nil {
		return nil
	}

	clone := *proof
	if proof.PublicKey != nil {
		clone.PublicKey = append([]byte(nil), proof.PublicKey...)
	}
	if proof.AAGUID != nil {
		clone.AAGUID = append([]byte(nil), proof.AAGUID...)
	}
	return &clone
}
