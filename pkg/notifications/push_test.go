package notifications

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatNotificationTitle(t *testing.T) {
	require.Equal(t, "alice followed you", FormatNotificationTitle("follow", "alice"))
	require.Equal(t, "New notification", FormatNotificationTitle("unknown", "alice"))
}

func TestFormatNotificationBody(t *testing.T) {
	require.Equal(t, "", FormatNotificationBody("follow", "hi"))

	long := strings.Repeat("a", 200)
	body := FormatNotificationBody("mention", long)
	require.Len(t, body, 100)
	require.True(t, strings.HasSuffix(body, "..."))
}
