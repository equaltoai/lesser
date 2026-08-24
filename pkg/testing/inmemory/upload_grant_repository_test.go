package inmemory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestUploadGrantRepositoryConcurrentConsumeExactlyOneWins(t *testing.T) {
	repo := NewUploadGrantRepository()
	now := time.Now().UTC()
	grant := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-race",
		Status: models.UploadGrantStatusMinted, GrantedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, repo.CreateUploadGrant(context.Background(), grant))

	const racers = 8
	var wg sync.WaitGroup
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each racer reads its own copy at the observed version.
			observed, err := repo.GetUploadGrant(context.Background(), "alice", "grant-race")
			if err != nil {
				results <- err
				return
			}
			results <- repo.ConsumeUploadGrant(context.Background(), observed, models.UploadGrantStatusUsed, "", time.Now().UTC())
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	consumedConflicts := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, interfaces.ErrUploadGrantConsumed):
			consumedConflicts++
		default:
			t.Fatalf("unexpected consume error: %v", err)
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent finalize may win")
	require.Equal(t, racers-1, consumedConflicts, "every loser must fail closed with the consumed error")

	stored, err := repo.GetUploadGrant(context.Background(), "alice", "grant-race")
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, stored.Status)
	require.Equal(t, 1, stored.Version, "version bumps exactly once across all racers")
}

func TestUploadGrantRepositoryRejectsConsumeOfNonMintedGrant(t *testing.T) {
	repo := NewUploadGrantRepository()
	now := time.Now().UTC()
	grant := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-used",
		Status: models.UploadGrantStatusUsed, GrantedAt: now, ExpiresAt: now.Add(time.Hour), Version: 1,
	}
	repo.SeedUploadGrant(grant)

	observed, err := repo.GetUploadGrant(context.Background(), "alice", "grant-used")
	require.NoError(t, err)
	err = repo.ConsumeUploadGrant(context.Background(), observed, models.UploadGrantStatusFailedDigest, "late mismatch", now)
	require.ErrorIs(t, err, interfaces.ErrUploadGrantConsumed, "a consumed grant cannot be re-transitioned")
}
