package reputation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round20CacheStore struct {
	mu sync.Mutex

	items     map[string]map[string]any
	firstErr  error
	createErr error
}

type round20CacheDB struct {
	store *round20CacheStore
}

func (db *round20CacheDB) WithContext(ctx context.Context) reputationCacheDB {
	_ = ctx
	return db
}

func (db *round20CacheDB) Model(model any) reputationCacheQuery {
	return &round20CacheQuery{store: db.store, model: model}
}

type round20CacheQuery struct {
	store *round20CacheStore
	model any

	pk string
	sk string
}

func (q *round20CacheQuery) Where(field string, op string, value any) reputationCacheQuery {
	if op != "=" {
		return q
	}
	val, ok := value.(string)
	if !ok {
		return q
	}
	switch field {
	case "PK":
		q.pk = val
	case "SK":
		q.sk = val
	}
	return q
}

func (q *round20CacheQuery) First(dest any) error {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()

	if q.store.firstErr != nil {
		return q.store.firstErr
	}
	if q.store.items == nil {
		return fmt.Errorf("not found")
	}
	fields, ok := q.store.items[q.pk+"|"+q.sk]
	if !ok {
		return fmt.Errorf("not found")
	}

	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("dest must be non-nil pointer")
	}
	elem := rv.Elem()
	for name, val := range fields {
		field := elem.FieldByName(name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		value := reflect.ValueOf(val)
		if value.IsValid() && value.Type().AssignableTo(field.Type()) {
			field.Set(value)
		}
	}
	return nil
}

func (q *round20CacheQuery) Create() error {
	q.store.mu.Lock()
	defer q.store.mu.Unlock()

	if q.store.createErr != nil {
		return q.store.createErr
	}
	rv := reflect.ValueOf(q.model)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("model must be non-nil pointer")
	}
	elem := rv.Elem()
	pkField := elem.FieldByName("PK")
	skField := elem.FieldByName("SK")
	if !pkField.IsValid() || !skField.IsValid() {
		return fmt.Errorf("model missing PK/SK")
	}
	pk := pkField.String()
	sk := skField.String()

	fields := map[string]any{}
	for _, name := range []string{"PK", "SK", "Value", "CachedAt", "TTL"} {
		f := elem.FieldByName(name)
		if f.IsValid() {
			fields[name] = f.Interface()
		}
	}
	if q.store.items == nil {
		q.store.items = make(map[string]map[string]any)
	}
	q.store.items[pk+"|"+sk] = fields
	return nil
}

type round20UserRepo struct {
	mu sync.Mutex

	getFn   func(ctx context.Context, actorID string) (*storage.Reputation, error)
	storeFn func(ctx context.Context, actorID string, reputation *storage.Reputation) error

	stored map[string]*storage.Reputation
}

func (r *round20UserRepo) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	if r.getFn != nil {
		return r.getFn(ctx, actorID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stored[actorID], nil
}

func (r *round20UserRepo) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	if r.storeFn != nil {
		return r.storeFn(ctx, actorID, reputation)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stored == nil {
		r.stored = make(map[string]*storage.Reputation)
	}
	r.stored[actorID] = reputation
	return nil
}

type round20ActorRepo struct {
	actorByUsername map[string]*activitypub.Actor
	errByUsername   map[string]error
	cachedRemote    map[string]*activitypub.Actor
	errRemote       map[string]error
	usernameCalls   int
	remoteCalls     int
}

func (r *round20ActorRepo) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	_ = ctx
	r.usernameCalls++
	if r.errByUsername != nil {
		if err, ok := r.errByUsername[username]; ok {
			return nil, err
		}
	}
	if actor, ok := r.actorByUsername[username]; ok {
		return actor, nil
	}
	return nil, fmt.Errorf("not found")
}

func (r *round20ActorRepo) GetCachedRemoteActor(ctx context.Context, identifier string) (*activitypub.Actor, error) {
	_ = ctx
	r.remoteCalls++
	if r.errRemote != nil {
		if err, ok := r.errRemote[identifier]; ok {
			return nil, err
		}
	}
	if actor, ok := r.cachedRemote[identifier]; ok {
		return actor, nil
	}
	return nil, fmt.Errorf("not found")
}

type round20StatusRepo struct {
	countByUsername map[string]int
	countErr        error
	countCalls      int

	timelineByUser map[string][]*models.Status
	timelineErr    error
}

func (r *round20StatusRepo) CountStatusesByAuthor(ctx context.Context, username string) (int, error) {
	_ = ctx
	r.countCalls++
	if r.countErr != nil {
		return 0, r.countErr
	}
	return r.countByUsername[username], nil
}

func (r *round20StatusRepo) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	_ = ctx
	_ = opts
	if r.timelineErr != nil {
		return nil, r.timelineErr
	}
	return &interfaces.PaginatedResult[*models.Status]{Items: r.timelineByUser[userID]}, nil
}

type round20ActivityRepo struct {
	activitiesByUser map[string][]*activitypub.Activity
	err              error
}

func (r *round20ActivityRepo) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	_ = ctx
	_ = limit
	_ = cursor
	if r.err != nil {
		return nil, "", r.err
	}
	return r.activitiesByUser[username], "", nil
}

type round20RelationshipRepo struct {
	followersByUser map[string][]string
	err             error
}

func (r *round20RelationshipRepo) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	_ = ctx
	_ = limit
	_ = cursor
	if r.err != nil {
		return nil, "", r.err
	}
	return r.followersByUser[username], "", nil
}

type round20TrustRepo struct {
	trusting    []*storage.TrustRelationship
	trustedBy   []*storage.TrustRelationship
	trustingErr error
	trustedErr  error
}

func (r *round20TrustRepo) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	_ = ctx
	_ = trusterID
	_ = limit
	_ = cursor
	if r.trustingErr != nil {
		return nil, "", r.trustingErr
	}
	return r.trusting, "", nil
}

func (r *round20TrustRepo) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	_ = ctx
	_ = trusteeID
	_ = limit
	_ = cursor
	if r.trustedErr != nil {
		return nil, "", r.trustedErr
	}
	return r.trustedBy, "", nil
}

type round20ModerationRepo struct {
	events  []*storage.ModerationEvent
	reports []*storage.Report

	eventsErr  error
	reportsErr error
}

func (r *round20ModerationRepo) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	_ = ctx
	_ = actorID
	_ = limit
	_ = cursor
	if r.eventsErr != nil {
		return nil, "", r.eventsErr
	}
	return r.events, "", nil
}

func (r *round20ModerationRepo) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	_ = ctx
	_ = targetAccountID
	_ = limit
	_ = cursor
	if r.reportsErr != nil {
		return nil, "", r.reportsErr
	}
	return r.reports, "", nil
}

type round20CommunityNoteRepo struct {
	notes         []*storage.CommunityNote
	votesByNoteID map[string][]*storage.CommunityNoteVote
	notesErr      error
	votesErr      error
}

func (r *round20CommunityNoteRepo) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	_ = ctx
	_ = authorID
	_ = limit
	_ = cursor
	if r.notesErr != nil {
		return nil, "", r.notesErr
	}
	return r.notes, "", nil
}

func (r *round20CommunityNoteRepo) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	_ = ctx
	if r.votesErr != nil {
		return nil, r.votesErr
	}
	return r.votesByNoteID[noteID], nil
}

type round20Calculator struct {
	rep   *Reputation
	err   error
	input *CalculationInput
}

func (c *round20Calculator) Calculate(ctx context.Context, input *CalculationInput) (*Reputation, error) {
	_ = ctx
	c.input = input
	if c.err != nil {
		return nil, c.err
	}
	return c.rep, nil
}

type round20Signer struct {
	signRepErr      error
	signPortableErr error
	publicKey       string
}

func (s *round20Signer) SignReputation(rep *Reputation) error {
	if s.signRepErr != nil {
		return s.signRepErr
	}
	rep.PublicKey = "pub"
	rep.Signature = "sig"
	return nil
}

func (s *round20Signer) SignPortableReputation(pr *PortableReputation) error {
	if s.signPortableErr != nil {
		return s.signPortableErr
	}
	pr.Issuer = "issuer"
	pr.IssuerProof = "proof"
	return nil
}

func (s *round20Signer) GetPublicKeyBase64() string {
	if s.publicKey == "" {
		return "pub"
	}
	return s.publicKey
}

type round20Verifier struct {
	verifyResult *VerificationResult
	verifyErr    error
}

func (v *round20Verifier) VerifyPortableReputation(pr *PortableReputation) (*VerificationResult, error) {
	_ = pr
	if v.verifyErr != nil {
		return nil, v.verifyErr
	}
	return v.verifyResult, nil
}

func (v *round20Verifier) VerifyVouchSignature(vouch *Vouch) (bool, error) {
	_ = vouch
	return true, nil
}

type round20VouchManager struct {
	vouchesForActor  []Vouch
	vouchesFromActor []Vouch

	forErr  error
	fromErr error

	imported  int
	importErr error

	createVouch *Vouch
	createErr   error

	revokeErr error
}

func (m *round20VouchManager) GetVouchesForActor(ctx context.Context, actorID string) ([]Vouch, error) {
	_ = ctx
	_ = actorID
	if m.forErr != nil {
		return nil, m.forErr
	}
	return m.vouchesForActor, nil
}

func (m *round20VouchManager) GetVouchesFromActor(ctx context.Context, actorID string) ([]Vouch, error) {
	_ = ctx
	_ = actorID
	if m.fromErr != nil {
		return nil, m.fromErr
	}
	return m.vouchesFromActor, nil
}

func (m *round20VouchManager) ImportVouches(ctx context.Context, vouches []Vouch, verifier vouchSignatureVerifier) (int, error) {
	_ = ctx
	_ = vouches
	_ = verifier
	return m.imported, m.importErr
}

func (m *round20VouchManager) CreateVouch(ctx context.Context, input *CreateVouchInput) (*Vouch, error) {
	_ = ctx
	_ = input
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createVouch, nil
}

func (m *round20VouchManager) RevokeVouch(ctx context.Context, vouchID, actorID string) error {
	_ = ctx
	_ = vouchID
	_ = actorID
	return m.revokeErr
}

func TestImportReputationForcesCanonicalActorPartitionForRemoteSameNameActors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	var storedPKs []string
	userRepo := &round20UserRepo{
		storeFn: func(_ context.Context, actorID string, reputation *storage.Reputation) error {
			reputationModel := &models.Reputation{}
			require.NoError(t, reputationModel.UpdateKeys(actorID, reputation))
			storedPKs = append(storedPKs, reputationModel.PK)
			return nil
		},
	}

	svc := &Service{
		userRepo:     userRepo,
		verifier:     &round20Verifier{verifyResult: &VerificationResult{Valid: true}},
		vouchManager: &round20VouchManager{},
		logger:       zap.NewNop(),
		instanceURL:  "https://local.example",
	}

	for _, doc := range []PortableReputation{
		{
			Actor:  "https://remote1.example/users/alice",
			Issuer: "https://remote1.example",
			Reputation: &Reputation{
				ActorID:      "https://remote1.example/users/alice",
				InstanceURL:  "https://remote1.example",
				TotalScore:   100,
				CalculatedAt: now,
				Version:      "1",
			},
		},
		{
			Actor:  "https://remote2.example/users/alice",
			Issuer: "https://remote2.example",
			Reputation: &Reputation{
				ActorID:      "https://remote2.example/users/alice",
				InstanceURL:  "https://remote2.example",
				TotalScore:   200,
				CalculatedAt: now.Add(time.Second),
				Version:      "1",
			},
		},
	} {
		docBytes, err := json.Marshal(doc)
		require.NoError(t, err)

		result, err := svc.ImportReputation(ctx, string(docBytes))
		require.NoError(t, err)
		require.True(t, result.Success)
	}

	require.Equal(t, []string{
		"ACTOR#https://remote1.example/users/alice",
		"ACTOR#https://remote2.example/users/alice",
	}, storedPKs)
	require.NotContains(t, storedPKs, "ACTOR#alice")
}

func TestImportReputationRejectsMismatchedOuterAndInnerActors(t *testing.T) {
	ctx := context.Background()
	storeCalled := false
	userRepo := &round20UserRepo{
		getFn: func(_ context.Context, _ string) (*storage.Reputation, error) {
			return nil, nil
		},
		storeFn: func(_ context.Context, _ string, _ *storage.Reputation) error {
			storeCalled = true
			return nil
		},
	}
	svc := &Service{
		userRepo:     userRepo,
		verifier:     &round20Verifier{verifyResult: &VerificationResult{Valid: true}},
		vouchManager: &round20VouchManager{},
		logger:       zap.NewNop(),
		instanceURL:  "https://local.example",
	}

	doc := PortableReputation{
		Actor:  "https://local.example/users/bob",
		Issuer: "https://remote1.example",
		Reputation: &Reputation{
			ActorID:      "https://remote1.example/users/alice",
			InstanceURL:  "https://remote1.example",
			TotalScore:   100,
			CalculatedAt: time.Now().UTC(),
			Version:      "1",
		},
	}
	docBytes, err := json.Marshal(doc)
	require.NoError(t, err)

	result, err := svc.ImportReputation(ctx, string(docBytes))
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "Reputation actor does not match document actor", result.Error)
	require.False(t, storeCalled, "mismatched imported reputation must not be stored")
}

func TestService_Round20_ExtractUsername_ParseSeverity_AndOutcome(t *testing.T) {
	svc := &Service{logger: zap.NewNop()}

	username, err := svc.extractUsername("https://example.com/users/alice")
	require.NoError(t, err)
	require.Equal(t, "alice", username)
	username, err = svc.extractUsername("https://example.com/@bob")
	require.NoError(t, err)
	require.Equal(t, "bob", username)
	username, err = svc.extractUsername("@carol")
	require.NoError(t, err)
	require.Equal(t, "carol", username)
	_, err = svc.extractUsername("https://example.com/users/admin/mallory")
	require.Error(t, err)

	require.Equal(t, 1, svc.parseSeverity("1"))
	require.Equal(t, 4, svc.parseSeverity("4"))
	require.Equal(t, 2, svc.parseSeverity("nope"))

	now := time.Now()
	require.Equal(t, "explicit", svc.deriveOutcomeFromEvent(&storage.ModerationEvent{
		EventType: "anything",
		Data:      map[string]interface{}{"outcome": "explicit"},
		Created:   now,
	}))
	require.Equal(t, "status", svc.deriveOutcomeFromEvent(&storage.ModerationEvent{
		EventType: "anything",
		Data:      map[string]interface{}{"status": "status"},
		Created:   now,
	}))
	require.Equal(t, OutcomeUpheld, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "warn"}))
	require.Equal(t, OutcomeUpheld, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "ban"}))
	require.Equal(t, OutcomeUpheld, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "report", Category: "spam"}))
	require.Equal(t, OutcomePending, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "report", Category: "other"}))
	require.Equal(t, OutcomeUpheld, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "appeal", ConfidenceScore: 0.9}))
	require.Equal(t, OutcomeDismissed, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "appeal", ConfidenceScore: 0.1}))
	require.Equal(t, OutcomePending, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "appeal", ConfidenceScore: 0.5}))
	require.Equal(t, OutcomePending, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "review"}))
	require.Equal(t, OutcomeDismissed, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "dismiss"}))
	require.Equal(t, OutcomeUpheld, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "confirmed"}))
	require.Equal(t, OutcomeDismissed, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "unknown", Severity: "low"}))
	require.Equal(t, OutcomeUpheld, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "unknown", Severity: "critical"}))
	require.Equal(t, OutcomePending, svc.deriveOutcomeFromEvent(&storage.ModerationEvent{EventType: "unknown"}))
}

func TestService_Round20_GetReputation_CalculateAndStore_ExportImport(t *testing.T) {
	ctx := context.Background()
	actorID := "https://example.com/users/alice"
	now := time.Now()

	cacheStore := &round20CacheStore{}
	cache := &round20CacheDB{store: cacheStore}

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: actorID, Published: ptrTime(now.AddDate(-1, 0, 0))},
	}

	statusTime := now.Add(-2 * time.Hour)
	outboxTime := now.Add(-1 * time.Hour)
	statuses := []*models.Status{
		{Note: &activitypub.Note{BaseObject: activitypub.BaseObject{Published: &statusTime}}},
	}
	activities := []*activitypub.Activity{
		{BaseObject: activitypub.BaseObject{Published: &outboxTime}},
	}

	userRepo := &round20UserRepo{}
	statusRepo := &round20StatusRepo{
		countByUsername: map[string]int{"alice": 12},
		timelineByUser:  map[string][]*models.Status{"alice": statuses},
	}

	svc := &Service{
		userRepo:          userRepo,
		actorRepo:         &round20ActorRepo{actorByUsername: map[string]*activitypub.Actor{"alice": actor}},
		statusRepo:        statusRepo,
		activityRepo:      &round20ActivityRepo{activitiesByUser: map[string][]*activitypub.Activity{"alice": activities}},
		relationshipRepo:  &round20RelationshipRepo{followersByUser: map[string][]string{actorID: {"f1", "f2"}}},
		trustRepo:         &round20TrustRepo{trusting: []*storage.TrustRelationship{{TrusterID: actorID, TrusteeID: "t1", Score: 0.5, Category: "general", Updated: now}}},
		moderationRepo:    &round20ModerationRepo{events: []*storage.ModerationEvent{{ID: "m1", EventType: "warn", Severity: "3", Created: now}}},
		communityNoteRepo: &round20CommunityNoteRepo{notes: []*storage.CommunityNote{{ID: "n1"}, {ID: "n2"}}, votesByNoteID: map[string][]*storage.CommunityNoteVote{"n1": {{Helpful: true}, {Helpful: false}}}},
		cache:             cache,
		calculator: &round20Calculator{
			rep: &Reputation{
				ActorID:      actorID,
				InstanceURL:  "https://example.com",
				TrustScore:   10,
				TotalScore:   10,
				CalculatedAt: now,
				Version:      "2",
			},
		},
		signer:       &round20Signer{},
		verifier:     &round20Verifier{verifyResult: &VerificationResult{Valid: true}},
		vouchManager: &round20VouchManager{vouchesForActor: []Vouch{{ID: "v1"}}, vouchesFromActor: []Vouch{{ID: "v2"}}, imported: 2, importErr: errors.New("partial")},
		logger:       zap.NewNop(),
		instanceURL:  "https://example.com",
	}

	t.Run("GetReputation_cached_fresh", func(t *testing.T) {
		userRepo.stored = map[string]*storage.Reputation{
			actorID: {
				ActorID:        actorID,
				InstanceURL:    "https://example.com",
				TrustScore:     1,
				TotalScore:     5,
				CalculatedAt:   now.Add(-1 * time.Hour),
				Version:        3,
				TrustingActors: []string{"a", "b"},
			},
		}

		rep, err := svc.GetReputation(ctx, actorID)
		require.NoError(t, err)
		require.Equal(t, 5, rep.TotalScore)
		require.Equal(t, 2, rep.TrustingActors)
	})

	t.Run("GetReputation_stale_triggers_recalc_and_store", func(t *testing.T) {
		userRepo.stored = map[string]*storage.Reputation{
			actorID: {ActorID: actorID, CalculatedAt: now.Add(-25 * time.Hour), Version: 1},
		}

		rep, err := svc.GetReputation(ctx, actorID)
		require.NoError(t, err)
		require.Equal(t, "sig", rep.Signature)
		require.NotNil(t, userRepo.stored[actorID])
	})

	t.Run("getPostCount_uses_cache_on_second_call", func(t *testing.T) {
		cacheStore.mu.Lock()
		cacheStore.items = nil
		cacheStore.mu.Unlock()
		statusRepo.countCalls = 0
		got1 := svc.getPostCount(ctx, actorID)
		got2 := svc.getPostCount(ctx, actorID)
		require.Equal(t, got1, got2)
		require.Equal(t, 1, statusRepo.countCalls)
	})

	t.Run("getLastActivityTime_caches_timestamp", func(t *testing.T) {
		ts1 := svc.getLastActivityTime(ctx, actorID)
		ts2 := svc.getLastActivityTime(ctx, actorID)
		require.Equal(t, ts1, ts2)
		require.True(t, ts1.Equal(outboxTime))
	})

	t.Run("ExportReputation_and_ImportReputation_paths", func(t *testing.T) {
		userRepo.stored = map[string]*storage.Reputation{
			actorID: {ActorID: actorID, TotalScore: 123, CalculatedAt: now, Version: 1},
		}

		pr, err := svc.ExportReputation(ctx, actorID)
		require.NoError(t, err)
		require.Equal(t, actorID, pr.Actor)
		require.Equal(t, "proof", pr.IssuerProof)

		docBytes, err := json.Marshal(pr)
		require.NoError(t, err)

		result, err := svc.ImportReputation(ctx, string(docBytes))
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Equal(t, 123, result.PreviousScore)
		require.Equal(t, 2, result.VouchesImported)
	})

	t.Run("ImportReputation_invalid_json_and_verification_failures", func(t *testing.T) {
		result, err := svc.ImportReputation(ctx, "not-json")
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Equal(t, "Invalid JSON document", result.Error)

		svc.verifier = &round20Verifier{verifyErr: errors.New("boom")}
		result, err = svc.ImportReputation(ctx, string(mustJSONMarshal(t, PortableReputation{Actor: actorID})))
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Contains(t, result.Error, "Verification failed")

		svc.verifier = &round20Verifier{verifyResult: &VerificationResult{Valid: false, Error: "nope"}}
		result, err = svc.ImportReputation(ctx, string(mustJSONMarshal(t, PortableReputation{Actor: actorID})))
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Equal(t, "nope", result.Error)
	})

	t.Run("storeReputation_error_and_version_parse_fallback", func(t *testing.T) {
		userRepo.storeFn = func(_ context.Context, _ string, rep *storage.Reputation) error {
			require.Equal(t, 0, rep.Version)
			return errors.New("store")
		}
		err := svc.storeReputation(ctx, &Reputation{ActorID: actorID, Version: "not-int"})
		require.Error(t, err)
		userRepo.storeFn = nil
	})

	t.Run("Vouch_and_verify_helpers", func(t *testing.T) {
		vm := &round20VouchManager{
			createVouch: &Vouch{ID: "new"},
		}
		svc.vouchManager = vm
		svc.signer = &round20Signer{publicKey: "pubkey"}
		svc.verifier = &round20Verifier{verifyResult: &VerificationResult{Valid: true}}

		v, err := svc.CreateVouch(ctx, "from", "to", 0.5, "ctx")
		require.NoError(t, err)
		require.Equal(t, "new", v.ID)

		require.NoError(t, svc.RevokeVouch(ctx, "vouchID", "actor"))
		vouches, err := svc.GetVouches(ctx, actorID)
		require.NoError(t, err)
		require.Empty(t, vouches)

		vr, err := svc.VerifyReputation(ctx, string(mustJSONMarshal(t, PortableReputation{Actor: actorID})))
		require.NoError(t, err)
		require.True(t, vr.Valid)
		require.Equal(t, "pubkey", svc.GetPublicKey())
	})

	t.Run("VerifyReputation_invalid_json", func(t *testing.T) {
		vr, err := svc.VerifyReputation(ctx, "{bad")
		require.NoError(t, err)
		require.False(t, vr.Valid)
	})
}

func TestService_GetReputation_RemoteActorDoesNotUseLocalUsername(t *testing.T) {
	ctx := context.Background()
	remoteActorID := "https://evil.example/users/alice"
	localActorID := "https://example.com/users/alice"
	actorRepo := &round20ActorRepo{
		actorByUsername: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: localActorID}},
		},
	}

	svc := &Service{
		userRepo:    &round20UserRepo{},
		actorRepo:   actorRepo,
		calculator:  &round20Calculator{rep: &Reputation{ActorID: remoteActorID, InstanceURL: "https://example.com", CalculatedAt: time.Now()}},
		signer:      &round20Signer{},
		logger:      zap.NewNop(),
		instanceURL: "https://example.com",
	}

	rep, err := svc.GetReputation(ctx, remoteActorID)
	require.Error(t, err)
	require.Nil(t, rep)
	require.Contains(t, err.Error(), "actor not found")
	require.Equal(t, 0, actorRepo.usernameCalls, "remote actor reputation must not fall back to local username lookup")
	require.Equal(t, 1, actorRepo.remoteCalls)
}

func TestService_L05RejectsCraftedActorURIWithoutStorageWrite(t *testing.T) {
	ctx := context.Background()
	craftedActorID := "https://example.com/users/admin/mallory"

	getCalls := 0
	storeCalls := 0
	userRepo := &round20UserRepo{
		getFn: func(_ context.Context, _ string) (*storage.Reputation, error) {
			getCalls++
			return nil, storage.ErrNotFound
		},
		storeFn: func(_ context.Context, _ string, _ *storage.Reputation) error {
			storeCalls++
			return nil
		},
	}
	actorRepo := &round20ActorRepo{
		actorByUsername: map[string]*activitypub.Actor{
			"admin": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/admin"}},
		},
	}
	svc := &Service{
		userRepo:    userRepo,
		actorRepo:   actorRepo,
		logger:      zap.NewNop(),
		instanceURL: "https://example.com",
	}

	for i := 0; i < 2; i++ {
		rep, err := svc.GetReputation(ctx, craftedActorID)
		require.Error(t, err)
		require.Nil(t, rep)
		require.Contains(t, err.Error(), "invalid actor ID")
	}
	require.Equal(t, 0, getCalls, "invalid crafted actor URI must not perform reputation storage reads")
	require.Equal(t, 0, actorRepo.usernameCalls, "invalid crafted actor URI must not resolve admin actor data")
	require.Equal(t, 0, storeCalls, "invalid crafted actor URI must not create reputation rows")
}

func TestService_L05RepeatedMissingLocalActorDoesNotStoreReputation(t *testing.T) {
	ctx := context.Background()
	actorID := "https://example.com/users/mallory"

	storeCalls := 0
	userRepo := &round20UserRepo{
		getFn: func(_ context.Context, _ string) (*storage.Reputation, error) {
			return nil, storage.ErrNotFound
		},
		storeFn: func(_ context.Context, _ string, _ *storage.Reputation) error {
			storeCalls++
			return nil
		},
	}
	actorRepo := &round20ActorRepo{}
	svc := &Service{
		userRepo:    userRepo,
		actorRepo:   actorRepo,
		logger:      zap.NewNop(),
		instanceURL: "https://example.com",
	}

	for i := 0; i < 3; i++ {
		rep, err := svc.GetReputation(ctx, actorID)
		require.Error(t, err)
		require.Nil(t, rep)
		require.Contains(t, err.Error(), "actor not found")
	}
	require.Equal(t, 3, actorRepo.usernameCalls, "valid but missing actors may be checked each read")
	require.Equal(t, 0, storeCalls, "never-resolvable actor reads must not append reputation rows")
}

func TestService_L05LocalActorResolutionMustMatchCanonicalActorID(t *testing.T) {
	ctx := context.Background()
	actorID := "https://example.com/users/mallory"

	storeCalls := 0
	userRepo := &round20UserRepo{
		getFn: func(_ context.Context, _ string) (*storage.Reputation, error) {
			return nil, storage.ErrNotFound
		},
		storeFn: func(_ context.Context, _ string, _ *storage.Reputation) error {
			storeCalls++
			return nil
		},
	}
	actorRepo := &round20ActorRepo{
		actorByUsername: map[string]*activitypub.Actor{
			"mallory": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/admin"}},
		},
	}
	svc := &Service{
		userRepo:    userRepo,
		actorRepo:   actorRepo,
		logger:      zap.NewNop(),
		instanceURL: "https://example.com",
	}

	rep, err := svc.GetReputation(ctx, actorID)
	require.Error(t, err)
	require.Nil(t, rep)
	require.Contains(t, err.Error(), "actor not found")
	require.Equal(t, 0, storeCalls, "mismatched actor resolution must not write reputation")
}

func TestService_L05CalculationUsernameMatchesStoragePartitionKey(t *testing.T) {
	ctx := context.Background()
	actorID := "https://example.com/users/alice"
	now := time.Now()
	var storedPK string

	userRepo := &round20UserRepo{
		getFn: func(_ context.Context, _ string) (*storage.Reputation, error) {
			return nil, storage.ErrNotFound
		},
		storeFn: func(_ context.Context, actorID string, reputation *storage.Reputation) error {
			reputationModel := &models.Reputation{}
			require.NoError(t, reputationModel.UpdateKeys(actorID, reputation))
			storedPK = reputationModel.PK
			return nil
		},
	}
	calculator := &round20Calculator{rep: &Reputation{
		ActorID:      actorID,
		InstanceURL:  "https://example.com",
		CalculatedAt: now,
		Version:      "1",
	}}
	svc := &Service{
		userRepo:          userRepo,
		actorRepo:         &round20ActorRepo{actorByUsername: map[string]*activitypub.Actor{"alice": {BaseObject: activitypub.BaseObject{ID: actorID, Published: ptrTime(now.AddDate(-1, 0, 0))}}}},
		statusRepo:        &round20StatusRepo{countByUsername: map[string]int{"alice": 1}, timelineByUser: map[string][]*models.Status{"alice": nil}},
		activityRepo:      &round20ActivityRepo{activitiesByUser: map[string][]*activitypub.Activity{"alice": nil}},
		relationshipRepo:  &round20RelationshipRepo{followersByUser: map[string][]string{actorID: nil}},
		trustRepo:         &round20TrustRepo{},
		moderationRepo:    &round20ModerationRepo{},
		communityNoteRepo: &round20CommunityNoteRepo{},
		cache:             &round20CacheDB{store: &round20CacheStore{}},
		calculator:        calculator,
		signer:            &round20Signer{},
		vouchManager:      &round20VouchManager{},
		logger:            zap.NewNop(),
		instanceURL:       "https://example.com",
	}

	rep, err := svc.GetReputation(ctx, actorID)
	require.NoError(t, err)
	require.NotNil(t, rep)
	require.Equal(t, "alice", calculator.input.ActorUsername)
	require.Equal(t, "ACTOR#alice", storedPK)
	require.Equal(t, "ACTOR#"+calculator.input.ActorUsername, storedPK)
}

func TestService_L05CalculationStorageKeyAssertionErrors(t *testing.T) {
	svc := &Service{instanceURL: "https://example.com", logger: zap.NewNop()}
	validInput := &CalculationInput{ActorID: "https://example.com/users/alice", ActorUsername: "alice"}
	validRep := &Reputation{ActorID: "https://example.com/users/alice"}

	require.Error(t, svc.assertCalculationMatchesStorageKey(nil, validRep))
	require.Error(t, svc.assertCalculationMatchesStorageKey(validInput, nil))

	err := svc.assertCalculationMatchesStorageKey(validInput, &Reputation{ActorID: "https://example.com/users/bob"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "actor mismatch")

	err = svc.assertCalculationMatchesStorageKey(
		&CalculationInput{ActorID: "https://example.com/users/alice", ActorUsername: "mallory"},
		validRep,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "calculation username")

	remoteSvc := &Service{instanceURL: "https://example.com", logger: zap.NewNop()}
	require.NoError(t, remoteSvc.assertCalculationMatchesStorageKey(
		&CalculationInput{ActorID: "https://remote.example/users/alice"},
		&Reputation{ActorID: "https://remote.example/users/alice"},
	))
}

func TestService_getActorData_UsesCachedRemoteActorForRemoteIDs(t *testing.T) {
	ctx := context.Background()
	remoteActorID := "https://remote.example/users/alice"
	actorRepo := &round20ActorRepo{
		cachedRemote: map[string]*activitypub.Actor{
			remoteActorID: {BaseObject: activitypub.BaseObject{ID: remoteActorID}},
		},
	}
	svc := &Service{
		actorRepo:   actorRepo,
		logger:      zap.NewNop(),
		instanceURL: "https://example.com",
	}

	actor, err := svc.getActorData(ctx, remoteActorID, "alice")
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, remoteActorID, actor.ID)
	require.Equal(t, 0, actorRepo.usernameCalls)
	require.Equal(t, 1, actorRepo.remoteCalls)
}

func TestService_Round20_CacheExpiry_And_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	cacheStore := &round20CacheStore{}
	cache := &round20CacheDB{store: cacheStore}

	svc := &Service{
		cache:  cache,
		logger: zap.NewNop(),
	}

	// Insert an expired metric directly into cache store.
	cacheStore.mu.Lock()
	cacheStore.items = map[string]map[string]any{
		"REPUTATION_CACHE#x|METRIC": {
			"PK":       "REPUTATION_CACHE#x",
			"SK":       "METRIC",
			"Value":    99,
			"CachedAt": time.Now().Add(-2 * time.Hour),
			"TTL":      int64(0),
		},
	}
	cacheStore.mu.Unlock()

	require.Equal(t, -1, svc.getCachedMetric(ctx, "x", 1*time.Minute))

	cacheStore.firstErr = errors.New("db down")
	require.Equal(t, -1, svc.getCachedMetric(ctx, "x", 10*time.Minute))

	cacheStore.firstErr = nil
	cacheStore.createErr = errors.New("create down")
	svc.setCachedMetric(ctx, "y", 1)
	svc.setCachedTimestamp(ctx, "z", time.Now())
}

func TestService_Round20_DynamormCacheWrappers_NilDB(t *testing.T) {
	db := dynamormReputationCacheDB{}
	require.NotNil(t, db.WithContext(context.Background()))

	q := db.Model(&struct{}{}).Where("PK", "=", "x").Where("SK", "=", "y")
	require.Error(t, q.First(&struct{}{}))
	require.Error(t, q.Create())
}

func ptrTime(t time.Time) *time.Time { return &t }

func mustJSONMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestService_Round20_ErrorAndEdgeBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("NewService_storage_required", func(t *testing.T) {
		_, err := NewService(&Config{})
		require.Error(t, err)
	})

	t.Run("isDuplicateTrust_true_and_false", func(t *testing.T) {
		svc := &Service{logger: zap.NewNop()}
		relationships := []TrustRelationship{{FromActor: "a", ToActor: "b"}}
		require.True(t, svc.isDuplicateTrust(relationships, &storage.TrustRelationship{TrusterID: "a", TrusteeID: "b"}))
		require.False(t, svc.isDuplicateTrust(relationships, &storage.TrustRelationship{TrusterID: "a", TrusteeID: "c"}))
	})

	t.Run("getActorData_error", func(t *testing.T) {
		svc := &Service{
			actorRepo: &round20ActorRepo{errByUsername: map[string]error{"alice": errors.New("db")}},
			logger:    zap.NewNop(),
		}
		_, err := svc.getActorData(ctx, "id", "alice")
		require.Error(t, err)
	})

	t.Run("gatherActorBasics_default_when_published_missing", func(t *testing.T) {
		svc := &Service{logger: zap.NewNop()}
		input := &CalculationInput{}
		svc.gatherActorBasics(input, &activitypub.Actor{})
		require.WithinDuration(t, time.Now().AddDate(-1, 0, 0), input.AccountCreated, 2*time.Second)
	})

	t.Run("getPostCount_error_returns_zero", func(t *testing.T) {
		svc := &Service{
			statusRepo: &round20StatusRepo{countErr: errors.New("db")},
			cache:      &round20CacheDB{store: &round20CacheStore{}},
			logger:     zap.NewNop(),
		}
		require.Equal(t, 0, svc.getPostCount(ctx, "https://example.com/users/alice"))
	})

	t.Run("getLastStatusTime_error_and_empty_and_missing_fields", func(t *testing.T) {
		svc := &Service{
			statusRepo: &round20StatusRepo{timelineErr: errors.New("db")},
			logger:     zap.NewNop(),
		}
		require.True(t, svc.getLastStatusTime(ctx, "alice").IsZero())

		svc.statusRepo = &round20StatusRepo{timelineByUser: map[string][]*models.Status{"alice": nil}}
		require.True(t, svc.getLastStatusTime(ctx, "alice").IsZero())

		svc.statusRepo = &round20StatusRepo{timelineByUser: map[string][]*models.Status{"alice": {{Note: nil}}}}
		require.True(t, svc.getLastStatusTime(ctx, "alice").IsZero())

		svc.statusRepo = &round20StatusRepo{timelineByUser: map[string][]*models.Status{"alice": {{Note: &activitypub.Note{}}}}}
		require.True(t, svc.getLastStatusTime(ctx, "alice").IsZero())
	})

	t.Run("getLastOutboxActivityTime_error_and_empty_and_missing_published", func(t *testing.T) {
		svc := &Service{
			activityRepo: &round20ActivityRepo{err: errors.New("db")},
			logger:       zap.NewNop(),
		}
		require.True(t, svc.getLastOutboxActivityTime(ctx, "alice").IsZero())

		svc.activityRepo = &round20ActivityRepo{activitiesByUser: map[string][]*activitypub.Activity{"alice": nil}}
		require.True(t, svc.getLastOutboxActivityTime(ctx, "alice").IsZero())

		svc.activityRepo = &round20ActivityRepo{activitiesByUser: map[string][]*activitypub.Activity{"alice": {{}}}}
		require.True(t, svc.getLastOutboxActivityTime(ctx, "alice").IsZero())
	})

	t.Run("getLastActivityTime_prefers_status_and_defaults_when_no_activity", func(t *testing.T) {
		statusTime := now.Add(-1 * time.Hour)
		outboxTime := now.Add(-2 * time.Hour)
		svc := &Service{
			statusRepo:   &round20StatusRepo{timelineByUser: map[string][]*models.Status{"alice": {{Note: &activitypub.Note{BaseObject: activitypub.BaseObject{Published: &statusTime}}}}}},
			activityRepo: &round20ActivityRepo{activitiesByUser: map[string][]*activitypub.Activity{"alice": {{BaseObject: activitypub.BaseObject{Published: &outboxTime}}}}},
			cache:        &round20CacheDB{store: &round20CacheStore{}},
			logger:       zap.NewNop(),
		}
		require.True(t, svc.getLastActivityTime(ctx, "alice").Equal(statusTime))

		svc2 := &Service{
			statusRepo:   &round20StatusRepo{timelineByUser: map[string][]*models.Status{"alice": nil}},
			activityRepo: &round20ActivityRepo{activitiesByUser: map[string][]*activitypub.Activity{"alice": nil}},
			cache:        &round20CacheDB{store: &round20CacheStore{}},
			logger:       zap.NewNop(),
		}
		got := svc2.getLastActivityTime(ctx, "alice")
		require.WithinDuration(t, time.Now().Add(-30*24*time.Hour), got, 2*time.Second)
	})

	t.Run("getFollowerCount_error_returns_zero", func(t *testing.T) {
		svc := &Service{
			relationshipRepo: &round20RelationshipRepo{err: errors.New("db")},
			logger:           zap.NewNop(),
		}
		require.Equal(t, 0, svc.getFollowerCount(ctx, "alice"))
	})

	t.Run("addOutgoingTrust_addIncomingTrust_and_report_errors", func(t *testing.T) {
		svc := &Service{
			trustRepo:      &round20TrustRepo{trustingErr: errors.New("trust"), trustedErr: errors.New("trusted")},
			moderationRepo: &round20ModerationRepo{reportsErr: errors.New("reports")},
			logger:         zap.NewNop(),
		}

		var relationships []TrustRelationship
		svc.addOutgoingTrust(ctx, "actor", &relationships)
		svc.addIncomingTrust(ctx, "actor", &relationships)
		require.Empty(t, relationships)

		var history []ModerationEvent
		svc.addReportEvents(ctx, "actor", &history)
		require.Empty(t, history)

		svc.trustRepo = &round20TrustRepo{trustedBy: []*storage.TrustRelationship{{TrusterID: "a", TrusteeID: "b", Updated: now}}}
		relationships = []TrustRelationship{{FromActor: "a", ToActor: "b"}}
		svc.addIncomingTrust(ctx, "actor", &relationships)
		require.Len(t, relationships, 1)
	})

	t.Run("gatherVouches_and_community_note_errors", func(t *testing.T) {
		vm := &round20VouchManager{forErr: errors.New("vouches"), fromErr: errors.New("vouches")}
		notes := &round20CommunityNoteRepo{notesErr: errors.New("notes"), votesErr: errors.New("votes")}

		svc := &Service{
			vouchManager:      vm,
			communityNoteRepo: notes,
			logger:            zap.NewNop(),
		}

		input := &CalculationInput{}
		svc.gatherVouches(ctx, input, "actor")
		require.Empty(t, input.VouchesReceived)
		require.Empty(t, input.VouchesGiven)

		require.Empty(t, svc.getCommunityNotes(ctx, "actor"))
		require.Equal(t, 0, svc.countNoteHelpfulVotes(ctx, "note"))
	})

	t.Run("ExportReputation_error_paths_and_VerifyReputation_verifier_error", func(t *testing.T) {
		userRepo := &round20UserRepo{getFn: func(_ context.Context, _ string) (*storage.Reputation, error) {
			return nil, errors.New("db")
		}}
		svc := &Service{
			userRepo: userRepo,
			logger:   zap.NewNop(),
			verifier: &round20Verifier{verifyErr: errors.New("verify")},
		}
		_, err := svc.ExportReputation(ctx, "actor")
		require.Error(t, err)

		vr, err := svc.VerifyReputation(ctx, string(mustJSONMarshal(t, PortableReputation{Actor: "actor"})))
		require.Error(t, err)
		require.Nil(t, vr)

		// Success GetReputation, but vouch lookup/signing failures.
		userRepo.getFn = nil
		userRepo.stored = map[string]*storage.Reputation{"actor": {ActorID: "actor", CalculatedAt: time.Now(), Version: 1}}
		svc.vouchManager = &round20VouchManager{forErr: errors.New("vouch")}
		_, err = svc.ExportReputation(ctx, "actor")
		require.Error(t, err)

		svc.vouchManager = &round20VouchManager{}
		svc.signer = &round20Signer{signPortableErr: errors.New("sign")}
		_, err = svc.ExportReputation(ctx, "actor")
		require.Error(t, err)
	})

	t.Run("getCachedTimestamp_expired_and_nil_cache", func(t *testing.T) {
		svc := &Service{logger: zap.NewNop()}
		require.True(t, svc.getCachedTimestamp(ctx, "x", 1*time.Second).IsZero())

		cacheStore := &round20CacheStore{
			items: map[string]map[string]any{
				"REPUTATION_CACHE#x|TIMESTAMP": {
					"PK":       "REPUTATION_CACHE#x",
					"SK":       "TIMESTAMP",
					"Value":    time.Now().Add(-5 * time.Hour),
					"CachedAt": time.Now().Add(-5 * time.Hour),
					"TTL":      int64(0),
				},
			},
		}
		svc.cache = &round20CacheDB{store: cacheStore}
		require.True(t, svc.getCachedTimestamp(ctx, "x", 1*time.Minute).IsZero())
	})

	t.Run("calculateAndStore_error_paths", func(t *testing.T) {
		svc := &Service{
			actorRepo: &round20ActorRepo{errByUsername: map[string]error{"alice": errors.New("actor")}},
			logger:    zap.NewNop(),
			cache:     &round20CacheDB{store: &round20CacheStore{}},
		}
		_, err := svc.calculateAndStore(ctx, "https://example.com/users/alice")
		require.Error(t, err)

		svc.actorRepo = &round20ActorRepo{actorByUsername: map[string]*activitypub.Actor{"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}}}
		svc.statusRepo = &round20StatusRepo{}
		svc.activityRepo = &round20ActivityRepo{}
		svc.relationshipRepo = &round20RelationshipRepo{}
		svc.trustRepo = &round20TrustRepo{}
		svc.moderationRepo = &round20ModerationRepo{}
		svc.communityNoteRepo = &round20CommunityNoteRepo{}
		svc.vouchManager = &round20VouchManager{}

		svc.calculator = &round20Calculator{err: errors.New("calc")}
		_, err = svc.calculateAndStore(ctx, "https://example.com/users/alice")
		require.Error(t, err)

		svc.calculator = &round20Calculator{rep: &Reputation{ActorID: "https://example.com/users/alice", CalculatedAt: time.Now(), Version: "1"}}
		svc.signer = &round20Signer{signRepErr: errors.New("sign")}
		_, err = svc.calculateAndStore(ctx, "https://example.com/users/alice")
		require.Error(t, err)

		svc.signer = &round20Signer{}
		svc.userRepo = &round20UserRepo{storeFn: func(_ context.Context, _ string, _ *storage.Reputation) error { return errors.New("store") }}
		_, err = svc.calculateAndStore(ctx, "https://example.com/users/alice")
		require.Error(t, err)
	})
}
