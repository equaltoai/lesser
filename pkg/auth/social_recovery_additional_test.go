package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type socialRecoveryRepoDeleteErr struct {
	*inMemorySocialRecoveryRepo
	err error
}

func (r *socialRecoveryRepoDeleteErr) DeleteTrustee(_ context.Context, _, _ string) error { return r.err }

type socialRecoveryRepoGetTrusteesErr struct {
	*inMemorySocialRecoveryRepo
	err error
}

func (r *socialRecoveryRepoGetTrusteesErr) GetTrustees(_ context.Context, _ string) ([]*storage.TrusteeConfig, error) {
	return nil, r.err
}

type socialRecoveryRepoStoreTokenErr struct {
	*inMemorySocialRecoveryRepo
	err error
}

func (r *socialRecoveryRepoStoreTokenErr) StoreRecoveryToken(_ context.Context, _ string, _ map[string]any) error {
	return r.err
}

type socialRecoveryRepoUpdateErr struct {
	*inMemorySocialRecoveryRepo
	err error
}

func (r *socialRecoveryRepoUpdateErr) UpdateRecoveryRequest(_ context.Context, _ *storage.SocialRecoveryRequest) error {
	return r.err
}

func TestNewSocialRecoveryService_WiresRepo(t *testing.T) {
	t.Parallel()

	repos := testmocks.NewMockRepositoryStorage()
	svc := NewSocialRecoveryService(repos, zap.NewNop())
	require.NotNil(t, svc)
	require.NotNil(t, svc.repo)
	require.Nil(t, svc.fedService)
}

func TestSocialRecoveryService_FallbackNotificationsAndErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemorySocialRecoveryRepo()
	svc := &SocialRecoveryService{repo: repo, logger: zap.NewNop()}

	// notifyTrusteeAdded fallback (no federation service).
	require.NoError(t, svc.AddTrustee(context.Background(), "alice", "https://example.com/users/bob"))

	// StoreTrustee error.
	repo.errStoreTrustee = errors.New("store failed")
	require.ErrorIs(t, svc.AddTrustee(context.Background(), "alice", "https://example.com/users/carla"), ErrTrusteeStorage)

	// DeleteTrustee error.
	svc.repo = &socialRecoveryRepoDeleteErr{inMemorySocialRecoveryRepo: repo, err: errors.New("delete failed")}
	require.ErrorIs(t, svc.RemoveTrustee(context.Background(), "alice", "https://example.com/users/bob"), ErrTrusteeDeletion)

	// GetTrustees error.
	svc.repo = &socialRecoveryRepoGetTrusteesErr{inMemorySocialRecoveryRepo: repo, err: errors.New("get failed")}
	_, err := svc.InitiateRecovery(context.Background(), "alice")
	require.ErrorIs(t, err, ErrTrusteeRetrieval)
}

func TestSocialRecoveryService_ConfirmRecovery_FallbackApprovalAndFailurePaths(t *testing.T) {
	t.Parallel()

	repo := newInMemorySocialRecoveryRepo()
	svc := &SocialRecoveryService{repo: repo, logger: zap.NewNop()}

	// Not found.
	require.ErrorIs(t, svc.ConfirmRecovery(context.Background(), "missing", "trustee"), ErrRecoveryRequestNotFound)

	// Not pending.
	repo.requestsByID["done"] = &storage.SocialRecoveryRequest{ID: "done", Username: "alice", Status: "approved", ExpiresAt: time.Now().Add(time.Hour)}
	require.ErrorIs(t, svc.ConfirmRecovery(context.Background(), "done", "trustee"), ErrRecoveryRequestNotPending)

	// Approval path without federation service (notifyRecoveryApproved fallback).
	repo.requestsByID["req"] = &storage.SocialRecoveryRequest{
		ID:            "req",
		Username:      "alice",
		Status:        "pending",
		ExpiresAt:     time.Now().Add(time.Hour),
		RequiredVotes: 1,
		ReceivedVotes: 0,
		TrusteeVotes:  []string{},
		RecoveryToken: "tok",
	}
	require.NoError(t, svc.ConfirmRecovery(context.Background(), "req", "trustee-1"))
	require.Equal(t, "approved", repo.requestsByID["req"].Status)
	require.NotEmpty(t, repo.recoveryTokensByKey["RECOVERY#tok"])

	// enableRecoveryToken failure.
	repo2 := newInMemorySocialRecoveryRepo()
	repo2.requestsByID["req2"] = &storage.SocialRecoveryRequest{
		ID:            "req2",
		Username:      "alice",
		Status:        "pending",
		ExpiresAt:     time.Now().Add(time.Hour),
		RequiredVotes: 1,
		RecoveryToken: "tok2",
	}
	svc2 := &SocialRecoveryService{repo: &socialRecoveryRepoStoreTokenErr{inMemorySocialRecoveryRepo: repo2, err: errors.New("store token failed")}, logger: zap.NewNop()}
	require.ErrorIs(t, svc2.ConfirmRecovery(context.Background(), "req2", "trustee-1"), ErrRecoveryTokenStorage)

	// UpdateRecoveryRequest failure.
	repo3 := newInMemorySocialRecoveryRepo()
	repo3.requestsByID["req3"] = &storage.SocialRecoveryRequest{
		ID:            "req3",
		Username:      "alice",
		Status:        "pending",
		ExpiresAt:     time.Now().Add(time.Hour),
		RequiredVotes: 1,
		RecoveryToken: "tok3",
	}
	svc3 := &SocialRecoveryService{repo: &socialRecoveryRepoUpdateErr{inMemorySocialRecoveryRepo: repo3, err: errors.New("update failed")}, logger: zap.NewNop()}
	require.ErrorIs(t, svc3.ConfirmRecovery(context.Background(), "req3", "trustee-1"), ErrRecoveryRequestUpdate)
}
