package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"
)

func TestNewBootstrapDBFactoryCreatesConfiguredDB(t *testing.T) {
	factory := newBootstrapDBFactory(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
	})

	db, err := factory()
	require.NoError(t, err)
	require.NotNil(t, db)
}
