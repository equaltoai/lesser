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

	// Viewer interaction loaders (GraphQL fields on Object).
	ViewerFavouritedLoader *dataloader.Loader
	ViewerBookmarkedLoader *dataloader.Loader
	ViewerPinnedLoader     *dataloader.Loader
}

// NewLoaders creates new instances of all dataloaders
func NewLoaders(repos core.RepositoryStorage, logger *zap.Logger) *Loaders {
	return &Loaders{
		ActorLoader:            newActorLoader(repos, logger),
		ObjectLoader:           newObjectLoader(repos, logger),
		TrustScoreLoader:       newTrustScoreLoader(repos, logger),
		QuoteTargetLoader:      newQuoteTargetLoader(repos, logger),
		ViewerFavouritedLoader: newViewerFavouritedLoader(repos, logger),
		ViewerBookmarkedLoader: newViewerBookmarkedLoader(repos, logger),
		ViewerPinnedLoader:     newViewerPinnedLoader(repos, logger),
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

type viewerStatusFlagLookup func(ctx context.Context, viewerUsername string, statusIDs []string) (map[string]bool, error)

func newViewerStatusFlagLoader(logger *zap.Logger, lookup viewerStatusFlagLookup, warnMsg string) *dataloader.Loader {
	logWarn := func(msg string, fields ...zap.Field) {
		if logger != nil {
			logger.Warn(msg, fields...)
		}
	}

	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		viewerUsername := getUsernameFromContext(ctx)
		if viewerUsername == "" || lookup == nil {
			for i := range results {
				results[i] = &dataloader.Result{Data: false}
			}
			return results
		}

		statusIDs := make([]string, len(keys))
		for i, key := range keys {
			statusIDs[i] = key.String()
		}

		flags, err := lookup(ctx, viewerUsername, statusIDs)
		if err != nil {
			logWarn(warnMsg,
				zap.String("viewer", viewerUsername),
				zap.Int("requested", len(statusIDs)),
				zap.Error(err))
			for i := range results {
				results[i] = &dataloader.Result{Data: false}
			}
			return results
		}

		for i, key := range keys {
			results[i] = &dataloader.Result{Data: flags[key.String()]}
		}
		return results
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

type viewerFavouritedLookup func(ctx context.Context, actorID string, objectIDs []string) (map[string]bool, error)

type viewerFavouritedParsedKey struct {
	actorID  string
	objectID string
}

func parseViewerFavouritedKeys(keys dataloader.Keys) ([]viewerFavouritedParsedKey, map[string][]string) {
	parsed := make([]viewerFavouritedParsedKey, len(keys))
	byActor := make(map[string][]string)
	for i, key := range keys {
		parts := strings.SplitN(key.String(), "\n", 2)
		if len(parts) != 2 {
			continue
		}

		actorID := strings.TrimSpace(parts[0])
		objectID := strings.TrimSpace(parts[1])
		parsed[i] = viewerFavouritedParsedKey{actorID: actorID, objectID: objectID}
		if actorID == "" || objectID == "" {
			continue
		}
		byActor[actorID] = append(byActor[actorID], objectID)
	}

	return parsed, byActor
}

func fetchViewerFavouritedByActor(ctx context.Context, byActor map[string][]string, lookup viewerFavouritedLookup, logger *zap.Logger) map[string]map[string]bool {
	logWarn := func(msg string, fields ...zap.Field) {
		if logger != nil {
			logger.Warn(msg, fields...)
		}
	}

	likedByActor := make(map[string]map[string]bool, len(byActor))
	for actorID, objectIDs := range byActor {
		liked, err := lookup(ctx, actorID, objectIDs)
		if err != nil {
			logWarn("viewer favourited loader batch lookup failed",
				zap.String("actor_id", actorID),
				zap.Int("requested", len(objectIDs)),
				zap.Error(err))
			likedByActor[actorID] = map[string]bool{}
			continue
		}
		likedByActor[actorID] = liked
	}

	return likedByActor
}

func viewerFavouritedBatch(ctx context.Context, keys dataloader.Keys, lookup viewerFavouritedLookup, logger *zap.Logger) []*dataloader.Result {
	results := make([]*dataloader.Result, len(keys))
	if lookup == nil {
		for i := range results {
			results[i] = &dataloader.Result{Data: false}
		}
		return results
	}

	parsed, byActor := parseViewerFavouritedKeys(keys)
	likedByActor := fetchViewerFavouritedByActor(ctx, byActor, lookup, logger)

	for i, p := range parsed {
		results[i] = &dataloader.Result{Data: likedByActor[p.actorID][p.objectID]}
	}

	return results
}

func newViewerFavouritedLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	var lookup viewerFavouritedLookup
	if repos != nil {
		if likeRepo := repos.Like(); likeRepo != nil {
			lookup = likeRepo.CheckLikesForStatuses
		}
	}

	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		return viewerFavouritedBatch(ctx, keys, lookup, logger)
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

func newViewerBookmarkedLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	var lookup viewerStatusFlagLookup
	if repos != nil {
		if bookmarkRepo := repos.Bookmark(); bookmarkRepo != nil {
			lookup = bookmarkRepo.CheckBookmarksForStatuses
		}
	}
	return newViewerStatusFlagLoader(logger, lookup, "viewer bookmarked loader batch lookup failed")
}

func newViewerPinnedLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	var lookup viewerStatusFlagLookup
	if repos != nil {
		if socialRepo := repos.Social(); socialRepo != nil {
			lookup = socialRepo.CheckPinnedStatuses
		}
	}
	return newViewerStatusFlagLoader(logger, lookup, "viewer pinned loader batch lookup failed")
}

// Quote target loader batches quote target lookups so timelines with many quotes avoid N+1 lookups.
func newQuoteTargetLoader(repos core.RepositoryStorage, logger *zap.Logger) *dataloader.Loader {
	logDebug := func(msg string, fields ...zap.Field) {
		if logger != nil {
			logger.Debug(msg, fields...)
		}
	}
	logError := func(msg string, fields ...zap.Field) {
		if logger != nil {
			logger.Error(msg, fields...)
		}
	}

	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		if repos.Status() == nil {
			logError("quote target loader unavailable: status repository missing",
				zap.Int("requested_keys", len(keys)))
			for i := range results {
				results[i] = &dataloader.Result{Error: ErrStatusRepositoryUnavailable}
			}
			return results
		}

		statusIDs := make([]string, len(keys))
		for i, key := range keys {
			statusIDs[i] = key.String()
		}

		logDebug("quote target loader fetching statuses",
			zap.Int("requested_keys", len(statusIDs)))

		statuses, err := repos.Status().GetStatusesByIDs(ctx, statusIDs)
		if err != nil {
			logError("quote target loader failed to fetch statuses",
				zap.Int("requested_keys", len(statusIDs)),
				zap.Error(err))
			for i := range results {
				results[i] = &dataloader.Result{Error: err}
			}
			return results
		}

		logDebug("quote target loader resolved statuses",
			zap.Int("requested_keys", len(statusIDs)),
			zap.Int("resolved_statuses", len(statuses)))

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
