package common

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsecureTLSOverrideEnabled(t *testing.T) {
	originalValue, hadOriginal := os.LookupEnv(InsecureTLSOverrideEnvVar)
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv(InsecureTLSOverrideEnvVar, originalValue)
			return
		}
		_ = os.Unsetenv(InsecureTLSOverrideEnvVar)
	})

	_ = os.Unsetenv(InsecureTLSOverrideEnvVar)
	assert.False(t, InsecureTLSOverrideEnabled())

	_ = os.Setenv(InsecureTLSOverrideEnvVar, "not-a-bool")
	assert.False(t, InsecureTLSOverrideEnabled())

	_ = os.Setenv(InsecureTLSOverrideEnvVar, "1")
	assert.True(t, InsecureTLSOverrideEnabled())
}
