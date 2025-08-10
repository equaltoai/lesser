// Package testing provides a simple test to verify the infrastructure compiles
package testing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSimple verifies the test infrastructure is working
func TestSimple(t *testing.T) {
	assert.True(t, true, "Simple test should pass")
}

// TestConstants verifies test constants are accessible
func TestConstants(t *testing.T) {
	assert.NotEmpty(t, TestDomain)
	assert.NotEmpty(t, TestUsername)
	assert.NotEmpty(t, TestTableName)
}