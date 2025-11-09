package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/graph-gophers/dataloader"
	"go.uber.org/zap"
)

var (
	errLoadersNotFound              = errors.New("graph dataloaders not found in context")
	errQuoteTargetLoaderUnavailable = errors.New("quote target loader unavailable")
)

// Loaders holds all the dataloaders for the GraphQL server
type Loaders struct {
	ActorLoader       *dataloader.Loader
	ObjectLoader      *dataloader.Loader
	TrustScoreLoader  *dataloader.Loader
	QuoteTargetLoader *dataloader.Loader
}

// NewLoaders creates new instances of all dataloaders
func NewLoaders(repos core.RepositoryStorage, logger *zap.Logger) *Loaders {
	return &Loaders{
		ActorLoader:       newActorLoader(repos, logger),
		ObjectLoader:      newObjectLoader(repos, logger),
		TrustScoreLoader:  newTrustScoreLoader(repos, logger),
		QuoteTargetLoader: newQuoteTargetLoader(repos, logger),
	}
}

// Actor loader functions
func newActorLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		for i, key := range keys {
			username := key.String()
			actor, err := repos.Actor().GetActor(ctx, username)
			if err != nil {
				logger.Error("Failed to load actor", zap.String("username", username), zap.Error(err))
				results[i] = &dataloader.Result{Error: err}
			} else {
				results[i] = &dataloader.Result{Data: actor}
			}
		}

		return results
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

// Object loader functions
func newObjectLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		for i, key := range keys {
			objectID := key.String()
			// Get object from storage - this will need to handle different object types
			obj, err := repos.Object().GetObject(ctx, objectID)
			if err != nil {
				logger.Error("Failed to load object", zap.String("id", objectID), zap.Error(err))
				results[i] = &dataloader.Result{Error: err}
			} else {
				results[i] = &dataloader.Result{Data: obj}
			}
		}

		return results
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

// Trust score loader functions
func newTrustScoreLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		for i, key := range keys {
			// Key format: "actorID:category"
			keyParts := strings.Split(key.String(), ":")
			if len(keyParts) != 2 {
				results[i] = &dataloader.Result{Error: errors.Join(ErrInvalidTrustScoreKey, errors.New(key.String()))}
				continue
			}

			actorID, category := keyParts[0], keyParts[1]
			loadStart := time.Now()
			logger.Info("trust score loader fetching",
				zap.String("actorID", actorID),
				zap.String("category", category))

			// Get trust score from storage
			score, err := repos.Trust().GetTrustScore(ctx, actorID, category)
			if err != nil {
				logger.Error("Failed to load trust score",
					zap.String("actorID", actorID),
					zap.String("category", category),
					zap.Duration("duration", time.Since(loadStart)),
					zap.Error(err))
				results[i] = &dataloader.Result{Error: err}
			} else if score == nil {
				logger.Warn("Trust score loader returned nil score, using neutral default",
					zap.String("actorID", actorID),
					zap.String("category", category))
				results[i] = &dataloader.Result{Data: 0.5}
			} else {
				logger.Info("trust score loader completed",
					zap.String("actorID", actorID),
					zap.String("category", category),
					zap.Duration("duration", time.Since(loadStart)),
					zap.Float64("score", score.Score))
				results[i] = &dataloader.Result{Data: score.Score}
			}
		}

		return results
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

// Quote target loader batches quote target lookups so timelines with many quotes avoid N+1 lookups.
func newQuoteTargetLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		if repos.Status() == nil {
			for i := range results {
				results[i] = &dataloader.Result{Error: ErrStatusRepositoryUnavailable}
			}
			return results
		}

		statusIDs := make([]string, len(keys))
		for i, key := range keys {
			statusIDs[i] = key.String()
		}

		statuses, err := repos.Status().GetStatusesByIDs(ctx, statusIDs)
		if err != nil {
			for i := range results {
				results[i] = &dataloader.Result{Error: err}
			}
			return results
		}

		statusMap := make(map[string]*models.Status, len(statuses))
		for _, status := range statuses {
			if status == nil {
				continue
			}
			statusMap[status.StatusID] = status
		}

		for i, key := range keys {
			if status, ok := statusMap[key.String()]; ok {
				results[i] = &dataloader.Result{Data: status}
			} else {
				results[i] = &dataloader.Result{Data: (*models.Status)(nil)}
			}
		}

		return results
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

// Middleware to attach loaders to context
type contextKey string

const (
	loadersKey          contextKey = "dataloaders"
	quoteLoaderStatsKey contextKey = "quoteLoaderStats"
)

type quoteLoaderStats struct {
	hitCount  int64
	missCount int64
}

func recordQuoteLoaderHit(ctx context.Context) {
	if stats := getQuoteLoaderStats(ctx); stats != nil {
		atomic.AddInt64(&stats.hitCount, 1)
	}
}

func recordQuoteLoaderMiss(ctx context.Context) {
	if stats := getQuoteLoaderStats(ctx); stats != nil {
		atomic.AddInt64(&stats.missCount, 1)
	}
}

func getQuoteLoaderStats(ctx context.Context) *quoteLoaderStats {
	stats, _ := ctx.Value(quoteLoaderStatsKey).(*quoteLoaderStats)
	return stats
}

// QuoteLoaderMetrics exposes aggregated loader cache metrics for logging/monitoring.
func QuoteLoaderMetrics(ctx context.Context) (hits, misses int64) {
	if stats := getQuoteLoaderStats(ctx); stats != nil {
		hits = atomic.LoadInt64(&stats.hitCount)
		misses = atomic.LoadInt64(&stats.missCount)
	}
	return
}

// WithLoaders attaches loaders to the context
func WithLoaders(ctx context.Context, loaders *Loaders) context.Context {
	ctx = context.WithValue(ctx, loadersKey, loaders)
	return context.WithValue(ctx, quoteLoaderStatsKey, &quoteLoaderStats{})
}

// GetLoaders retrieves loaders from context
func GetLoaders(ctx context.Context) *Loaders {
	loaders, _ := ctx.Value(loadersKey).(*Loaders)
	return loaders
}

// LoadActor loads an actor using DataLoader
func LoadActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	loaders := GetLoaders(ctx)
	if loaders == nil || loaders.ActorLoader == nil {
		return nil, errLoadersNotFound
	}
	thunk := loaders.ActorLoader.Load(ctx, dataloader.StringKey(username))
	result, err := thunk()
	if err != nil {
		return nil, err
	}
	return result.(*activitypub.Actor), nil
}

// LoadObject loads an object using DataLoader
func LoadObject(ctx context.Context, id string) (any, error) {
	loaders := GetLoaders(ctx)
	if loaders == nil || loaders.ObjectLoader == nil {
		return nil, errLoadersNotFound
	}
	thunk := loaders.ObjectLoader.Load(ctx, dataloader.StringKey(id))
	result, err := thunk()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LoadTrustScore loads a trust score using DataLoader
func LoadTrustScore(ctx context.Context, actorID, category string) (any, error) {
	loaders := GetLoaders(ctx)
	if loaders == nil || loaders.TrustScoreLoader == nil {
		return nil, errLoadersNotFound
	}
	key := fmt.Sprintf("%s:%s", actorID, category)
	thunk := loaders.TrustScoreLoader.Load(ctx, dataloader.StringKey(key))
	result, err := thunk()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LoadQuoteTargetStatus loads a quote target status using the dedicated DataLoader
func LoadQuoteTargetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	loaders := GetLoaders(ctx)
	if loaders == nil {
		recordQuoteLoaderMiss(ctx)
		return nil, errLoadersNotFound
	}
	if loaders.QuoteTargetLoader == nil {
		recordQuoteLoaderMiss(ctx)
		return nil, errQuoteTargetLoaderUnavailable
	}

	thunk := loaders.QuoteTargetLoader.Load(ctx, dataloader.StringKey(statusID))
	result, err := thunk()
	if err != nil {
		recordQuoteLoaderMiss(ctx)
		return nil, err
	}
	recordQuoteLoaderHit(ctx)
	if result == nil {
		return nil, nil
	}
	status, _ := result.(*models.Status)
	return status, nil
}
