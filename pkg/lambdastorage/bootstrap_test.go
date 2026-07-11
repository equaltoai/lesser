package lambdastorage

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

type stubRepositoryStorage struct {
	storagecore.RepositoryStorage
	db        dynamormcore.DB
	tableName string
}

func (s *stubRepositoryStorage) GetDB() dynamormcore.DB { return s.db }
func (s *stubRepositoryStorage) GetTableName() string   { return s.tableName }
func (s *stubRepositoryStorage) GetLogger() *zap.Logger { return zap.NewNop() }

type panicRepositoryStorage struct {
	storagecore.RepositoryStorage
}

func (p *panicRepositoryStorage) GetDB() dynamormcore.DB { panic("db unavailable") }
func (p *panicRepositoryStorage) GetTableName() string   { panic("table unavailable") }
func (p *panicRepositoryStorage) GetLogger() *zap.Logger { return zap.NewNop() }

func TestInitializeRequiresContextConfigAndTable(t *testing.T) {
	_, err := Initialize(context.Background(), nil, Options{})
	require.ErrorContains(t, err, "lambda storage bootstrap: lambda context is nil")

	_, err = Initialize(context.Background(), &common.LambdaContext{}, Options{ServiceName: "unit"})
	require.ErrorContains(t, err, "config is nil")

	_, err = Initialize(context.Background(), &common.LambdaContext{Config: &config.Config{Region: "us-east-1"}}, Options{ServiceName: "unit"})
	require.ErrorContains(t, err, "dynamodb table name is required")

	_, err = Initialize(context.Background(), &common.LambdaContext{Config: &config.Config{DynamoTableName: "lesser-test"}}, Options{ServiceName: "unit"})
	require.ErrorContains(t, err, "AWS region is required")
}

func TestInitializeCreatesDBAndRepositoryStorage(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	repos := &stubRepositoryStorage{db: db, tableName: "lesser-test"}
	ctx := &common.LambdaContext{
		Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		Logger: zap.NewNop(),
	}

	deps, err := Initialize(nil, ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			return db, nil
		},
		NewRepositoryStorage: func(gotDB dynamormcore.DB, tableName string, logger *zap.Logger) (storagecore.RepositoryStorage, error) {
			require.Same(t, db, gotDB)
			require.Equal(t, "lesser-test", tableName)
			require.NotNil(t, logger)
			return repos, nil
		},
	})
	require.NoError(t, err)
	require.Same(t, db, deps.DB)
	require.Same(t, repos, deps.Repos)
	require.Same(t, db, ctx.DynamoDB)
	require.Same(t, repos, ctx.Repos)

	ctx = &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		DynamoDB: db,
	}
	_, err = Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewRepositoryStorage: func(dynamormcore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error) {
			return nil, errors.New("repo boom")
		},
	})
	require.ErrorContains(t, err, "repository initialization failed")
}

func TestInitializeReusesExistingTypedDependencies(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	repos := &stubRepositoryStorage{db: db, tableName: "lesser-test"}
	ctx := &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		DynamoDB: db,
		Repos:    repos,
	}

	deps, err := Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			t.Fatal("NewDB should not be called when a typed DB already exists")
			return nil, nil
		},
	})
	require.NoError(t, err)
	require.Same(t, db, deps.DB)
	require.Same(t, repos, deps.Repos)
}

func TestInitializeValidatesExistingRepositories(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	ctx := &common.LambdaContext{
		Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		Repos:  struct{}{},
	}
	_, err := Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
	})
	require.ErrorContains(t, err, "invalid repository storage")

	ctx = &common.LambdaContext{
		Config:   &config.Config{Region: "us-east-1"},
		DynamoDB: db,
		Repos:    &stubRepositoryStorage{db: db},
	}
	_, err = Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
	})
	require.ErrorContains(t, err, "dynamodb table name is required")
}

func TestInitializeHandlesRepositoryIntrospectionPanics(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	repos := &panicRepositoryStorage{}
	ctx := &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		DynamoDB: db,
		Repos:    repos,
	}

	deps, err := Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
	})

	require.NoError(t, err)
	require.Same(t, db, deps.DB)
	require.Same(t, repos, deps.Repos)
	require.Equal(t, "lesser-test", deps.TableName)
}

func TestInitializeUsesExistingDBAndCreatesRepositories(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	repos := &stubRepositoryStorage{db: db, tableName: "lesser-test"}
	ctx := &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		DynamoDB: db,
	}

	deps, err := Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewRepositoryStorage: func(gotDB dynamormcore.DB, tableName string, logger *zap.Logger) (storagecore.RepositoryStorage, error) {
			require.Same(t, db, gotDB)
			require.Equal(t, "lesser-test", tableName)
			require.NotNil(t, logger)
			return repos, nil
		},
	})

	require.NoError(t, err)
	require.Same(t, db, deps.DB)
	require.Same(t, repos, deps.Repos)
	require.Same(t, repos, ctx.Repos)
}

func TestInitializeUsesAWSServiceRegionAndTableOverride(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	ctx := &common.LambdaContext{
		Config:      &config.Config{DynamoTableName: "ignored"},
		AWSServices: &awsinit.AWSServices{Config: aws.Config{Region: "us-west-2"}},
	}

	deps, err := Initialize(context.Background(), ctx, Options{
		ServiceName: "unit-processor",
		TableName:   "lesser-override",
		NewDB: func(_ context.Context, region string) (dynamormcore.DB, error) {
			require.Equal(t, "us-west-2", region)
			return db, nil
		},
	})

	require.NoError(t, err)
	require.Same(t, db, deps.DB)
	require.Equal(t, "lesser-override", deps.TableName)
	require.Equal(t, "us-west-2", ctx.Config.Region)
}

func TestInitializeValidatesExistingDBAndRepositoryResults(t *testing.T) {
	ctx := &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		DynamoDB: "not-a-db",
	}
	_, err := Initialize(context.Background(), ctx, Options{ServiceName: "unit-processor"})
	require.ErrorContains(t, err, "invalid dynamodb client")

	db := &tabletheory.LambdaDB{}
	ctx = &common.LambdaContext{Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"}}
	_, err = Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			return db, nil
		},
		NewRepositoryStorage: func(dynamormcore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error) {
			return nil, errors.New("repo boom")
		},
	})
	require.ErrorContains(t, err, "repository initialization failed")

	ctx = &common.LambdaContext{Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"}}
	_, err = Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			return db, nil
		},
		NewRepositoryStorage: func(dynamormcore.DB, string, *zap.Logger) (storagecore.RepositoryStorage, error) {
			return nil, nil
		},
	})
	require.ErrorContains(t, err, "repository initialization returned nil")
}

func TestResolveRepositoryStorageExistingBranches(t *testing.T) {
	db := &tabletheory.LambdaDB{}
	ctx := &common.LambdaContext{Repos: struct{}{}}
	_, err := resolveRepositoryStorage(ctx, Options{}, "unit-processor", db, "lesser-test", zap.NewNop())
	require.ErrorContains(t, err, "invalid repository storage")

	origRunningUnitTests := runningUnitTestsFn
	runningUnitTestsFn = func() bool { return false }
	t.Cleanup(func() { runningUnitTestsFn = origRunningUnitTests })

	repos := &stubRepositoryStorage{tableName: "lesser-test"}
	ctx = &common.LambdaContext{Repos: repos}
	_, err = resolveRepositoryStorage(ctx, Options{}, "unit-processor", db, "lesser-test", zap.NewNop())
	require.ErrorContains(t, err, "existing repository storage has no dynamodb client")

	runningUnitTestsFn = func() bool { return true }
	got, err := resolveRepositoryStorage(ctx, Options{}, "unit-processor", db, "lesser-test", zap.NewNop())
	require.NoError(t, err)
	require.Same(t, repos, got)
}

func TestInitializeRejectsExistingRepositoryWithoutDBOutsideTests(t *testing.T) {
	origRunningUnitTests := runningUnitTestsFn
	runningUnitTestsFn = func() bool { return false }
	t.Cleanup(func() { runningUnitTestsFn = origRunningUnitTests })

	db := &tabletheory.LambdaDB{}
	ctx := &common.LambdaContext{
		Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		Repos:  &stubRepositoryStorage{tableName: "lesser-test"},
	}

	_, err := Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			return db, nil
		},
	})
	require.ErrorContains(t, err, "existing repository storage has no dynamodb client")
}

func TestInitializeAllowsTestRepositoryWithoutDB(t *testing.T) {
	origRunningUnitTests := runningUnitTestsFn
	runningUnitTestsFn = func() bool { return true }
	t.Cleanup(func() { runningUnitTestsFn = origRunningUnitTests })

	repos := &stubRepositoryStorage{tableName: "lesser-test"}
	ctx := &common.LambdaContext{
		Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"},
		Repos:  repos,
	}

	deps, err := Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			t.Fatal("NewDB should not be called for unit-test repository compatibility")
			return nil, nil
		},
	})

	require.NoError(t, err)
	require.Nil(t, deps.DB)
	require.Same(t, repos, deps.Repos)
	require.Equal(t, "lesser-test", deps.TableName)

	ctx = &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1"},
		Repos:  &stubRepositoryStorage{},
	}
	deps, err = Initialize(context.Background(), ctx, Options{
		ServiceName:         "unit-processor",
		RequireRepositories: true,
	})
	require.NoError(t, err)
	require.Equal(t, "", deps.TableName)
}

func TestInitializePropagatesCreationErrors(t *testing.T) {
	ctx := &common.LambdaContext{Config: &config.Config{DynamoTableName: "lesser-test", Region: "us-east-1"}}
	_, err := Initialize(context.Background(), ctx, Options{
		ServiceName: "unit-processor",
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			return nil, errors.New("boom")
		},
	})
	require.ErrorContains(t, err, "storage client initialization failed")

	_, err = Initialize(context.Background(), ctx, Options{
		ServiceName: "unit-processor",
		NewDB: func(context.Context, string) (dynamormcore.DB, error) {
			return nil, nil
		},
	})
	require.ErrorContains(t, err, "storage client initialization returned nil")
}
