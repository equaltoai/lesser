package dynamodb

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestDynamoDBClient(t *testing.T) {
	// Setup test environment
	config.SetupTestEnvironment(t)

	// Test that we can create a client
	client, err := New()
	assert.NoError(t, err)
	assert.NotNil(t, client)
}
