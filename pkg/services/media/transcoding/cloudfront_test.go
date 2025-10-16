package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test RSA private key (for testing only - DO NOT use in production)
const testPrivateKeyPEM = `[REDACTED TEST KEY]`

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      CloudFrontConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: CloudFrontConfig{
				Domain:        "d123.cloudfront.net",
				KeyPairID:     "APKAXXXXXXX",
				PrivateKeyPEM: testPrivateKeyPEM,
				DefaultTTL:    24 * time.Hour,
			},
			expectError: false,
		},
		{
			name: "missing domain",
			config: CloudFrontConfig{
				KeyPairID:     "APKAXXXXXXX",
				PrivateKeyPEM: testPrivateKeyPEM,
			},
			expectError: true,
			errorMsg:    "domain is required",
		},
		{
			name: "missing key pair ID",
			config: CloudFrontConfig{
				Domain:        "d123.cloudfront.net",
				PrivateKeyPEM: testPrivateKeyPEM,
			},
			expectError: true,
			errorMsg:    "key pair ID is required",
		},
		{
			name: "missing private key",
			config: CloudFrontConfig{
				Domain:    "d123.cloudfront.net",
				KeyPairID: "APKAXXXXXXX",
			},
			expectError: true,
			errorMsg:    "private key is required",
		},
		{
			name: "invalid private key",
			config: CloudFrontConfig{
				Domain:        "d123.cloudfront.net",
				KeyPairID:     "APKAXXXXXXX",
				PrivateKeyPEM: "not-a-valid-key",
			},
			expectError: true,
			errorMsg:    "invalid private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewCloudFrontService(t *testing.T) {
	config := CloudFrontConfig{
		Domain:        "d123.cloudfront.net",
		KeyPairID:     "APKAXXXXXXX",
		PrivateKeyPEM: testPrivateKeyPEM,
		DefaultTTL:    12 * time.Hour,
	}

	service, err := NewCloudFrontService(config, nil)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, "d123.cloudfront.net", service.domain)
	assert.Equal(t, "APKAXXXXXXX", service.keyPairID)
	assert.Equal(t, 12*time.Hour, service.defaultTTL)
	assert.NotNil(t, service.privateKey)
}

func TestNewCloudFrontServiceDefaultTTL(t *testing.T) {
	config := CloudFrontConfig{
		Domain:        "d123.cloudfront.net",
		KeyPairID:     "APKAXXXXXXX",
		PrivateKeyPEM: testPrivateKeyPEM,
		// DefaultTTL not set
	}

	service, err := NewCloudFrontService(config, nil)
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, service.defaultTTL, "should default to 24 hours")
}

func TestSignURL(t *testing.T) {
	config := CloudFrontConfig{
		Domain:        "d123.cloudfront.net",
		KeyPairID:     "APKAXXXXXXX",
		PrivateKeyPEM: testPrivateKeyPEM,
	}

	service, err := NewCloudFrontService(config, nil)
	require.NoError(t, err)

	resourcePath := "media/test123/hls/master.m3u8"
	expireTime := time.Now().Add(1 * time.Hour)

	signedURL, err := service.SignURL(resourcePath, expireTime)
	require.NoError(t, err)
	assert.NotEmpty(t, signedURL)

	// Verify URL structure
	assert.Contains(t, signedURL, "https://d123.cloudfront.net/")
	assert.Contains(t, signedURL, "media/test123/hls/master.m3u8")
	assert.Contains(t, signedURL, "Policy=")
	assert.Contains(t, signedURL, "Signature=")
	assert.Contains(t, signedURL, "Key-Pair-Id=APKAXXXXXXX")
}

func TestSignStreamingURL(t *testing.T) {
	config := CloudFrontConfig{
		Domain:        "d123.cloudfront.net",
		KeyPairID:     "APKAXXXXXXX",
		PrivateKeyPEM: testPrivateKeyPEM,
	}

	service, err := NewCloudFrontService(config, nil)
	require.NoError(t, err)

	tests := []struct {
		name         string
		mediaID      string
		format       string
		quality      *string
		expectedPath string
		expectError  bool
	}{
		{
			name:         "HLS master",
			mediaID:      "media123",
			format:       "hls",
			quality:      nil,
			expectedPath: "media123/hls/master.m3u8",
			expectError:  false,
		},
		{
			name:         "HLS 720p",
			mediaID:      "media123",
			format:       "hls",
			quality:      stringPtr("720p"),
			expectedPath: "media123/hls/720p.m3u8",
			expectError:  false,
		},
		{
			name:         "DASH manifest",
			mediaID:      "media123",
			format:       "dash",
			quality:      nil,
			expectedPath: "media123/dash/manifest.mpd",
			expectError:  false,
		},
		{
			name:        "unsupported format",
			mediaID:     "media123",
			format:      "rtmp",
			quality:     nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signedURL, err := service.SignStreamingURL(tt.mediaID, tt.format, tt.quality, 1*time.Hour)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, signedURL, tt.expectedPath)
				assert.Contains(t, signedURL, "Policy=")
				assert.Contains(t, signedURL, "Signature=")
			}
		})
	}
}

func TestSignBatchURLs(t *testing.T) {
	config := CloudFrontConfig{
		Domain:        "d123.cloudfront.net",
		KeyPairID:     "APKAXXXXXXX",
		PrivateKeyPEM: testPrivateKeyPEM,
	}

	service, err := NewCloudFrontService(config, nil)
	require.NoError(t, err)

	paths := []string{
		"media/test1/hls/master.m3u8",
		"media/test2/hls/master.m3u8",
		"media/test3/hls/master.m3u8",
	}

	signedURLs, err := service.SignBatchURLs(paths, 1*time.Hour)
	require.NoError(t, err)
	assert.Len(t, signedURLs, 3)

	for _, path := range paths {
		signedURL, ok := signedURLs[path]
		assert.True(t, ok, "path %s should be in results", path)
		assert.NotEmpty(t, signedURL)
		assert.Contains(t, signedURL, "Policy=")
	}
}

func TestGetExpirationTime(t *testing.T) {
	config := CloudFrontConfig{
		Domain:        "d123.cloudfront.net",
		KeyPairID:     "APKAXXXXXXX",
		PrivateKeyPEM: testPrivateKeyPEM,
		DefaultTTL:    24 * time.Hour,
	}

	service, err := NewCloudFrontService(config, nil)
	require.NoError(t, err)

	now := time.Now()

	// Test with custom TTL
	expiration := service.GetExpirationTime(12 * time.Hour)
	assert.True(t, expiration.After(now.Add(11*time.Hour)))
	assert.True(t, expiration.Before(now.Add(13*time.Hour)))

	// Test with zero TTL (should use default)
	expiration = service.GetExpirationTime(0)
	assert.True(t, expiration.After(now.Add(23*time.Hour)))
	assert.True(t, expiration.Before(now.Add(25*time.Hour)))
}

func TestMakeURLSafe(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		contains    []string
		notContains []string
	}{
		{
			name:        "replaces plus",
			input:       "abc+def",
			contains:    []string{"-"},
			notContains: []string{"+"},
		},
		{
			name:        "replaces equals",
			input:       "abc=def",
			contains:    []string{"_"},
			notContains: []string{"="},
		},
		{
			name:        "replaces slash",
			input:       "abc/def",
			contains:    []string{"~"},
			notContains: []string{"/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeURLSafe(tt.input)
			for _, substr := range tt.contains {
				assert.Contains(t, result, substr)
			}
			for _, substr := range tt.notContains {
				assert.NotContains(t, result, substr)
			}
		})
	}
}

func TestParsePrivateKey(t *testing.T) {
	tests := []struct {
		name        string
		pemStr      string
		expectError bool
	}{
		{
			name:        "valid key",
			pemStr:      testPrivateKeyPEM,
			expectError: false,
		},
		{
			name:        "invalid PEM",
			pemStr:      "not a PEM",
			expectError: true,
		},
		{
			name:        "empty string",
			pemStr:      "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := parsePrivateKey(tt.pemStr)
			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, key)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, key)
			}
		})
	}
}

func TestCloudFrontPolicyJSON(t *testing.T) {
	expireTime := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)

	policy := cloudFrontPolicy{
		Statement: []policyStatement{
			{
				Resource: "https://example.com/test.m3u8",
				Condition: policyCondition{
					DateLessThan: awsEpochTime{Time: expireTime},
				},
			},
		},
	}

	jsonBytes, err := policy.Statement[0].Condition.DateLessThan.MarshalJSON()
	require.NoError(t, err)

	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, "AWS:EpochTime")
	assert.Contains(t, jsonStr, "1767225599") // Unix timestamp for 2025-12-31 23:59:59 UTC
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
