package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/graph-gophers/dataloader"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round12TrustNilStorage struct {
	core.RepositoryStorage
}

func (s round12TrustNilStorage) Trust() interfaces.TrustRepository {
	return round12TrustRepoNil{}
}

type round12TrustRepoNil struct{}

func (round12TrustRepoNil) CreateTrustRelationship(context.Context, *storage.TrustRelationship) error {
	return nil
}
func (round12TrustRepoNil) GetTrustRelationship(context.Context, string, string, string) (*storage.TrustRelationship, error) {
	return nil, storage.ErrNotFound
}
func (round12TrustRepoNil) UpdateTrustRelationship(context.Context, *storage.TrustRelationship) error { return nil }
func (round12TrustRepoNil) DeleteTrustRelationship(context.Context, string, string, string) error     { return nil }
func (round12TrustRepoNil) GetTrustRelationships(context.Context, string, int, string) ([]*storage.TrustRelationship, string, error) {
	return []*storage.TrustRelationship{}, "", nil
}
func (round12TrustRepoNil) GetTrustedByRelationships(context.Context, string, int, string) ([]*storage.TrustRelationship, string, error) {
	return []*storage.TrustRelationship{}, "", nil
}
func (round12TrustRepoNil) GetAllTrustRelationships(context.Context, int) ([]*storage.TrustRelationship, error) {
	return []*storage.TrustRelationship{}, nil
}
func (round12TrustRepoNil) GetTrustScore(context.Context, string, string) (*storage.TrustScore, error) {
	return nil, nil
}
func (round12TrustRepoNil) UpdateTrustScore(context.Context, *storage.TrustScore) error { return nil }
func (round12TrustRepoNil) GetUserTrustScore(context.Context, string) (float64, error)  { return 0.5, nil }
func (round12TrustRepoNil) RecordTrustUpdate(context.Context, *storage.TrustUpdate) error {
	return nil
}

func TestRound12Dataloader_NewLoadersAndHelpers(t *testing.T) {
	_, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()

	actorRepo := storageRepo.Actor()
	require.NotNil(t, actorRepo)
	require.NoError(t, actorRepo.CreateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   config.Get().ActorURL("alice"),
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}, ""))

	objectRepo := storageRepo.Object()
	require.NotNil(t, objectRepo)
	require.NoError(t, objectRepo.CreateObject(ctx, map[string]any{
		"id":           "obj-1",
		"type":         "Note",
		"attributedTo": config.Get().ActorURL("alice"),
	}))

	trustRepo := storageRepo.Trust()
	require.NotNil(t, trustRepo)
	require.NoError(t, trustRepo.UpdateTrustScore(ctx, &storage.TrustScore{
		ActorID:   "alice",
		Category:  storageModels.TrustCategoryContent,
		Score:     0.9,
		CacheTTL:  time.Now().Add(time.Hour),
	}))

	statusRepo := storageRepo.Status()
	require.NotNil(t, statusRepo)
	require.NoError(t, statusRepo.CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-1",
		AuthorID:       config.Get().ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "hello",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Minute),
		Visibility:     storageModels.VisibilityPublic,
	}))

	loaders := NewLoaders(storageRepo, zap.NewNop())
	ctxWith := WithLoaders(ctx, loaders)

	actor, err := LoadActor(ctxWith, "alice")
	require.NoError(t, err)
	require.NotNil(t, actor)

	obj, err := LoadObject(ctxWith, "obj-1")
	require.NoError(t, err)
	require.NotNil(t, obj)

	score, err := LoadTrustScore(ctxWith, "alice", string(trust.TrustCategoryContent))
	require.NoError(t, err)
	require.Equal(t, 0.9, score)

	target, err := LoadQuoteTargetStatus(ctxWith, "status-1")
	require.NoError(t, err)
	require.NotNil(t, target)

	hits, misses := QuoteLoaderMetrics(ctxWith)
	require.Equal(t, int64(1), hits)
	require.Equal(t, int64(0), misses)

	_, err = LoadQuoteTargetStatus(ctxWith, "missing-status")
	require.NoError(t, err)

	hits, misses = QuoteLoaderMetrics(ctxWith)
	require.Equal(t, int64(2), hits)
	require.Equal(t, int64(0), misses)
}

func TestRound12Dataloader_MissingLoadersAndMetrics(t *testing.T) {
	ctx := context.Background()

	_, err := LoadActor(ctx, "alice")
	require.Error(t, err)

	hits, misses := QuoteLoaderMetrics(ctx)
	require.Equal(t, int64(0), hits)
	require.Equal(t, int64(0), misses)

	ctxWithStats := WithLoaders(ctx, &Loaders{})
	_, err = LoadQuoteTargetStatus(ctxWithStats, "status-1")
	require.Error(t, err)

	hits, misses = QuoteLoaderMetrics(ctxWithStats)
	require.Equal(t, int64(0), hits)
	require.Equal(t, int64(1), misses)
}

func TestRound12Dataloader_TrustScoreNilFallsBackToNeutral(t *testing.T) {
	_, storageRepo := newRound12GraphResolver(t)
	ctx := WithLoaders(context.Background(), NewLoaders(round12TrustNilStorage{RepositoryStorage: storageRepo}, zap.NewNop()))

	thunk := GetLoaders(ctx).TrustScoreLoader.Load(ctx, dataloader.StringKey("alice:content"))
	value, err := thunk()
	require.NoError(t, err)
	require.Equal(t, 0.5, value)

	thunk = GetLoaders(ctx).TrustScoreLoader.Load(ctx, dataloader.StringKey("invalid-key"))
	_, err = thunk()
	require.Error(t, err)
}
