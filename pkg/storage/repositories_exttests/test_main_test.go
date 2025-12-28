package repositories_exttests

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("DYNAMODB_TABLE") == "" &&
		os.Getenv("DYNAMO_TABLE_NAME") == "" &&
		os.Getenv("ENVIRONMENT") == "" &&
		os.Getenv("STAGE") == "" {
		_ = os.Setenv("DYNAMODB_TABLE", "test-table")
	}

	if os.Getenv("JWT_SECRET") == "" {
		_ = os.Setenv("JWT_SECRET", "dummy")
	}

	os.Exit(m.Run())
}
