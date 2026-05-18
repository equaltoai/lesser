// Package lambdastorage provides explicit product-level storage bootstrap for
// Lambda handlers that need TableTheory DB clients and repository storage.
package lambdastorage

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// NewDBFunc creates a Lambda-optimized TableTheory database client.
type NewDBFunc func(context.Context, string) (dynamormcore.DB, error)

// NewRepositoryStorageFunc creates repository storage over a TableTheory DB.
type NewRepositoryStorageFunc func(dynamormcore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error)

// Options controls explicit Lambda storage bootstrap.
type Options struct {
	// ServiceName is used in validation errors to make cold-start failures
	// operator-readable without relying on ad-hoc processor text.
	ServiceName string

	// TableName overrides Config.DynamoTableName when a Lambda stores data in a
	// non-main table. Empty uses Config.DynamoTableName.
	TableName string

	// RequireRepositories initializes and validates RepositoryStorage in addition
	// to the TableTheory DB client.
	RequireRepositories bool

	// AllowEmptyRegion permits custom NewDB hooks that deliberately resolve AWS
	// region from their own environment/config path.
	AllowEmptyRegion bool

	// NewDB and NewRepositoryStorage are injectable for tests. Production callers
	// should leave them nil unless they already expose package-local hooks.
	NewDB                NewDBFunc
	NewRepositoryStorage NewRepositoryStorageFunc
}

// Dependencies are the typed storage dependencies produced by Initialize.
type Dependencies struct {
	Config    *config.Config
	Logger    *zap.Logger
	DB        dynamormcore.DB
	Repos     storagecore.RepositoryStorage
	TableName string
}

var runningUnitTestsFn = common.RunningUnitTests

// Initialize ensures a LambdaContext has typed storage dependencies and writes
// them back to LambdaContext for compatibility with existing handlers.
func Initialize(ctx context.Context, lambdaCtx *common.LambdaContext, opts Options) (*Dependencies, error) {
	serviceName := normalizeServiceName(opts.ServiceName)
	if ctx == nil {
		ctx = context.Background()
	}
	if lambdaCtx == nil {
		return nil, fmt.Errorf("%s storage bootstrap: lambda context is nil", serviceName)
	}
	if lambdaCtx.Config == nil {
		return nil, fmt.Errorf("%s storage bootstrap: config is nil", serviceName)
	}

	logger := ensureLogger(lambdaCtx)
	if deps, handled, err := existingRepositoryDependencies(lambdaCtx, opts, serviceName, logger); handled || err != nil {
		return deps, err
	}

	tableName, err := tableNameFromConfig(lambdaCtx, opts, serviceName)
	if err != nil {
		return nil, err
	}

	if existingDB, ok := lambdaCtx.DynamoDB.(dynamormcore.DB); ok && existingDB != nil {
		deps := &Dependencies{
			Config:    lambdaCtx.Config,
			Logger:    logger,
			DB:        existingDB,
			TableName: tableName,
		}
		if opts.RequireRepositories {
			repos, err := resolveRepositoryStorage(lambdaCtx, opts, serviceName, existingDB, tableName, logger)
			if err != nil {
				return nil, err
			}
			deps.Repos = repos
		}
		return deps, nil
	}

	region, err := resolveRegion(lambdaCtx, opts, serviceName)
	if err != nil {
		return nil, err
	}

	db, err := resolveDB(ctx, lambdaCtx, opts, serviceName, region)
	if err != nil {
		return nil, err
	}

	deps := &Dependencies{
		Config:    lambdaCtx.Config,
		Logger:    logger,
		DB:        db,
		TableName: tableName,
	}

	if opts.RequireRepositories {
		repos, err := resolveRepositoryStorage(lambdaCtx, opts, serviceName, db, tableName, logger)
		if err != nil {
			return nil, err
		}
		deps.Repos = repos
	}

	return deps, nil
}

func normalizeServiceName(serviceName string) string {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return "lambda"
	}
	return serviceName
}

func ensureLogger(lambdaCtx *common.LambdaContext) *zap.Logger {
	if lambdaCtx.Logger != nil {
		return lambdaCtx.Logger
	}
	logger := zap.NewNop()
	lambdaCtx.Logger = logger
	return logger
}

func existingRepositoryDependencies(lambdaCtx *common.LambdaContext, opts Options, serviceName string, logger *zap.Logger) (*Dependencies, bool, error) {
	if !opts.RequireRepositories || lambdaCtx.Repos == nil {
		return nil, false, nil
	}

	repos, ok := lambdaCtx.Repos.(storagecore.RepositoryStorage)
	if !ok || repos == nil {
		return nil, true, fmt.Errorf("%s storage bootstrap: invalid repository storage", serviceName)
	}

	db := safeRepositoryDB(repos)
	if db == nil {
		if existingDB, ok := lambdaCtx.DynamoDB.(dynamormcore.DB); ok && existingDB != nil {
			db = existingDB
		}
	}

	if db != nil {
		tableName, err := tableNameWithRepositoryFallback(lambdaCtx, opts, serviceName, repos)
		if err != nil {
			return nil, true, err
		}
		lambdaCtx.DynamoDB = db
		lambdaCtx.Repos = repos
		return &Dependencies{
			Config:    lambdaCtx.Config,
			Logger:    logger,
			DB:        db,
			Repos:     repos,
			TableName: tableName,
		}, true, nil
	}

	if !runningUnitTestsFn() {
		return nil, false, nil
	}

	tableName := strings.TrimSpace(opts.TableName)
	if tableName == "" {
		tableName = strings.TrimSpace(safeRepositoryTableName(repos))
	}
	if tableName == "" {
		tableName = strings.TrimSpace(lambdaCtx.Config.DynamoTableName)
	}
	lambdaCtx.Repos = repos
	return &Dependencies{
		Config:    lambdaCtx.Config,
		Logger:    logger,
		Repos:     repos,
		TableName: tableName,
	}, true, nil
}

func tableNameWithRepositoryFallback(lambdaCtx *common.LambdaContext, opts Options, serviceName string, repos storagecore.RepositoryStorage) (string, error) {
	tableName := strings.TrimSpace(opts.TableName)
	if tableName == "" {
		tableName = strings.TrimSpace(safeRepositoryTableName(repos))
	}
	if tableName == "" {
		tableName = strings.TrimSpace(lambdaCtx.Config.DynamoTableName)
	}
	if tableName == "" {
		return "", fmt.Errorf("%s storage bootstrap: dynamodb table name is required", serviceName)
	}
	return tableName, nil
}

func tableNameFromConfig(lambdaCtx *common.LambdaContext, opts Options, serviceName string) (string, error) {
	tableName := strings.TrimSpace(opts.TableName)
	if tableName == "" {
		tableName = strings.TrimSpace(lambdaCtx.Config.DynamoTableName)
	}
	if tableName == "" {
		return "", fmt.Errorf("%s storage bootstrap: dynamodb table name is required", serviceName)
	}
	return tableName, nil
}

func resolveRegion(lambdaCtx *common.LambdaContext, opts Options, serviceName string) (string, error) {
	region := strings.TrimSpace(lambdaCtx.Config.Region)
	if region == "" && lambdaCtx.AWSServices != nil {
		region = strings.TrimSpace(lambdaCtx.AWSServices.Config.Region)
		if region != "" {
			lambdaCtx.Config.Region = region
		}
	}
	if region == "" && !opts.AllowEmptyRegion {
		return "", fmt.Errorf("%s storage bootstrap: AWS region is required", serviceName)
	}
	return region, nil
}

func resolveDB(ctx context.Context, lambdaCtx *common.LambdaContext, opts Options, serviceName string, region string) (dynamormcore.DB, error) {
	if lambdaCtx.DynamoDB != nil {
		db, ok := lambdaCtx.DynamoDB.(dynamormcore.DB)
		if !ok || db == nil {
			return nil, fmt.Errorf("%s storage bootstrap: invalid dynamodb client", serviceName)
		}
		return db, nil
	}

	newDB := opts.NewDB
	if newDB == nil {
		newDB = theorydb.NewLambdaOptimizedClient
	}

	db, err := newDB(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("%s storage bootstrap: storage client initialization failed: %w", serviceName, err)
	}
	if db == nil {
		return nil, fmt.Errorf("%s storage bootstrap: storage client initialization returned nil", serviceName)
	}
	lambdaCtx.DynamoDB = db
	return db, nil
}

func resolveRepositoryStorage(lambdaCtx *common.LambdaContext, opts Options, serviceName string, db dynamormcore.DB, tableName string, logger *zap.Logger) (storagecore.RepositoryStorage, error) {
	if lambdaCtx.Repos != nil {
		repos, ok := lambdaCtx.Repos.(storagecore.RepositoryStorage)
		if !ok || repos == nil {
			return nil, fmt.Errorf("%s storage bootstrap: invalid repository storage", serviceName)
		}
		if safeRepositoryDB(repos) == nil && !runningUnitTestsFn() {
			return nil, fmt.Errorf("%s storage bootstrap: existing repository storage has no dynamodb client", serviceName)
		}
		return repos, nil
	}

	newRepos := opts.NewRepositoryStorage
	if newRepos == nil {
		newRepos = func(db dynamormcore.DB, tableName string, logger *zap.Logger) (storagecore.RepositoryStorage, error) {
			return factory.NewRepositoryFactory(db, tableName, logger)
		}
	}

	repos, err := newRepos(db, tableName, logger)
	if err != nil {
		return nil, fmt.Errorf("%s storage bootstrap: repository initialization failed: %w", serviceName, err)
	}
	if repos == nil {
		return nil, fmt.Errorf("%s storage bootstrap: repository initialization returned nil", serviceName)
	}
	lambdaCtx.Repos = repos
	return repos, nil
}

func safeRepositoryDB(repos storagecore.RepositoryStorage) (db dynamormcore.DB) {
	defer func() {
		if recover() != nil {
			db = nil
		}
	}()
	return repos.GetDB()
}

func safeRepositoryTableName(repos storagecore.RepositoryStorage) (tableName string) {
	defer func() {
		if recover() != nil {
			tableName = ""
		}
	}()
	return repos.GetTableName()
}
