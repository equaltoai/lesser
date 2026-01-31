package lift

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultRemoteSearchServiceFactory(t *testing.T) {
	require.NotNil(t, defaultRemoteSearchServiceFactory(nil))
}
