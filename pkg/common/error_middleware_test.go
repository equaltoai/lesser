package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppTheoryStatusForErrorCode(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{code: "app.bad_request", want: 400},
		{code: "app.validation_failed", want: 400},
		{code: "app.unauthorized", want: 401},
		{code: "app.forbidden", want: 403},
		{code: "app.not_found", want: 404},
		{code: "app.method_not_allowed", want: 405},
		{code: "app.conflict", want: 409},
		{code: "app.too_large", want: 413},
		{code: "app.timeout", want: 408},
		{code: "app.rate_limited", want: 429},
		{code: "app.overloaded", want: 503},
		{code: "app.internal", want: 500},
		{code: "unknown", want: 500},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			require.Equal(t, tc.want, appTheoryStatusForErrorCode(tc.code))
		})
	}
}

