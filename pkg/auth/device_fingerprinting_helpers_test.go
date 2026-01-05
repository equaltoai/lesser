package auth

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDeviceFingerprintManager_HelperCoverage(t *testing.T) {
	t.Parallel()

	dfm := NewDeviceFingerprintManager(nil, zap.NewNop(), nil)
	require.NotNil(t, dfm.config)

	require.Equal(t, "Chrome", dfm.extractBrowserFromUA("Mozilla/5.0 Chrome/120"))
	require.Equal(t, "Firefox", dfm.extractBrowserFromUA("Mozilla/5.0 Firefox/120"))
	require.Equal(t, "Safari", dfm.extractBrowserFromUA("Mozilla/5.0 Safari/605.1.15"))
	require.Equal(t, "Edge", dfm.extractBrowserFromUA("Mozilla/5.0 Edge/120"))
	require.Equal(t, "Unknown", dfm.extractBrowserFromUA("Mozilla/5.0"))

	require.True(t, dfm.isHighRiskUserAgentChange("Mozilla/5.0 Chrome/120", "Mozilla/5.0 Firefox/120"))
	require.False(t, dfm.isHighRiskUserAgentChange("Mozilla/5.0 Chrome/120", "Mozilla/5.0 Chrome/121"))

	device := &storage.Device{LastUserAgent: "Mozilla/5.0 Chrome/120", LastIPAddress: "192.0.2.1"}
	fp := &EnhancedDeviceFingerprint{UserAgent: "Mozilla/5.0 Firefox/120", IPAddress: "192.0.3.1"}
	changes := dfm.detectDeviceChanges(device, fp)
	require.Contains(t, changes, "user_agent")
	require.Contains(t, changes, "ip_address")
	require.True(t, dfm.containsChange(changes, "user_agent"))
	require.False(t, dfm.containsChange(changes, "missing"))

	risk := dfm.calculateDeviceRiskScore(device, fp, changes)
	require.GreaterOrEqual(t, risk, 0.7)

	// Cap risk at 1.0.
	many := make([]string, 25)
	for i := range many {
		many[i] = "x"
	}
	many[0] = "user_agent"
	many[1] = "ip_address"
	require.Equal(t, 1.0, dfm.calculateDeviceRiskScore(device, fp, many))

	require.Equal(t, "Windows", dfm.extractPlatformFromUserAgent("Mozilla/5.0 (Windows NT 10.0)"))
	require.Equal(t, "macOS", dfm.extractPlatformFromUserAgent("Mozilla/5.0 (Macintosh)"))
	require.Equal(t, "Linux", dfm.extractPlatformFromUserAgent("Mozilla/5.0 (X11; Linux x86_64)"))
	require.Equal(t, "Android", dfm.extractPlatformFromUserAgent("Mozilla/5.0 (Android 13)"))
	require.Equal(t, "iOS", dfm.extractPlatformFromUserAgent("Mozilla/5.0 (iPhone)"))
	require.Equal(t, "Unknown", dfm.extractPlatformFromUserAgent("Mozilla/5.0"))

	require.Equal(t, "mobile", dfm.detectDeviceType("Mozilla/5.0 iPhone Mobile"))
	require.Equal(t, "tablet", dfm.detectDeviceType("Mozilla/5.0 iPad Tablet"))
	require.Equal(t, "desktop", dfm.detectDeviceType("Mozilla/5.0"))

	require.True(t, dfm.isIPInSameNetwork("192.0.2.1", "192.0.2.99"))
	require.False(t, dfm.isIPInSameNetwork("192.0.2.1", "192.0.3.1"))
	require.False(t, dfm.isIPInSameNetwork("bad", "192.0.2.1"))
	require.False(t, dfm.isIPInSameNetwork("2001:db8::1", "2001:db8::2"))
}

