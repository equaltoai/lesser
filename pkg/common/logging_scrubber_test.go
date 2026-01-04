package common

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/logging"
	"github.com/stretchr/testify/require"
)

func TestLogger_UsesScrubbingCore(t *testing.T) {
	_, ok := Logger().Core().(*logging.ScrubbingCore)
	require.True(t, ok)
}

func TestSecurityLogger_UsesScrubbingCore(t *testing.T) {
	orig := SecurityLogger
	t.Cleanup(func() { SecurityLogger = orig })

	SecurityLogger = nil
	InitSecurityLogger()

	_, ok := SecurityLogger.Core().(*logging.ScrubbingCore)
	require.True(t, ok)
}
