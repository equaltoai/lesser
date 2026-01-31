package lift

import (
	"context"

	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

type remoteSearchService interface {
	SearchRemoteActors(ctx context.Context, query string, limit int) ([]*federation.SearchResult, error)
}

type remoteSearchServiceFactory func(store core.RepositoryStorage) remoteSearchService

var defaultRemoteSearchServiceFactory remoteSearchServiceFactory = func(store core.RepositoryStorage) remoteSearchService {
	return federation.NewRemoteSearchService(store)
}
