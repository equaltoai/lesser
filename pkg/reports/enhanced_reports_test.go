package reports

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/stretchr/testify/require"
)

func TestGetSeverityString(t *testing.T) {
	require.Equal(t, "1", getSeverityString(moderation.SeverityLow))
	require.Equal(t, "2", getSeverityString(moderation.SeverityMedium))
	require.Equal(t, "3", getSeverityString(moderation.SeverityHigh))
	require.Equal(t, "4", getSeverityString(moderation.SeverityCritical))
	require.Equal(t, "2", getSeverityString(moderation.Severity(99)))
}
