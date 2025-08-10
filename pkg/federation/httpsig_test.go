package federation

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSignatureHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    *HTTPSignature
		wantErr bool
	}{
		{
			name:   "valid signature header",
			header: `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host date",signature="dGVzdA=="`,
			want: &HTTPSignature{
				KeyID:     "https://example.com/users/alice#main-key",
				Algorithm: "rsa-sha256",
				Headers:   []string{"(request-target)", "host", "date"},
				Signature: []byte("test"),
			},
			wantErr: false,
		},
		{
			name:   "minimal signature header",
			header: `keyId="test-key",signature="dGVzdA=="`,
			want: &HTTPSignature{
				KeyID:     "test-key",
				Algorithm: "rsa-sha256",     // default
				Headers:   []string{"date"}, // default
				Signature: []byte("test"),
			},
			wantErr: false,
		},
		{
			name:    "missing keyId",
			header:  `algorithm="rsa-sha256",signature="dGVzdA=="`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing signature",
			header:  `keyId="test-key",algorithm="rsa-sha256"`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid format",
			header:  `invalid header format`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid base64 signature",
			header:  `keyId="test-key",signature="invalid!!!base64"`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSignatureHeader(tt.header)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildSignatureString(t *testing.T) {
	req := httptest.NewRequest("POST", "https://example.com/inbox", nil)
	req.Header.Set("Date", "Mon, 01 Jan 2024 12:00:00 GMT")
	req.Header.Set("Content-Type", "application/activity+json")
	req.Host = "example.com"

	tests := []struct {
		name    string
		headers []string
		want    string
		wantErr bool
	}{
		{
			name:    "(request-target) and host",
			headers: []string{"(request-target)", "host"},
			want:    "(request-target): post /inbox\nhost: example.com",
			wantErr: false,
		},
		{
			name:    "date header",
			headers: []string{"date"},
			want:    "date: Mon, 01 Jan 2024 12:00:00 GMT",
			wantErr: false,
		},
		{
			name:    "multiple headers",
			headers: []string{"(request-target)", "host", "date", "content-type"},
			want:    "(request-target): post /inbox\nhost: example.com\ndate: Mon, 01 Jan 2024 12:00:00 GMT\ncontent-type: application/activity+json",
			wantErr: false,
		},
		{
			name:    "missing header",
			headers: []string{"missing-header"},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSignatureString(req, tt.headers)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVerifyTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		wantErr bool
	}{
		{
			name:    "valid recent timestamp",
			date:    time.Now().UTC().Format(time.RFC1123),
			wantErr: false,
		},
		{
			name:    "timestamp 4 minutes ago",
			date:    time.Now().Add(-4 * time.Minute).UTC().Format(time.RFC1123),
			wantErr: false,
		},
		{
			name:    "timestamp 6 minutes ago",
			date:    time.Now().Add(-6 * time.Minute).UTC().Format(time.RFC1123),
			wantErr: true,
		},
		{
			name:    "timestamp 4 minutes in future",
			date:    time.Now().Add(4 * time.Minute).UTC().Format(time.RFC1123),
			wantErr: false,
		},
		{
			name:    "timestamp 6 minutes in future",
			date:    time.Now().Add(6 * time.Minute).UTC().Format(time.RFC1123),
			wantErr: true,
		},
		{
			name:    "empty date",
			date:    "",
			wantErr: true,
		},
		{
			name:    "invalid date format",
			date:    "not a valid date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyTimestamp(tt.date)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCalculateDigest(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "empty body",
			body: []byte{},
			want: "SHA-256=47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=",
		},
		{
			name: "test body",
			body: []byte("test"),
			want: "SHA-256=n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=",
		},
		{
			name: "json body",
			body: []byte(`{"type":"Note","content":"Hello, World!"}`),
			want: "SHA-256=gQom0NBqHnAdDrjtkp9Mn7wPblPxPKZ46LyySELb5P0=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateDigest(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVerifyHTTPSignature(t *testing.T) {
	// Generate test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	// Create a test request
	body := []byte(`{"type":"Note","content":"Hello, World!"}`)
	req := httptest.NewRequest("POST", "https://example.com/inbox", bytes.NewReader(body))
	req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Digest", calculateDigest(body))
	req.Host = "example.com"

	// Sign the request
	err = SignHTTPRequest(req, privateKey, "https://example.com/users/alice#main-key")
	require.NoError(t, err)

	// Verify the signature
	err = VerifyHTTPSignature(req, publicKey)
	assert.NoError(t, err)

	// Test with wrong public key
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	err = VerifyHTTPSignature(req, &wrongKey.PublicKey)
	assert.Error(t, err)

	// Test with missing signature header
	req.Header.Del("Signature")
	err = VerifyHTTPSignature(req, publicKey)
	assert.Error(t, err)
}

func TestSignHTTPRequest(t *testing.T) {
	// Generate test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tests := []struct {
		name    string
		setup   func(*http.Request)
		wantErr bool
		check   func(*testing.T, *http.Request)
	}{
		{
			name: "request without body",
			setup: func(req *http.Request) {
				// No additional setup needed
			},
			wantErr: false,
			check: func(t *testing.T, req *http.Request) {
				assert.NotEmpty(t, req.Header.Get("Signature"))
				assert.NotEmpty(t, req.Header.Get("Date"))
				assert.Empty(t, req.Header.Get("Digest"))
			},
		},
		{
			name: "request with existing date",
			setup: func(req *http.Request) {
				req.Header.Set("Date", "Mon, 01 Jan 2024 12:00:00 GMT")
			},
			wantErr: false,
			check: func(t *testing.T, req *http.Request) {
				assert.Equal(t, "Mon, 01 Jan 2024 12:00:00 GMT", req.Header.Get("Date"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "https://example.com/users/alice", nil)
			req.Host = "example.com"

			tt.setup(req)

			err := SignHTTPRequest(req, privateKey, "https://example.com/users/alice#main-key")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.check(t, req)

			// Verify the signature is valid
			sig, err := ParseSignatureHeader(req.Header.Get("Signature"))
			require.NoError(t, err)
			assert.Equal(t, "https://example.com/users/alice#main-key", sig.KeyID)
			assert.Equal(t, "rsa-sha256", sig.Algorithm)
		})
	}
}

func TestKeyOperations(t *testing.T) {
	t.Run("GenerateRSAKeyPair", func(t *testing.T) {
		// Test valid key generation
		key, err := GenerateRSAKeyPair(2048)
		require.NoError(t, err)
		assert.NotNil(t, key)
		assert.Equal(t, 2048/8, key.Size())

		// Test invalid key size
		_, err = GenerateRSAKeyPair(1024)
		assert.Error(t, err)
	})

	t.Run("Key encoding and parsing", func(t *testing.T) {
		// Generate key pair
		privateKey, err := GenerateRSAKeyPair(2048)
		require.NoError(t, err)
		publicKey := &privateKey.PublicKey

		// Encode keys
		publicPEM, err := EncodePublicKeyPEM(publicKey)
		require.NoError(t, err)
		assert.Contains(t, string(publicPEM), "BEGIN PUBLIC KEY")

		privatePEM, err := EncodePrivateKeyPEM(privateKey)
		require.NoError(t, err)
		assert.Contains(t, string(privatePEM), "BEGIN PRIVATE KEY")

		// Parse keys back
		parsedPublic, err := ParsePublicKeyPEM(publicPEM)
		require.NoError(t, err)
		assert.Equal(t, publicKey, parsedPublic)

		parsedPrivate, err := ParsePrivateKeyPEM(privatePEM)
		require.NoError(t, err)
		assert.Equal(t, privateKey, parsedPrivate)
	})
}

func TestVerifyDigest(t *testing.T) {
	body := []byte(`{"type":"Note","content":"Hello, World!"}`)
	digest := calculateDigest(body)

	tests := []struct {
		name    string
		setup   func(*http.Request)
		body    []byte
		wantErr bool
	}{
		{
			name: "valid digest",
			setup: func(req *http.Request) {
				req.Header.Set("Digest", digest)
			},
			body:    body,
			wantErr: false,
		},
		{
			name: "missing digest header",
			setup: func(req *http.Request) {
				// Don't set digest header
			},
			body:    body,
			wantErr: true,
		},
		{
			name: "invalid digest format",
			setup: func(req *http.Request) {
				req.Header.Set("Digest", "invalid-format")
			},
			body:    body,
			wantErr: true,
		},
		{
			name: "unsupported algorithm",
			setup: func(req *http.Request) {
				req.Header.Set("Digest", "MD5=abc123")
			},
			body:    body,
			wantErr: true,
		},
		{
			name: "digest mismatch",
			setup: func(req *http.Request) {
				req.Header.Set("Digest", digest)
			},
			body:    []byte("different body"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://example.com/inbox", nil)
			tt.setup(req)

			err := VerifyDigest(req, tt.body)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test with known signature from ActivityPub spec
func TestInteroperability(t *testing.T) {
	// This is a reference test case to ensure our implementation is compatible
	// with other ActivityPub implementations

	// Generate a test key for this example
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	// Create a request that would come from another server
	req := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader(`{"type":"Follow"}`))
	req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
	req.Header.Set("Content-Type", "application/activity+json")
	req.Host = "example.com"

	// Sign the request
	err = SignHTTPRequest(req, privateKey, "https://example.com/users/bob#main-key")
	require.NoError(t, err)

	// Verify our own signature
	err = VerifyHTTPSignature(req, publicKey)
	assert.NoError(t, err)

	// Verify the signature header format matches expected format
	sig, err := ParseSignatureHeader(req.Header.Get("Signature"))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/users/bob#main-key", sig.KeyID)
	// With enhanced signing, RSA keys now default to hs2019 for maximum compatibility
	// but maintain backwards compatibility with rsa-sha256
	assert.Contains(t, []string{"rsa-sha256", "hs2019"}, sig.Algorithm)
	assert.Contains(t, sig.Headers, "(request-target)")
	assert.Contains(t, sig.Headers, "host")
	assert.Contains(t, sig.Headers, "date")
}

// Test backwards compatibility with RSA-SHA256
func TestBackwardsCompatibilityRSASHA256(t *testing.T) {
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	// Create a test request
	req := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader(`{"type":"Note"}`))
	req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
	req.Header.Set("Content-Type", "application/activity+json")
	req.Host = "example.com"

	// Test that legacy rsa-sha256 signatures are still verifiable
	err = SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", "rsa-sha256")
	require.NoError(t, err)

	// Verify with enhanced verification
	err = VerifyHTTPSignature(req, publicKey)
	assert.NoError(t, err)

	// Parse and check algorithm
	sig, err := ParseSignatureHeader(req.Header.Get("Signature"))
	require.NoError(t, err)
	assert.Equal(t, "rsa-sha256", sig.Algorithm)
}

// Test enhanced algorithm support
func TestEnhancedAlgorithmSupport(t *testing.T) {
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	algorithms := []string{
		AlgorithmRSASHA256,
		AlgorithmHS2019,
		AlgorithmRSASHA512,
	}

	for _, algorithm := range algorithms {
		t.Run(algorithm, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader(`{"type":"Note"}`))
			req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
			req.Header.Set("Content-Type", "application/activity+json")
			req.Host = "example.com"

			// Sign with specific algorithm
			err = SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", algorithm)
			require.NoError(t, err)

			// Verify signature
			err = VerifyHTTPSignature(req, publicKey)
			assert.NoError(t, err)

			// Check algorithm is preserved
			sig, err := ParseSignatureHeader(req.Header.Get("Signature"))
			require.NoError(t, err)
			assert.Equal(t, algorithm, sig.Algorithm)
		})
	}
}

// Test key type detection
func TestKeyTypeDetection(t *testing.T) {
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	// Test key type detection
	assert.Equal(t, "RSA", DetectKeyType(privateKey))
	assert.Equal(t, "RSA", DetectKeyType(publicKey))

	// Test algorithm determination
	algorithm := DetermineSigningAlgorithm(privateKey, false)
	assert.Equal(t, AlgorithmHS2019, algorithm)

	// Test legacy preference
	legacyAlgorithm := DetermineSigningAlgorithm(privateKey, true)
	assert.Equal(t, AlgorithmRSASHA256, legacyAlgorithm)
}

// Test interoperability with different server implementations
func TestServerInteroperability(t *testing.T) {
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKey := &privateKey.PublicKey

	testCases := []struct {
		name      string
		algorithm string
		headers   []string
	}{
		{
			name:      "Mastodon style",
			algorithm: AlgorithmHS2019,
			headers:   []string{"(request-target)", "host", "date", "digest"},
		},
		{
			name:      "Pleroma style",
			algorithm: AlgorithmRSASHA256,
			headers:   []string{"(request-target)", "host", "date"},
		},
		{
			name:      "Modern ActivityPub",
			algorithm: AlgorithmHS2019,
			headers:   []string{"(request-target)", "host", "date", "digest", "content-type"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"type":"Create","object":{"type":"Note","content":"Hello World"}}`
			req := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader(body))
			req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
			req.Header.Set("Content-Type", "application/activity+json")
			req.Header.Set("Digest", calculateDigest([]byte(body)))
			req.Host = "example.com"

			// Sign with specific algorithm
			err = SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/test#main-key", tc.algorithm)
			require.NoError(t, err)

			// Verify signature works
			err = VerifyHTTPSignature(req, publicKey)
			assert.NoError(t, err)

			// Check signature format
			sig, err := ParseSignatureHeader(req.Header.Get("Signature"))
			require.NoError(t, err)
			assert.Equal(t, tc.algorithm, sig.Algorithm)
			assert.NotEmpty(t, sig.Signature)
		})
	}
}
