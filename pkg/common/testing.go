package common

import (
	"flag"
	"os"
	"strings"
)

// RunningUnitTests reports whether the current binary is the Go test harness.
// It uses the convention that test binaries end with the .test suffix.
func RunningUnitTests() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	name := os.Args[0]
	return strings.HasSuffix(name, ".test") || strings.Contains(name, "go-build")
}
