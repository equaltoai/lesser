package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrFeatureDisabled(t *testing.T) {
	err := ErrFeatureDisabled("test-feature")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "feature disabled")
	assert.Contains(t, err.Error(), "test-feature")
}

// Note: IsModerationMLEnabled and MustCheckModerationMLAccess depend on
// pkg/config.Get() which returns nil in tests without proper setup.
// Testing the nil config path.

func TestIsModerationMLEnabled_NilConfig(t *testing.T) {
	// When config is nil, should return false
	result := IsModerationMLEnabled(nil, "test-tenant")
	assert.False(t, result)
}

func TestMustCheckModerationMLAccess_NilConfig(t *testing.T) {
	// When config is nil, IsModerationMLEnabled returns false, so error expected
	err := MustCheckModerationMLAccess(nil, "test-tenant")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "feature disabled")
}
