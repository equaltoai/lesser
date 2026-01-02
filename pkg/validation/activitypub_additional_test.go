package validation

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityPubValidator_ValidateActivity_AdditionalFailures(t *testing.T) {
	v := NewActivityPubValidator(zap.NewNop())

	t.Run("rejects oversized payload", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MaxObjectSize = 5

		_, err := v.ValidateActivity([]byte(`{"type":"Create"}`), cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "activity too large")
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowLocalURLs = true

		_, err := v.ValidateActivity([]byte(`{`), cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("rejects empty type when not required-field checked", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "",
			Actor: "https://example.com/actor",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.RequiredFields = []string{"actor"}
		cfg.AllowLocalURLs = true

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "activity type cannot be empty")
	})

	t.Run("rejects disallowed type", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "UnknownType",
			Actor: "https://example.com/actor",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = true

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not allowed")
	})

	t.Run("rejects invalid URL", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "https://%zz",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid URL")
	})

	t.Run("rejects unsupported URL scheme", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "ftp://example.com/actor",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid URL scheme")
	})

	t.Run("requires https in production", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")

		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "http://example.com/actor",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = true

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must use HTTPS")
	})

	t.Run("rejects private IP hosts when local URLs disallowed", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "")

		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "http://10.0.0.1/actor",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = false

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "private IP addresses not allowed")
	})

	t.Run("allows localhost when AllowLocalURLs true", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "http://127.0.0.1/actor",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = true

		activity, err := v.ValidateActivity(payload, cfg)
		require.NoError(t, err)
		require.Equal(t, "http://127.0.0.1/actor", activity.Actor)
	})

	t.Run("rejects empty hostname and covers unresolved-host SSRF path", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "https://",
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = false

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "hostname cannot be empty")
	})

	t.Run("rejects too many recipients", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "https://example.com/actor",
			To:    []string{"https://example.com/a", "https://example.com/b"},
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = true
		cfg.MaxArrayLength = 1

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too many recipients")
	})

	t.Run("rejects invalid recipient URLs", func(t *testing.T) {
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: "https://example.com/actor",
			To:    []string{"ftp://example.com"},
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = true

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid URL scheme")
	})

	t.Run("rejects overly long string fields", func(t *testing.T) {
		longActor := "https://example.com/" + strings.Repeat("a", 50)
		payload, err := json.Marshal(Activity{
			Type:  "Create",
			Actor: longActor,
		})
		require.NoError(t, err)

		cfg := DefaultConfig()
		cfg.AllowedTypes = []string{"Create"}
		cfg.AllowLocalURLs = true
		cfg.MaxStringLength = 10

		_, err = v.ValidateActivity(payload, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "string field actor too long")
	})
}

func TestActivityPubValidator_PrivateHelpers(t *testing.T) {
	v := NewActivityPubValidator(zap.NewNop())

	require.ErrorContains(t, v.validateHostname(""), "hostname cannot be empty")
	require.ErrorContains(t, v.validateHostname(strings.Repeat("a", 254)), "hostname too long")
	require.ErrorContains(t, v.validateHostname("bad_host!name"), "invalid hostname format")
	require.NoError(t, v.validateHostname("example.com"))

	require.True(t, v.isPrivateIP(net.ParseIP("10.0.0.1")))
	require.True(t, v.isPrivateIP(net.ParseIP("127.0.0.1")))
	require.True(t, v.isPrivateIP(net.ParseIP("fc00::1")))
	require.True(t, v.isPrivateIP(net.ParseIP("fe80::1")))
	require.True(t, v.isPrivateIP(net.ParseIP("::1")))
	require.False(t, v.isPrivateIP(net.ParseIP("8.8.8.8")))

	require.ErrorContains(t, v.validateHTTPSignature("keyId=abc"), "missing signature value")
	require.ErrorContains(t, v.validateHTTPSignature("signature=abc"), "missing keyId")
	require.NoError(t, v.validateHTTPSignature("keyId=abc,signature=def"))
}

func TestActivityPubValidator_ValidateNestedObjects_DepthExceeded(t *testing.T) {
	v := NewActivityPubValidator(zap.NewNop())

	cfg := DefaultConfig()
	cfg.MaxDepth = 1

	err := v.validateNestedObjects(&Activity{}, cfg, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "object nesting too deep")
}

func TestActivityPubValidator_ValidateInboxDelivery_SignaturePaths(t *testing.T) {
	v := NewActivityPubValidator(zap.NewNop())

	validPayload, err := json.Marshal(Activity{
		Type:  "Create",
		Actor: "https://example.com/actor",
		ID:    "https://example.com/activity/1",
	})
	require.NoError(t, err)

	_, err = v.ValidateInboxDelivery(validPayload, "keyId=abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing signature value")

	_, err = v.ValidateInboxDelivery(validPayload, "signature=abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing keyId")

	activity, err := v.ValidateInboxDelivery(validPayload, "keyId=abc,signature=def")
	require.NoError(t, err)
	require.Equal(t, "Create", activity.Type)

	// Missing id should fail inbox validation required fields.
	missingIDPayload, err := json.Marshal(Activity{
		Type:  "Create",
		Actor: "https://example.com/actor",
	})
	require.NoError(t, err)
	_, err = v.ValidateInboxDelivery(missingIDPayload, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required field: id")
}
