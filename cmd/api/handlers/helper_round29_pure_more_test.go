package handlers

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestOAuthDeviceCodeExpiresAt_Round29MoreCoverage(t *testing.T) {
	now := time.Date(2026, 4, 6, 18, 5, 0, 0, time.UTC)
	require.Equal(t, now.Add(time.Duration(oauthDeviceCodeTTLSeconds)*time.Second), oauthDeviceCodeExpiresAt(now))
	require.WithinDuration(t, time.Now().Add(time.Duration(oauthDeviceCodeTTLSeconds)*time.Second), oauthDeviceCodeExpiresAt(time.Time{}), 2*time.Second)
}

func TestSafeHTTPURL_Round29MoreCoverage(t *testing.T) {
	safe, ok := safeHTTPURL("https://example.com/path?q=1")
	require.True(t, ok)
	require.Equal(t, "https://example.com/path?q=1", safe)

	_, ok = safeHTTPURL("ftp://example.com/path")
	require.False(t, ok)

	_, ok = safeHTTPURL("https:///missing-host")
	require.False(t, ok)

	_, ok = safeHTTPURL("   ")
	require.False(t, ok)
}

func TestExtractStringField_Round29MoreCoverage(t *testing.T) {
	handler := &Handler{}
	require.Equal(t, "value", handler.extractStringField(map[string]any{"field": "value"}, "field"))
	require.Equal(t, "", handler.extractStringField(map[string]any{"field": 42}, "field"))
	require.Equal(t, "", handler.extractStringField(nil, "field"))
}

func TestIsImageAttachment_Round29MoreCoverage(t *testing.T) {
	handler := &Handler{}
	require.True(t, handler.isImageAttachment(activitypub.Attachment{Type: "Document", MediaType: "image/png"}))
	require.True(t, handler.isImageAttachment(activitypub.Attachment{Type: "Image", MediaType: "image/jpeg"}))
	require.False(t, handler.isImageAttachment(activitypub.Attachment{Type: "Video", MediaType: "image/png"}))
	require.False(t, handler.isImageAttachment(activitypub.Attachment{Type: "Image", MediaType: "text/plain"}))
}
