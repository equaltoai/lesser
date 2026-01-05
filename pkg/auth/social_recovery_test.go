package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type inMemorySocialRecoveryRepo struct {
	trusteesByUser      map[string][]*storage.TrusteeConfig
	requestsByID        map[string]*storage.SocialRecoveryRequest
	recoveryTokensByKey map[string]map[string]any

	errStoreTrustee error
}

func newInMemorySocialRecoveryRepo() *inMemorySocialRecoveryRepo {
	return &inMemorySocialRecoveryRepo{
		trusteesByUser:      make(map[string][]*storage.TrusteeConfig),
		requestsByID:        make(map[string]*storage.SocialRecoveryRequest),
		recoveryTokensByKey: make(map[string]map[string]any),
	}
}

func (r *inMemorySocialRecoveryRepo) StoreTrustee(_ context.Context, username string, trustee *storage.TrusteeConfig) error {
	if r.errStoreTrustee != nil {
		return r.errStoreTrustee
	}
	r.trusteesByUser[username] = append(r.trusteesByUser[username], trustee)
	return nil
}

func (r *inMemorySocialRecoveryRepo) DeleteTrustee(_ context.Context, username, trusteeActorID string) error {
	var remaining []*storage.TrusteeConfig
	for _, t := range r.trusteesByUser[username] {
		if t.ActorID != trusteeActorID {
			remaining = append(remaining, t)
		}
	}
	r.trusteesByUser[username] = remaining
	return nil
}

func (r *inMemorySocialRecoveryRepo) GetTrustees(_ context.Context, username string) ([]*storage.TrusteeConfig, error) {
	return append([]*storage.TrusteeConfig(nil), r.trusteesByUser[username]...), nil
}

func (r *inMemorySocialRecoveryRepo) StoreRecoveryRequest(_ context.Context, request *storage.SocialRecoveryRequest) error {
	r.requestsByID[request.ID] = request
	return nil
}

func (r *inMemorySocialRecoveryRepo) GetRecoveryRequest(_ context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	return r.requestsByID[requestID], nil
}

func (r *inMemorySocialRecoveryRepo) UpdateRecoveryRequest(_ context.Context, request *storage.SocialRecoveryRequest) error {
	r.requestsByID[request.ID] = request
	return nil
}

func (r *inMemorySocialRecoveryRepo) StoreRecoveryToken(_ context.Context, key string, data map[string]any) error {
	r.recoveryTokensByKey[key] = data
	return nil
}

type fakeSocialRecoveryFederation struct {
	invites   []string
	requests  []string
	approvals []string
	err       error
}

func (f *fakeSocialRecoveryFederation) SendTrusteeInvitation(_ context.Context, fromUser string, trusteeActorID string) error {
	f.invites = append(f.invites, fromUser+"->"+trusteeActorID)
	return f.err
}

func (f *fakeSocialRecoveryFederation) SendRecoveryRequest(_ context.Context, request *storage.SocialRecoveryRequest, trusteeActorID string) error {
	f.requests = append(f.requests, request.ID+"->"+trusteeActorID)
	return f.err
}

func (f *fakeSocialRecoveryFederation) SendRecoveryApprovalNotification(_ context.Context, username string, recoveryToken string) error {
	f.approvals = append(f.approvals, username+"#"+recoveryToken)
	return f.err
}

func TestSocialRecoveryService_AddRemoveTrustee_AndFederationHook(t *testing.T) {
	t.Parallel()

	repo := newInMemorySocialRecoveryRepo()
	svc := &SocialRecoveryService{repo: repo, logger: zap.NewNop()}

	require.ErrorIs(t, svc.AddTrustee(context.Background(), "alice", ""), ErrTrusteeActorIDRequired)

	fed := &fakeSocialRecoveryFederation{err: errors.New("federation down")}
	svc.SetFederationService(fed)
	require.Error(t, svc.AddTrustee(context.Background(), "alice", "https://example.com/users/bob"))
	require.Len(t, fed.invites, 1)

	// Remove still succeeds locally.
	require.NoError(t, svc.RemoveTrustee(context.Background(), "alice", "https://example.com/users/bob"))
}

func TestSocialRecoveryService_InitiateAndConfirmRecovery_HappyPath(t *testing.T) {
	t.Parallel()

	repo := newInMemorySocialRecoveryRepo()
	fed := &fakeSocialRecoveryFederation{}
	svc := &SocialRecoveryService{repo: repo, logger: zap.NewNop(), fedService: fed}

	repo.trusteesByUser["alice"] = []*storage.TrusteeConfig{
		{Username: "alice", ActorID: "https://example.com/users/bob", Confirmed: true},
		{Username: "alice", ActorID: "https://example.com/users/carla", Confirmed: true},
	}

	request, err := svc.InitiateRecovery(context.Background(), "alice")
	require.NoError(t, err)
	require.NotEmpty(t, request.ID)
	require.Equal(t, "pending", request.Status)
	require.Len(t, fed.requests, 2)

	require.NoError(t, svc.ConfirmRecovery(context.Background(), request.ID, "https://example.com/users/bob"))
	require.ErrorIs(t, svc.ConfirmRecovery(context.Background(), request.ID, "https://example.com/users/bob"), ErrTrusteeAlreadyVoted)

	require.NoError(t, svc.ConfirmRecovery(context.Background(), request.ID, "https://example.com/users/carla"))

	updated := repo.requestsByID[request.ID]
	require.Equal(t, "approved", updated.Status)
	require.GreaterOrEqual(t, updated.ReceivedVotes, updated.RequiredVotes)
	require.NotEmpty(t, repo.recoveryTokensByKey["RECOVERY#"+updated.RecoveryToken])
	require.Len(t, fed.approvals, 1)
}

func TestSocialRecoveryService_ConfirmRecovery_ExpiredRequest(t *testing.T) {
	t.Parallel()

	repo := newInMemorySocialRecoveryRepo()
	svc := &SocialRecoveryService{repo: repo, logger: zap.NewNop()}

	repo.requestsByID["req-1"] = &storage.SocialRecoveryRequest{
		ID:        "req-1",
		Username:  "alice",
		Status:    "pending",
		ExpiresAt: time.Now().Add(-time.Minute),
	}

	require.ErrorIs(t, svc.ConfirmRecovery(context.Background(), "req-1", "https://example.com/users/bob"), ErrRecoveryRequestExpired)
	require.Equal(t, "expired", repo.requestsByID["req-1"].Status)
}
