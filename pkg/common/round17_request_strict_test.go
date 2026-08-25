package common

import (
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestParseRequestStrict(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("success", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{
			Method: "POST",
			Path:   "/test",
			Body:   []byte(`{"name":"alice"}`),
		}}

		var out payload
		err := ParseRequestStrict(ctx, &out)
		require.NoError(t, err)
		require.Equal(t, "alice", out.Name)
	})

	t.Run("missing context or body", func(t *testing.T) {
		var out payload

		err := ParseRequestStrict(nil, &out)
		appErr, ok := apperrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, apperrors.CodeValidationFailed, appErr.Code)

		ctx := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test"}}
		err = ParseRequestStrict(ctx, &out)
		appErr, ok = apperrors.AsAppError(err)
		require.True(t, ok)
		require.Equal(t, apperrors.CodeValidationFailed, appErr.Code)
	})

	t.Run("unknown field invalid json and trailing payload rejected", func(t *testing.T) {
		cases := []string{
			`{"name":"alice","extra":"x"}`,
			`{"name":`,
			`{"name":"alice"} {"name":"bob"}`,
		}

		for _, body := range cases {
			ctx := &apptheory.Context{Request: apptheory.Request{
				Method: "POST",
				Path:   "/test",
				Body:   []byte(body),
			}}

			var out payload
			err := ParseRequestStrict(ctx, &out)
			appErr, ok := apperrors.AsAppError(err)
			require.True(t, ok)
			require.Equal(t, apperrors.CodeValidationFailed, appErr.Code)
		}
	})
}

func TestParseRequestStrictWithValidation(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "POST",
		Path:   "/test",
		Body:   []byte(`{"name":"alice","extra":"x"}`),
	}}

	var out payload
	resp, err := ParseRequestStrictWithValidation(ctx, &out)
	require.NoError(t, err)
	status, body := parseResponse(t, resp)
	require.Equal(t, 400, status)
	require.Equal(t, string(apperrors.CodeValidationFailed), body.Code)
	require.NotEmpty(t, body.Error)
}
