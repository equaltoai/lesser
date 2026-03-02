package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeIPPrefix_MoreBranches(t *testing.T) {
	assert.Equal(t, "", normalizeIPPrefix(""))
	assert.Equal(t, "", normalizeIPPrefix("   "))

	// X-Forwarded-For style lists.
	assert.Equal(t, "1.2.3.0/24", normalizeIPPrefix("1.2.3.4, 9.9.9.9"))

	// host:port parsing.
	assert.Equal(t, "1.2.3.0/24", normalizeIPPrefix("1.2.3.4:443"))
	assert.Equal(t, "2001:db8::/64", normalizeIPPrefix("[2001:db8::1]:443"))

	// Raw IPv6 normalization (/64 prefix).
	assert.Equal(t, "2001:db8::/64", normalizeIPPrefix("2001:db8::1"))

	// Invalid IPs return the raw string.
	assert.Equal(t, "not-an-ip", normalizeIPPrefix("not-an-ip"))
}
