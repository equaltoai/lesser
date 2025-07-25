package graph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/graph-gophers/dataloader"
	"go.uber.org/zap"
)

// Loaders holds all the dataloaders for the GraphQL server
type Loaders struct {
	ActorLoader      *dataloader.Loader
	ObjectLoader     *dataloader.Loader
	TrustScoreLoader *dataloader.Loader
}

// NewLoaders creates new instances of all dataloaders
func NewLoaders(storage storage.Storage, logger *zap.Logger) *Loaders {
	return &Loaders{
		ActorLoader:      newActorLoader(storage, logger),
		ObjectLoader:     newObjectLoader(storage, logger),
		TrustScoreLoader: newTrustScoreLoader(storage, logger),
	}
}

// Actor loader functions
func newActorLoader(storage storage.Storage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		for i, key := range keys {
			username := key.String()
			actor, err := storage.GetActor(ctx, username)
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
func newObjectLoader(storage storage.Storage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		for i, key := range keys {
			objectID := key.String()
			// Get object from storage - this will need to handle different object types
			obj, err := storage.GetObject(ctx, objectID)
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
func newTrustScoreLoader(storage storage.Storage, logger *zap.Logger) *dataloader.Loader {
	batchFn := func(ctx context.Context, keys dataloader.Keys) []*dataloader.Result {
		results := make([]*dataloader.Result, len(keys))

		for i, key := range keys {
			// Key format: "actorID:category"
			keyParts := strings.Split(key.String(), ":")
			if len(keyParts) != 2 {
				results[i] = &dataloader.Result{Error: fmt.Errorf("invalid trust score key format: %s", key.String())}
				continue
			}

			actorID, category := keyParts[0], keyParts[1]
			// Get trust score from storage
			score, err := storage.GetTrustScore(ctx, actorID, category)
			if err != nil {
				logger.Error("Failed to load trust score",
					zap.String("actorID", actorID),
					zap.String("category", category),
					zap.Error(err))
				results[i] = &dataloader.Result{Error: err}
			} else {
				results[i] = &dataloader.Result{Data: score}
			}
		}

		return results
	}

	return dataloader.NewBatchedLoader(batchFn, dataloader.WithWait(2*time.Millisecond))
}

// Middleware to attach loaders to context
type contextKey string

const loadersKey contextKey = "dataloaders"

// WithLoaders attaches loaders to the context
func WithLoaders(ctx context.Context, loaders *Loaders) context.Context {
	return context.WithValue(ctx, loadersKey, loaders)
}

// GetLoaders retrieves loaders from context
func GetLoaders(ctx context.Context) *Loaders {
	loaders, ok := ctx.Value(loadersKey).(*Loaders)
	if !ok {
		panic("loaders not found in context")
	}
	return loaders
}

// LoadActor loads an actor using DataLoader
func LoadActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	loaders := GetLoaders(ctx)
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
	thunk := loaders.ObjectLoader.Load(ctx, dataloader.StringKey(id))
	result, err := thunk()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LoadTrustScore loads a trust score using DataLoader
func LoadTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	loaders := GetLoaders(ctx)
	key := fmt.Sprintf("%s:%s", actorID, category)
	thunk := loaders.TrustScoreLoader.Load(ctx, dataloader.StringKey(key))
	result, err := thunk()
	if err != nil {
		return nil, err
	}
	return result.(*storage.TrustScore), nil
}
