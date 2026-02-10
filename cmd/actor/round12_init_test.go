package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type noopCoreDB struct{}

func (noopCoreDB) Model(any) dynamormCore.Query { return nil }

func (noopCoreDB) Transaction(fn func(tx *dynamormCore.Tx) error) error {
	if fn == nil {
		return nil
	}
	return fn(nil)
}

func (noopCoreDB) Migrate() error { return nil }

func (noopCoreDB) AutoMigrate(...any) error { return nil }

func (noopCoreDB) Close() error { return nil }

func (noopCoreDB) WithContext(context.Context) dynamormCore.DB { return noopCoreDB{} }

func TestInitializeActor_ManualStorageInitialization_Round12(t *testing.T) {
	origMustInitializeLambdaFn := mustInitializeLambdaFn
	origInitializeWithDefaultsFn := initializeWithDefaultsFn
	origNewLambdaOptimizedClientFn := newLambdaOptimizedClientFn
	origNewRepositoryFactoryFn := newRepositoryFactoryFn

	origLambdaCtx := lambdaCtx
	origCfg := cfg
	origLogger := logger
	origRepos := repos

	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInitializeLambdaFn
		initializeWithDefaultsFn = origInitializeWithDefaultsFn
		newLambdaOptimizedClientFn = origNewLambdaOptimizedClientFn
		newRepositoryFactoryFn = origNewRepositoryFactoryFn

		lambdaCtx = origLambdaCtx
		cfg = origCfg
		logger = origLogger
		repos = origRepos
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:   &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
			Logger:   zap.NewNop(),
			Repos:    nil,
			DynamoDB: nil,
		}
	}

	initializeWithDefaultsFn = func(*common.LambdaContext) error {
		return errors.New("defaults not available in unit test")
	}

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return noopCoreDB{}, nil
	}

	newRepositoryFactoryFn = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (storageCore.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}

	initializeActor()

	if lambdaCtx == nil {
		t.Fatalf("expected lambdaCtx to be set")
	}
	if lambdaCtx.DynamoDB == nil {
		t.Fatalf("expected lambdaCtx.DynamoDB to be set")
	}
	if lambdaCtx.Repos == nil {
		t.Fatalf("expected lambdaCtx.Repos to be set")
	}
	if repos == nil {
		t.Fatalf("expected repos to be set")
	}
}
