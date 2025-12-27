package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuality_JSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(QualityHigh)
	require.NoError(t, err)
	require.Equal(t, `"high"`, string(b))

	var q Quality
	require.NoError(t, json.Unmarshal(b, &q))
	require.Equal(t, QualityHigh, q)
}

func TestMediaFormat_JSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(FormatHLS)
	require.NoError(t, err)
	require.Equal(t, `"hls"`, string(b))

	var f MediaFormat
	require.NoError(t, json.Unmarshal(b, &f))
	require.Equal(t, FormatHLS, f)
}
