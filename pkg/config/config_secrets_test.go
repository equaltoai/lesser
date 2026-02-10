package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOptionalSecretFromEnvOrARN_ValueEnvTakesPrecedence(t *testing.T) {
	t.Setenv("UNIT_TEST_SECRET_VALUE", "  value-from-env  ")
	t.Setenv("UNIT_TEST_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:ignored")

	assert.Equal(t, "value-from-env", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestGetOptionalSecretFromEnvOrARN_EmptyWhenUnset(t *testing.T) {
	t.Setenv("UNIT_TEST_SECRET_VALUE", "")
	t.Setenv("UNIT_TEST_SECRET_ARN", "")

	assert.Equal(t, "", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestGetOptionalSecretFromEnvOrARN_ArnSkipsAwsInTests(t *testing.T) {
	originalArgs := append([]string(nil), os.Args...)
	t.Cleanup(func() { os.Args = originalArgs })

	// Ensure we do not attempt to call AWS in unit tests, even if an ARN is present.
	os.Args[0] = "config.test"

	t.Setenv("UNIT_TEST_SECRET_VALUE", "")
	t.Setenv("UNIT_TEST_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:unit-test")

	assert.Equal(t, "", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}
