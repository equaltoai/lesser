package lift

import (
	stdErrors "errors"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestLiftErrorConstructors(t *testing.T) {
	notFound := NotFoundError("thing")
	require.IsType(t, &lift.LiftError{}, notFound)
	require.Equal(t, 404, notFound.StatusCode)

	validation := ValidationErrorWithField("field", "bad")
	require.Equal(t, 422, validation.StatusCode)
	require.Equal(t, "field", validation.Details["field"])

	unauthorized := UnauthorizedError("")
	require.Equal(t, 401, unauthorized.StatusCode)

	forbidden := ForbiddenError("read", "resource")
	require.Equal(t, 403, forbidden.StatusCode)
	require.Contains(t, forbidden.Message, "Not authorized")

	conflict := ConflictError("resource", "dupe")
	require.Equal(t, 409, conflict.StatusCode)
	require.Equal(t, "resource", conflict.Details["resource"])

	rateLimited := RateLimitError("")
	require.Equal(t, 429, rateLimited.StatusCode)

	federationErr := FederationError("deliver", "remote", stdErrors.New("boom"))
	require.Equal(t, 502, federationErr.StatusCode)
	require.Equal(t, "deliver", federationErr.Details["operation"])

	internal := InternalError("")
	require.Equal(t, 500, internal.StatusCode)

	serviceUnavailable := ServiceUnavailableError("db")
	require.Equal(t, 503, serviceUnavailable.StatusCode)
	require.Equal(t, "db", serviceUnavailable.Details["service"])

	timeout := TimeoutError("op")
	require.Equal(t, 504, timeout.StatusCode)
	require.Equal(t, "op", timeout.Details["operation"])
}
