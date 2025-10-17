# Incomplete Implementations Report

_Generated on Fri Oct 17 04:32:50 PM EDT 2025_

## "not implemented" occurrences (1)
./pkg/testing/mocks/storage_mock.go:4145:	// Create repository instances with nil DB - they'll return "not implemented" errors

## TODO comments (0)
_None found._

## context.TODO() occurrences (0)
_None found._

## Authentication repository gaps (0)
_None found._

## Pagination TODO markers (2)
pkg/storage/repositories/account_repository_oauth.go:411:// ListOAuthClients lists OAuth clients with deterministic cursor-based pagination.
pkg/storage/repositories/base_repository.go:723:	UseCursor bool   // Enables cursor-based pagination on the configured sort key

## GraphQL TODOs (0)
_None found._

## "return nil, nil" patterns (13)
pkg/storage/repositories/actor_repository.go:151:			return nil, nil, common.ActorNotFoundError{Username: username}
pkg/storage/repositories/actor_repository.go:153:		return nil, nil, ErrorHandler.HandleGetError(err, EntityActor, username)
pkg/storage/repositories/federation_instance_repository.go:759:		return nil, nil, err
pkg/storage/repositories/search_repository.go:1050:		return nil, nil, ErrorHandler.HandleQueryError(err, "hashtag search", "pagination validation")
pkg/storage/repositories/search_repository.go:1063:		return nil, nil, ErrorHandler.HandleQueryError(err, "hashtag search", "cursor decoding")
pkg/storage/repositories/search_repository.go:1085:		return nil, nil, ErrorHandler.HandleQueryError(err, "hashtag search", "advanced search")
pkg/storage/repositories/search_repository.go:1878:		return nil, nil, err
pkg/storage/repositories/search_repository.go:1885:		return nil, nil, ErrorHandler.HandleQueryError(err, "embedding search", "cursor decoding")
pkg/storage/repositories/search_repository.go:436:		return nil, nil, ErrorHandler.HandleQueryError(err, "search", "pagination validation")
pkg/storage/repositories/search_repository.go:505:		return nil, nil, err
pkg/storage/repositories/search_repository.go:890:		return nil, nil, ErrorHandler.HandleQueryError(err, "search all", "pagination validation")
pkg/storage/repositories/status_repository.go:1005:		return nil, nil, ErrorHandler.HandleGetError(err, EntityStatus, statusID)
pkg/storage/repositories/user_repository.go:2970:		return nil, nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "activity object", fmt.Sprintf("type %T", activity.Object))

