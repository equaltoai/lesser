package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoutingErrors(t *testing.T) {
	require.Equal(t, "No healthy routes available", ErrNoHealthyRoutes.Error())
	require.Equal(t, "Circuit breaker is open", ErrCircuitOpen.Error())

	custom := &RoutingError{Code: "X", Message: "msg"}
	require.Equal(t, "msg", custom.Error())
}
