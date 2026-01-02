package common

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeysHelpers(t *testing.T) {
	assert.Equal(t, []string{"user", "123"}, SplitKey("user#123"))
	assert.Equal(t, "user#123", JoinKey("user", "123"))
}

func TestRunningUnitTests_Branches(t *testing.T) {
	origFlags := flag.CommandLine
	origArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = origFlags
		os.Args = origArgs
	})

	// In normal test execution, the harness flag exists.
	assert.True(t, RunningUnitTests())

	// Force Lookup("test.v") to be nil to cover fallback logic.
	flag.CommandLine = flag.NewFlagSet("round14", flag.ContinueOnError)
	os.Args = []string{"not-a-test-binary"}
	assert.False(t, RunningUnitTests())

	os.Args = []string{"some-binary.test"}
	assert.True(t, RunningUnitTests())

	os.Args = []string{"go-build1234"}
	assert.True(t, RunningUnitTests())
}
