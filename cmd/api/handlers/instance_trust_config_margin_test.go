package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveExplicitLesserHostTrustBaseURL_CoverageMargin(t *testing.T) {
	t.Run("no explicit env", func(t *testing.T) {
		t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
		t.Setenv("LESSER_HOST_URL", "")

		base, ok := resolveExplicitLesserHostTrustBaseURL()
		require.False(t, ok)
		require.Empty(t, base)
	})

	t.Run("attestations url wins and trims", func(t *testing.T) {
		t.Setenv("LESSER_HOST_ATTESTATIONS_URL", " https://trust.example/attestations/// ")
		t.Setenv("LESSER_HOST_URL", "https://host.example")

		base, ok := resolveExplicitLesserHostTrustBaseURL()
		require.True(t, ok)
		require.Equal(t, "https://trust.example/attestations", base)
	})

	t.Run("host url fallback trims", func(t *testing.T) {
		t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "   ")
		t.Setenv("LESSER_HOST_URL", " https://host.example/// ")

		base, ok := resolveExplicitLesserHostTrustBaseURL()
		require.True(t, ok)
		require.Equal(t, "https://host.example", base)
	})
}

func TestValidateTrustBaseURL_CoverageMargin(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "empty",
			raw:     "   ",
			wantErr: "missing base URL",
		},
		{
			name:    "bad scheme",
			raw:     "ftp://trust.example",
			wantErr: "base URL must include http(s) scheme",
		},
		{
			name:    "missing host",
			raw:     "https:///attestations",
			wantErr: "base URL missing hostname",
		},
		{
			name:    "lambda url host",
			raw:     "https://abc.lambda-url.us-east-1.on.aws",
			wantErr: "lambda function URL hosts are not supported",
		},
		{
			name: "https success",
			raw:  " https://trust.example/attestations ",
		},
		{
			name: "http success",
			raw:  "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTrustBaseURL(tt.raw)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsLambdaFunctionURLHost_CoverageMargin(t *testing.T) {
	require.True(t, isLambdaFunctionURLHost(" ABC.lambda-url.us-east-1.on.aws "))
	require.False(t, isLambdaFunctionURLHost("lambda-url.us-east-1.amazonaws.com"))
	require.False(t, isLambdaFunctionURLHost(strings.TrimSpace("trust.example")))
}
