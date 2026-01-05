package repositories

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestURLValidator_Round08_NormalizeAndExtractURLsFromText(t *testing.T) {
	validator := NewURLValidator(zap.NewNop())

	require.Equal(t, "https://example.com", validator.normalizeURL("www.example.com/"))
	require.Equal(t, "https://example.com/path", validator.normalizeURL("example.com/path/"))
	require.Equal(t, "not-a-url", validator.normalizeURL("not-a-url"))

	urls := validator.extractURLsFromText(`see https://example.com! and example.org/path). also example.org/path`)
	require.GreaterOrEqual(t, len(urls), 2)
	require.Contains(t, urls, "https://example.com")
	require.Contains(t, urls, "https://example.org/path")
}

func TestURLValidator_Round08_ExtractAndValidateURL_TagsAndShorteners(t *testing.T) {
	ctx := context.Background()
	validator := NewURLValidator(zap.NewNop())

	insecure, err := validator.ExtractAndValidateURL(ctx, "http://example.tk")
	require.NoError(t, err)
	require.True(t, insecure.IsValid)
	require.False(t, insecure.IsSecure)
	require.Contains(t, insecure.ValidationTags, "insecure_http")
	require.Contains(t, insecure.ValidationTags, "suspicious_tld")

	shortened, err := validator.ExtractAndValidateURL(ctx, "https://bit.ly/abc")
	require.NoError(t, err)
	require.True(t, shortened.IsValid)
	require.True(t, shortened.IsShortened)
	require.Contains(t, shortened.ValidationTags, "shortened_url")
	require.Contains(t, shortened.ValidationTags, "url_shortener")

	internal, err := validator.ExtractAndValidateURL(ctx, "http://127.0.0.1")
	require.NoError(t, err)
	require.True(t, internal.IsValid)
	require.Contains(t, internal.ValidationTags, "internal_url")
}

func TestURLValidator_Round08_ValidateAndNormalizeProfileURLs_Warnings(t *testing.T) {
	ctx := context.Background()
	validator := NewURLValidator(zap.NewNop())

	fields := []map[string]string{
		{"name": "http", "value": "http://example.com"},
		{"name": "tld", "value": "https://example.tk/path"},
		{"name": "short", "value": "https://bit.ly/xyz"},
		{"name": "bad", "value": "http://localhost"},
	}

	normalized, warnings, err := validator.ValidateAndNormalizeProfileURLs(ctx, fields)
	require.NoError(t, err)
	require.Len(t, normalized, 4)

	require.Equal(t, "http://example.com", normalized[0]["value"])
	require.Equal(t, "https://example.tk/path", normalized[1]["value"])
	require.Equal(t, "https://bit.ly/xyz", normalized[2]["value"])

	warnText := strings.Join(warnings, "\n")
	require.Contains(t, warnText, "insecure HTTP")
	require.Contains(t, warnText, "suspicious domain")
	require.Contains(t, warnText, "shortened URL")
	require.Contains(t, warnText, "Invalid URL")
}

func TestURLValidator_Round08_EnhancedExtractAccountFromReply_PathFallback(t *testing.T) {
	ctx := context.Background()
	validator := NewURLValidator(zap.NewNop())

	username, err := validator.EnhancedExtractAccountFromReply(ctx, "https://example.com/some/path/alice")
	require.NoError(t, err)
	require.Equal(t, "alice", username)
}

func TestURLValidator_Round08_ExtractUsernameFromPath_LongSegmentIgnored(t *testing.T) {
	validator := NewURLValidator(zap.NewNop())
	longSeg := strings.Repeat("a", 51)
	require.Equal(t, "", validator.extractUsernameFromPath("/users/"+longSeg))
}
