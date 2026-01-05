package dynamorm

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetClientState() {
	client = nil
	lambdaDB = nil
	clientErr = nil
	clientOnce = sync.Once{}
}

func TestGetClient_UsesDefaultRegionAndLocalEndpointCredentials(t *testing.T) {
	resetClientState()

	origGetConfig := getAppConfig
	origNewClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormStandardClient = origNewClient
		resetClientState()
	})

	getAppConfig = func() *config.Config {
		return &config.Config{
			Region:           "",
			DynamoDBEndpoint: "http://localhost:8000",
		}
	}

	var got session.Config
	calls := 0
	newDynamormStandardClient = func(cfg session.Config) (core.DB, error) {
		calls++
		got = cfg
		return fakeDB{}, nil
	}

	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	db, err := GetClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, db)

	assert.Equal(t, 1, calls)
	assert.Equal(t, "us-east-1", got.Region)
	assert.Equal(t, "http://localhost:8000", got.Endpoint)
	assert.Equal(t, "fakeMyKeyId", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "fakeSecretAccessKey", os.Getenv("AWS_SECRET_ACCESS_KEY"))

	// Second call should reuse the singleton.
	db2, err := GetClient(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, db, db2)
}

func TestGetClient_UsesConfiguredRegion(t *testing.T) {
	resetClientState()

	origGetConfig := getAppConfig
	origNewClient := newDynamormStandardClient
	t.Cleanup(func() {
		getAppConfig = origGetConfig
		newDynamormStandardClient = origNewClient
		resetClientState()
	})

	getAppConfig = func() *config.Config {
		return &config.Config{Region: "eu-west-1"}
	}

	var got session.Config
	newDynamormStandardClient = func(cfg session.Config) (core.DB, error) {
		got = cfg
		return fakeDB{}, nil
	}

	_, err := GetClient(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", got.Region)
}

func TestWithTimeoutBuffer_NilAndNonLambdaDB(t *testing.T) {
	assert.Nil(t, WithTimeoutBuffer(nil, 0))

	db := fakeDB{}
	assert.Equal(t, db, WithTimeoutBuffer(db, 0))
}
