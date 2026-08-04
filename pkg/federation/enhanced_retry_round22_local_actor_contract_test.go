package federation

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestEnhancedRetryProcessor_Round22_UsesCanonicalLocalActorFromRepositoryAdapter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	cfg := lesserconfig.Get()
	previousDomain := cfg.Domain
	cfg.Domain = "example.com"
	t.Cleanup(func() {
		cfg.Domain = previousDomain
	})

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	privateKeyPEM, err := EncodePrivateKeyPEM(privateKey)
	require.NoError(t, err)

	actorRepo := newNestedBaseObjectSigningActorRepository(t, "alice")
	stub := &canonicalActorReadbackStub{
		backing:    actorRepo,
		privateKey: string(privateKeyPEM),
	}
	federationRepo := &enhancedRetryFederationRepoStub{}

	store := &RepositoryStorageAdapter{
		actorRepo:      stub,
		federationRepo: federationRepo,
	}

	httpStub := &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
		require.Contains(t, req.Header.Get("Signature"), `keyId="https://example.com/users/alice#main-key"`)
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	}}

	deliverySvc := &DeliveryService{
		store:      store,
		httpClient: httpStub,
		logger:     logger,
	}

	processor := NewEnhancedRetryProcessor(deliverySvc, nil, "")
	msg := &EnhancedRetryMessage{
		DeliveryID:        "retry-1",
		Activity:          &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.AcceptType}},
		SigningActorID:    "alice",
		ActivityType:      activitypub.AcceptType,
		RetryCount:        1,
		MaxRetries:        25,
		MaxRetryDuration:  20 * 24 * time.Hour,
		CreatedAt:         time.Now().Add(-1 * time.Hour),
		TargetInboxes:     []string{"https://remote.example/inbox"},
		FailedInboxes:     map[string]string{},
		SuccessfulInboxes: []string{},
	}

	require.NoError(t, processor.ProcessEnhancedRetry(ctx, msg))
	require.NotNil(t, stub.lastActor)
	require.Equal(t, "https://example.com/users/alice", stub.lastActor.ID)
	require.Equal(t, activitypub.ServiceType, stub.lastActor.Type)
	require.Equal(t, "alice", stub.lastActor.PreferredUsername)
	require.Equal(t, "https://example.com/users/alice/inbox", stub.lastActor.Inbox)
	require.Equal(t, "https://example.com/users/alice#main-key", stub.lastActor.PublicKey.ID)
	require.NotNil(t, stub.lastActor.Endpoints)
	require.Equal(t, "https://example.com/inbox", stub.lastActor.Endpoints.SharedInbox)
	recorded := federationRepo.snapshot()
	require.NotEmpty(t, recorded)

	foundFinalSuccess := false
	for _, activity := range recorded {
		if activity != nil && activity.ID == "retry-1" && activity.Status == "delivered_with_retry" {
			foundFinalSuccess = true
			break
		}
	}
	require.True(t, foundFinalSuccess, "expected delivered_with_retry record for retry-1")
}

type canonicalActorReadbackStub struct {
	backing    *repositories.ActorRepository
	privateKey string
	lastActor  *activitypub.Actor
}

type enhancedRetryFederationRepoStub struct {
	mu         sync.Mutex
	activities []*storage.FederationActivity
}

func (s *enhancedRetryFederationRepoStub) RecordFederationActivity(_ context.Context, activity *storage.FederationActivity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activities = append(s.activities, activity)
	return nil
}

func (s *enhancedRetryFederationRepoStub) snapshot() []*storage.FederationActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*storage.FederationActivity(nil), s.activities...)
}

func (s *canonicalActorReadbackStub) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	actor, err := s.backing.GetActorByUsername(ctx, username)
	if err == nil {
		s.lastActor = actor
	}
	return actor, err
}

func (s *canonicalActorReadbackStub) GetActorPrivateKey(context.Context, string) (string, error) {
	return s.privateKey, nil
}

func (s *canonicalActorReadbackStub) GetCachedRemoteActor(context.Context, string) (*activitypub.Actor, error) {
	return nil, nil
}

func newNestedBaseObjectSigningActorRepository(t *testing.T, username string) *repositories.ActorRepository {
	t.Helper()

	actorDB := new(dynamormmocks.MockDB)
	actorQuery := new(dynamormmocks.MockQuery)

	actorDB.On("WithContext", mock.Anything).Return(actorDB).Maybe()
	actorDB.On("Model", mock.Anything).Return(actorQuery).Maybe()
	actorQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(actorQuery).Maybe()
	actorQuery.On("Select", mock.Anything).Return(actorQuery).Maybe()
	actorQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		row := args.Get(0).(*models.Actor)
		row.Username = username
		row.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/ALICE",
				Type: activitypub.ServiceType,
			},
			PreferredUsername: username,
			URL:               "https://example.com/@ALICE",
			Inbox:             "https://example.com/users/ALICE/inbox",
			Outbox:            "https://example.com/users/ALICE/outbox",
			Followers:         "https://example.com/users/ALICE/followers",
			Following:         "https://example.com/users/ALICE/following",
			Liked:             "https://example.com/users/ALICE/liked",
			Endpoints: &activitypub.Endpoints{
				SharedInbox: "https://example.com/inbox",
			},
			PublicKey: &activitypub.PublicKey{
				ID:           "https://example.com/users/alice#main-key",
				Owner:        "https://example.com/users/alice",
				PublicKeyPem: "unused-in-test",
			},
		}
		row.CreatedAt = time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
		row.UpdatedAt = time.Date(2026, 4, 7, 12, 5, 0, 0, time.UTC)
	}).Once()

	t.Cleanup(func() {
		actorDB.AssertExpectations(t)
		actorQuery.AssertExpectations(t)
	})

	return repositories.NewActorRepository(actorDB, "test-table", zap.NewNop(), "example.com")
}
