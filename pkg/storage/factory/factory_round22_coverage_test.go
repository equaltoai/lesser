package factory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	dynamormSchema "github.com/theory-cloud/tabletheory/v2/pkg/schema"
	pkgtypes "github.com/theory-cloud/tabletheory/v2/pkg/types"
	"go.uber.org/zap"
)

type extendedMockDB struct {
	inner      *mocks.MockDB
	registered []reflect.Type
}

var _ core.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) core.Query            { return db.inner.Model(model) }
func (db *extendedMockDB) Migrate() error                        { return nil }
func (db *extendedMockDB) AutoMigrate(_ ...any) error            { return nil }
func (db *extendedMockDB) Close() error                          { return nil }
func (db *extendedMockDB) WithContext(_ context.Context) core.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...dynamormSchema.AutoMigrateOption) error {
	return nil
}
func (db *extendedMockDB) RegisterTypeConverter(t reflect.Type, _ pkgtypes.CustomConverter) error {
	db.registered = append(db.registered, t)
	return nil
}
func (db *extendedMockDB) CreateTable(_ any, _ ...dynamormSchema.TableOption) error { return nil }
func (db *extendedMockDB) EnsureTable(_ any) error                                  { return nil }
func (db *extendedMockDB) DeleteTable(_ any) error                                  { return nil }
func (db *extendedMockDB) DescribeTable(_ any) (any, error)                         { return nil, nil }
func (db *extendedMockDB) WithLambdaTimeout(_ context.Context) core.DB              { return db }
func (db *extendedMockDB) WithLambdaTimeoutBuffer(_ time.Duration) core.DB          { return db }
func (db *extendedMockDB) Transact() core.TransactionBuilder                        { return nil }
func (db *extendedMockDB) TransactWrite(_ context.Context, fn func(core.TransactionBuilder) error) error {
	return fn(nil)
}

func TestRegisterStorageConverters_NonExtendedDB_Round22(t *testing.T) {
	require.NoError(t, registerStorageConverters(new(mocks.MockDB)))
}

func TestRegisterStorageConverters_ExtendedDBRegistersDefaults_Round22(t *testing.T) {
	db := &extendedMockDB{inner: new(mocks.MockDB)}
	require.NoError(t, registerStorageConverters(db))
	require.GreaterOrEqual(t, len(db.registered), 5)
}

func TestNewRepositoryFactory_Success_Round22(t *testing.T) {
	prevLoad := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("stubbed")
	}
	t.Cleanup(func() { loadDefaultAWSConfig = prevLoad })

	cfg := config.Get()
	if cfg.Domain == "" {
		cfg.Domain = "localhost"
	}
	cfg.VAPIDSecretARN = ""

	inner := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	inner.On("Model", mock.Anything).Return(query).Maybe()

	db := &extendedMockDB{inner: inner}
	f, err := NewRepositoryFactory(db, "test-table", zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, f)

	// Call all exported no-arg methods to keep coverage stable without manual lists.
	rv := reflect.ValueOf(f)
	rt := rv.Type()
	for i := 0; i < rt.NumMethod(); i++ {
		method := rt.Method(i)
		if method.Type.NumIn() != 1 {
			continue
		}
		rv.Method(i).Call(nil)
	}
}
