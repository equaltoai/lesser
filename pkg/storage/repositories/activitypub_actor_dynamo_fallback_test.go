package repositories

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	tablesession "github.com/theory-cloud/tabletheory/pkg/session"
)

func TestDefaultRepositoryDynamoClient_UsesConfiguredEndpoint(t *testing.T) {
	previousConfigGetter := getRepositoryConfig
	previousSessionFactory := newRepositorySession
	t.Cleanup(func() {
		getRepositoryConfig = previousConfigGetter
		newRepositorySession = previousSessionFactory
		resetRepositoryDynamoClientCache()
	})
	resetRepositoryDynamoClientCache()

	getRepositoryConfig = func() *config.Config {
		return &config.Config{
			Region:           "us-west-2",
			DynamoDBEndpoint: "http://localhost:8000",
		}
	}

	var captured tablesession.Config
	newRepositorySession = func(cfg *tablesession.Config) (repositoryDynamoSession, error) {
		captured = *cfg
		return fakeRepositoryDynamoSession{client: &dynamodb.Client{}}, nil
	}

	client, err := defaultRepositoryDynamoClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "us-west-2", captured.Region)
	require.Equal(t, "http://localhost:8000", captured.Endpoint)
	require.NotNil(t, captured.CredentialsProvider)
}

func TestDefaultRepositoryDynamoClient_CachesClientByConfig(t *testing.T) {
	previousConfigGetter := getRepositoryConfig
	previousSessionFactory := newRepositorySession
	t.Cleanup(func() {
		getRepositoryConfig = previousConfigGetter
		newRepositorySession = previousSessionFactory
		resetRepositoryDynamoClientCache()
	})
	resetRepositoryDynamoClientCache()

	getRepositoryConfig = func() *config.Config {
		return &config.Config{
			Region:           "us-west-2",
			DynamoDBEndpoint: "http://localhost:8000",
		}
	}

	sessionCalls := 0
	newRepositorySession = func(cfg *tablesession.Config) (repositoryDynamoSession, error) {
		sessionCalls++
		return fakeRepositoryDynamoSession{client: &dynamodb.Client{}}, nil
	}

	first, err := defaultRepositoryDynamoClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := defaultRepositoryDynamoClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, second)

	require.Equal(t, 1, sessionCalls)
	require.Same(t, first, second)
}

func TestDefaultRepositoryDynamoClient_RetriesAfterInitializationFailure(t *testing.T) {
	previousConfigGetter := getRepositoryConfig
	previousSessionFactory := newRepositorySession
	t.Cleanup(func() {
		getRepositoryConfig = previousConfigGetter
		newRepositorySession = previousSessionFactory
		resetRepositoryDynamoClientCache()
	})
	resetRepositoryDynamoClientCache()

	getRepositoryConfig = func() *config.Config {
		return &config.Config{
			Region:           "us-west-2",
			DynamoDBEndpoint: "http://localhost:8000",
		}
	}

	sessionCalls := 0
	newRepositorySession = func(cfg *tablesession.Config) (repositoryDynamoSession, error) {
		sessionCalls++
		if sessionCalls == 1 {
			return nil, errors.New("transient init failure")
		}
		return fakeRepositoryDynamoSession{client: &dynamodb.Client{}}, nil
	}

	first, err := defaultRepositoryDynamoClient(context.Background())
	require.Error(t, err)
	require.Nil(t, first)

	second, err := defaultRepositoryDynamoClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, 2, sessionCalls)
}

type fakeRepositoryDynamoSession struct {
	client *dynamodb.Client
	err    error
}

func (f fakeRepositoryDynamoSession) Client() (*dynamodb.Client, error) {
	return f.client, f.err
}

func resetRepositoryDynamoClientCache() {
	repositoryDynamoClients = sync.Map{}
}
